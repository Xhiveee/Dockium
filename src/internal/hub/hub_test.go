package hub

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	agentv1 "dockium/gen/agent/v1"
	"dockium/internal/auth"
	"dockium/internal/db"
	"dockium/internal/realtime"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type fakeAgentServer struct {
	agentv1.UnimplementedAgentServiceServer
	hub *Hub
}

func (f *fakeAgentServer) Connect(stream agentv1.AgentService_ConnectServer) error {
	return f.hub.Connect(stream)
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}

func TestAgentSessionLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store, err := db.Open(ctx, t.TempDir()+"/panel.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	rawToken, tokenHash, err := auth.NewToken()
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	agent, err := store.CreateAgent(ctx, "local", "", tokenHash)
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	instance, err := store.CreateInstance(ctx, &db.Instance{
		Name: "game", AgentID: agent.ID, Image: "alpine", ContainerName: "game-container",
		NetworkMode: "bridge", RestartPolicy: "no", TTY: true, DataMount: "/data", Encoding: "utf-8",
	})
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}

	publisher := realtime.NewPublisher()
	agentEvents, stopAgents := publisher.SubscribeAgents(32)
	defer stopAgents()
	instanceEvents, stopInstances := publisher.SubscribeInstances(32)
	defer stopInstances()

	agentHub := New(store, publisher, logger)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	agentv1.RegisterAgentServiceServer(grpcServer, &fakeAgentServer{hub: agentHub})
	go func() { _ = grpcServer.Serve(listener) }()
	defer grpcServer.Stop()

	connection, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer connection.Close()
	streamCtx, streamCancel := context.WithCancel(ctx)
	defer streamCancel()
	client := agentv1.NewAgentServiceClient(connection)
	stream, err := client.Connect(streamCtx)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	type incoming struct {
		message *agentv1.ServerMessage
		err     error
	}
	incomingMessages := make(chan incoming, 16)
	go func() {
		for {
			message, err := stream.Recv()
			incomingMessages <- incoming{message: message, err: err}
			if err != nil {
				return
			}
		}
	}()

	var sendMutex sync.Mutex
	send := func(message *agentv1.AgentMessage) error {
		sendMutex.Lock()
		defer sendMutex.Unlock()
		return stream.Send(message)
	}
	next := func(timeout time.Duration) *agentv1.ServerMessage {
		t.Helper()
		select {
		case item := <-incomingMessages:
			if item.err != nil {
				t.Fatalf("receive: %v", item.err)
			}
			return item.message
		case <-time.After(timeout):
			t.Fatal("timed out waiting for an agent message")
			return nil
		}
	}

	if err := send(&agentv1.AgentMessage{Payload: &agentv1.AgentMessage_Hello{Hello: &agentv1.Hello{
		Token: rawToken, Version: "2.0.0", Hostname: "test-host", Platform: "linux", Arch: "amd64",
	}}}); err != nil {
		t.Fatalf("send hello: %v", err)
	}

	welcome := next(2 * time.Second).GetWelcome()
	if welcome == nil || welcome.AgentId != agent.ID {
		t.Fatalf("welcome = %+v", welcome)
	}
	sync := next(2 * time.Second).GetCommand().GetSync()
	if sync == nil || len(sync.Specs) != 1 || sync.Specs[0].ContainerName != "game-container" {
		t.Fatalf("sync = %+v", sync)
	}

	waitFor(t, 2*time.Second, func() bool {
		stored, err := store.AgentByID(ctx, agent.ID)
		return err == nil && stored.Online
	})
	select {
	case event := <-agentEvents:
		if event.Agent == nil || !event.Agent.Agent.Online {
			t.Fatalf("agent event = %+v", event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("agent online event not published")
	}

	if err := send(&agentv1.AgentMessage{Payload: &agentv1.AgentMessage_Heartbeat{Heartbeat: &agentv1.Heartbeat{
		Host:     &agentv1.HostStats{CpuPercent: 22.5, MemoryUsedBytes: 1024, MemoryTotalBytes: 4096},
		DockerOk: true,
		Instances: []*agentv1.InstanceState{{
			Id: instance.ID, Status: agentv1.InstanceStatus_INSTANCE_STATUS_RUNNING,
			ContainerId: "abc", CpuPercent: 4.5, MemoryBytes: 512, MemoryLimitBytes: 1024,
		}},
	}}}); err != nil {
		t.Fatalf("send heartbeat: %v", err)
	}
	waitFor(t, 2*time.Second, func() bool {
		stored, err := store.InstanceByID(ctx, instance.ID)
		return err == nil && stored.Status == db.StatusRunning && stored.CPUPercent == 4.5
	})
	select {
	case event := <-instanceEvents:
		if event.Instance == nil || event.Instance.ID != instance.ID {
			t.Fatalf("instance event = %+v", event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("instance event not published")
	}

	resultValue := make(chan *agentv1.CommandResult, 1)
	executeError := make(chan error, 1)
	go func() {
		result, err := agentHub.Execute(ctx, agent.ID, &agentv1.Command{Payload: &agentv1.Command_Start{
			Start: &agentv1.InstanceTarget{Id: instance.ID},
		}}, 2*time.Second)
		executeError <- err
		if result != nil {
			resultValue <- result
		}
	}()
	command := next(2 * time.Second).GetCommand()
	if command == nil || command.GetStart() == nil {
		t.Fatalf("command = %+v", command)
	}
	if err := send(&agentv1.AgentMessage{Payload: &agentv1.AgentMessage_Result{Result: &agentv1.CommandResult{
		Id: command.Id,
		Payload: &agentv1.CommandResult_Instance{Instance: &agentv1.InstanceState{
			Id: instance.ID, Status: agentv1.InstanceStatus_INSTANCE_STATUS_RUNNING, ContainerId: "abc",
		}},
	}}}); err != nil {
		t.Fatalf("send result: %v", err)
	}
	if err := <-executeError; err != nil {
		t.Fatalf("execute error: %v", err)
	}
	if result := <-resultValue; result.GetInstance() == nil {
		t.Fatalf("result = %+v", result)
	}

	subscription, err := agentHub.Subscribe(agent.ID, "console-stream")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer subscription.Close()
	if err := agentHub.Send(agent.ID, &agentv1.Command{Payload: &agentv1.Command_ConsoleAttach{
		ConsoleAttach: &agentv1.ConsoleAttach{StreamId: "console-stream", InstanceId: instance.ID},
	}}); err != nil {
		t.Fatalf("attach: %v", err)
	}
	if command := next(2 * time.Second).GetCommand(); command.GetConsoleAttach() == nil {
		t.Fatalf("attach command = %+v", command)
	}
	if err := send(&agentv1.AgentMessage{Payload: &agentv1.AgentMessage_Console{Console: &agentv1.ConsoleData{
		StreamId: "console-stream",
		Lines:    []*agentv1.ConsoleLine{{Seq: 1, Kind: 1, Text: "hello"}},
	}}}); err != nil {
		t.Fatalf("send console: %v", err)
	}
	select {
	case message := <-subscription.Messages():
		if message.GetConsole() == nil || message.GetConsole().Lines[0].Text != "hello" {
			t.Fatalf("console message = %+v", message)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("console message was not routed")
	}

	streamCancel()
	waitFor(t, 3*time.Second, func() bool {
		stored, err := store.AgentByID(ctx, agent.ID)
		return err == nil && !stored.Online
	})
	if agentHub.Online(agent.ID) {
		t.Fatal("agent must be offline after disconnect")
	}
}

func TestSpecFromInstance(t *testing.T) {
	instance := &db.Instance{
		ID: 4, Name: "game", Image: "alpine", ContainerName: "container",
		Command: []string{"sh"}, Environment: []db.EnvVar{{Key: "A", Value: "B"}},
		Ports:       []db.PortMapping{{HostPort: 1, ContainerPort: 2, Protocol: "tcp"}},
		Volumes:     []db.VolumeMount{{HostPath: "/a", ContainerPath: "/b", ReadOnly: true}},
		NetworkMode: "bridge", RestartPolicy: "no", MemoryMB: 512, CPUs: 1.5,
		TTY: true, DataMount: "/data", Encoding: "utf-8", AutoStart: true, AutoRestart: true,
	}
	spec := SpecFromInstance(instance)
	if spec.Id != 4 || spec.ContainerName != "container" || len(spec.Environment) != 1 ||
		len(spec.Ports) != 1 || len(spec.Volumes) != 1 || !spec.Tty || !spec.AutoRestart {
		t.Fatalf("spec = %+v", spec)
	}
}
