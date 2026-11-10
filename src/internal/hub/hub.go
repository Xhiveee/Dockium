package hub

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	agentv1 "dockium/gen/agent/v1"
	"dockium/internal/auth"
	"dockium/internal/db"
	"dockium/internal/present"
	"dockium/internal/realtime"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

const (
	heartbeatWindow = 30 * time.Second
	pingInterval    = 15 * time.Second
)

var ErrOffline = errors.New("агент офлайн")

type CommandError struct {
	Message string
}

func (e *CommandError) Error() string {
	return e.Message
}

type TimeoutError struct{}

func (e *TimeoutError) Error() string {
	return "агент не ответил вовремя"
}

type Hub struct {
	store     *db.DB
	publisher *realtime.Publisher
	log       *slog.Logger

	mu       sync.RWMutex
	sessions map[int64]*session
	streams  map[string]*Stream
	nextID   atomic.Int64
}

func New(store *db.DB, publisher *realtime.Publisher, log *slog.Logger) *Hub {
	return &Hub{
		store:     store,
		publisher: publisher,
		log:       log,
		sessions:  make(map[int64]*session),
		streams:   make(map[string]*Stream),
	}
}

type session struct {
	hub    *Hub
	agent  *db.Agent
	stream agentv1.AgentService_ConnectServer
	ctx    context.Context
	cancel context.CancelFunc
	send   chan *agentv1.ServerMessage
	done   chan struct{}

	closeOnce sync.Once
	lastSeen  atomic.Int64
	lastPing  atomic.Int64

	mu      sync.Mutex
	pending map[int64]chan *agentv1.CommandResult
	pings   map[int64]time.Time
}

type Stream struct {
	AgentID int64
	ID      string
	ch      chan *agentv1.AgentMessage
	done    chan struct{}
	once    sync.Once
	hub     *Hub
}

func (s *Stream) Messages() <-chan *agentv1.AgentMessage {
	return s.ch
}

func (s *Stream) Done() <-chan struct{} {
	return s.done
}

func (s *Stream) Close() {
	s.once.Do(func() {
		close(s.done)
		s.hub.mu.Lock()
		if s.hub.streams[s.ID] == s {
			delete(s.hub.streams, s.ID)
		}
		s.hub.mu.Unlock()
	})
}

func (h *Hub) Connect(stream agentv1.AgentService_ConnectServer) error {
	first, err := receiveFirst(stream, 20*time.Second)
	if err != nil {
		return err
	}
	hello := first.GetHello()
	if hello == nil || hello.Token == "" {
		return status.Error(codes.Unauthenticated, "агент не представился")
	}
	lookupCtx, cancelLookup := context.WithTimeout(stream.Context(), 5*time.Second)
	agent, err := h.store.AgentByTokenHash(lookupCtx, auth.TokenHash(hello.Token))
	cancelLookup()
	if err != nil {
		h.log.Warn("agent authentication failed", "remote", peerAddress(stream), "error", err)
		return status.Error(codes.Unauthenticated, "неизвестный токен агента")
	}
	if !agent.Enabled {
		h.log.Info("agent connection rejected: disabled", "agent_id", agent.ID, "name", agent.Name)
		return status.Error(codes.FailedPrecondition, "агент отключён")
	}

	ctx, cancel := context.WithCancel(stream.Context())
	now := time.Now()
	connection := &session{
		hub:     h,
		agent:   agent,
		stream:  stream,
		ctx:     ctx,
		cancel:  cancel,
		send:    make(chan *agentv1.ServerMessage, 256),
		done:    make(chan struct{}),
		pending: make(map[int64]chan *agentv1.CommandResult),
		pings:   make(map[int64]time.Time),
	}
	connection.lastSeen.Store(now.UnixMilli())
	connection.lastPing.Store(now.UnixMilli())

	h.mu.Lock()
	previous := h.sessions[agent.ID]
	h.sessions[agent.ID] = connection
	h.mu.Unlock()
	if previous != nil {
		h.log.Info("agent reconnected", "agent_id", agent.ID, "name", agent.Name)
		previous.close()
	}

	helloCtx, cancelHello := context.WithTimeout(ctx, 5*time.Second)
	err = h.store.MarkAgentOnline(helloCtx, agent.ID, hello.Version, hello.Hostname, hello.Platform, hello.Arch, hello.DockerVersion, "", hello.CpuModel, hello.CpuCores, now.UnixMilli())
	cancelHello()
	if err != nil {
		h.log.Error("mark agent online", "agent_id", agent.ID, "error", err)
	}
	h.publishAgent(agent.ID)
	h.publishInstances(agent.ID)

	connection.enqueue(&agentv1.ServerMessage{Payload: &agentv1.ServerMessage_Welcome{
		Welcome: &agentv1.Welcome{
			AgentId:          agent.ID,
			AgentName:        agent.Name,
			ServerTime:       time.Now().UnixMilli(),
			HeartbeatSeconds: 5,
		},
	}})
	go connection.sendLoop()
	connection.syncInstances()
	h.log.Info("agent connected", "agent_id", agent.ID, "name", agent.Name, "version", hello.Version)

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		connection.readLoop()
	}()
	select {
	case <-readDone:
	case <-connection.done:
	case <-ctx.Done():
	}
	connection.close()
	h.log.Info("agent disconnected", "agent_id", agent.ID, "name", agent.Name)
	return nil
}

func (h *Hub) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			h.mu.RLock()
			sessions := make([]*session, 0, len(h.sessions))
			for _, value := range h.sessions {
				sessions = append(sessions, value)
			}
			h.mu.RUnlock()
			for _, connection := range sessions {
				seen := time.UnixMilli(connection.lastSeen.Load())
				if now.Sub(seen) > heartbeatWindow {
					h.log.Warn("agent heartbeat timeout", "agent_id", connection.agent.ID)
					connection.close()
					continue
				}
				if now.UnixMilli()-connection.lastPing.Load() >= pingInterval.Milliseconds() {
					connection.ping()
				}
			}
		}
	}
}

func (h *Hub) CloseAgent(agentID int64) {
	h.mu.RLock()
	connection := h.sessions[agentID]
	h.mu.RUnlock()
	if connection != nil {
		connection.close()
	}
}

func (h *Hub) Online(agentID int64) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.sessions[agentID] != nil
}

func (h *Hub) Send(agentID int64, command *agentv1.Command) error {
	connection := h.session(agentID)
	if connection == nil {
		return ErrOffline
	}
	command.Id = h.nextID.Add(1)
	if !connection.enqueue(&agentv1.ServerMessage{Payload: &agentv1.ServerMessage_Command{Command: command}}) {
		return ErrOffline
	}
	return nil
}

func (h *Hub) Execute(ctx context.Context, agentID int64, command *agentv1.Command, timeout time.Duration) (*agentv1.CommandResult, error) {
	connection := h.session(agentID)
	if connection == nil {
		return nil, ErrOffline
	}
	command.Id = h.nextID.Add(1)
	response := make(chan *agentv1.CommandResult, 1)
	connection.mu.Lock()
	connection.pending[command.Id] = response
	connection.mu.Unlock()
	defer func() {
		connection.mu.Lock()
		delete(connection.pending, command.Id)
		connection.mu.Unlock()
	}()
	if !connection.enqueue(&agentv1.ServerMessage{Payload: &agentv1.ServerMessage_Command{Command: command}}) {
		return nil, ErrOffline
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case result := <-response:
		if result.Error != "" {
			return result, &CommandError{Message: result.Error}
		}
		return result, nil
	case <-timer.C:
		return nil, &TimeoutError{}
	case <-connection.done:
		return nil, ErrOffline
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (h *Hub) Subscribe(agentID int64, streamID string) (*Stream, error) {
	if h.session(agentID) == nil {
		return nil, ErrOffline
	}
	stream := &Stream{
		AgentID: agentID,
		ID:      streamID,
		ch:      make(chan *agentv1.AgentMessage, 512),
		done:    make(chan struct{}),
		hub:     h,
	}
	h.mu.Lock()
	existing := h.streams[streamID]
	h.streams[streamID] = stream
	h.mu.Unlock()
	if existing != nil {
		existing.Close()
	}
	return stream, nil
}

func (h *Hub) PublishAgent(agentID int64) {
	h.publishAgent(agentID)
}

func (h *Hub) PublishInstances(agentID int64) {
	h.publishInstances(agentID)
}

func (h *Hub) session(agentID int64) *session {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.sessions[agentID]
}

func (s *session) readLoop() {
	for {
		message, err := s.stream.Recv()
		if err != nil {
			return
		}
		now := time.Now().UnixMilli()
		s.lastSeen.Store(now)
		switch payload := message.Payload.(type) {
		case *agentv1.AgentMessage_Hello:
			continue
		case *agentv1.AgentMessage_Heartbeat:
			s.hub.handleHeartbeat(s, payload.Heartbeat)
		case *agentv1.AgentMessage_Result:
			s.resolve(payload.Result)
		case *agentv1.AgentMessage_Console, *agentv1.AgentMessage_Stream, *agentv1.AgentMessage_FileChunk:
			s.hub.route(message)
		case *agentv1.AgentMessage_Pong:
			s.handlePong(payload.Pong)
		}
	}
}

func (s *session) sendLoop() {
	for {
		select {
		case <-s.done:
			return
		case message := <-s.send:
			if err := s.stream.Send(message); err != nil {
				s.close()
				return
			}
		}
	}
}

func (s *session) enqueue(message *agentv1.ServerMessage) bool {
	select {
	case <-s.done:
		return false
	default:
	}
	select {
	case s.send <- message:
		return true
	case <-s.done:
		return false
	default:
		select {
		case <-s.send:
		default:
		}
		select {
		case s.send <- message:
			return true
		case <-s.done:
			return false
		default:
			return false
		}
	}
}

func (s *session) resolve(result *agentv1.CommandResult) {
	s.mu.Lock()
	channel, ok := s.pending[result.Id]
	s.mu.Unlock()
	if !ok {
		return
	}
	select {
	case channel <- result:
	default:
	}
}

func (s *session) syncInstances() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	instances, err := s.hub.store.ListInstancesByAgent(ctx, s.agent.ID)
	if err != nil || len(instances) == 0 {
		return
	}
	specs := make([]*agentv1.InstanceSpec, 0, len(instances))
	for _, instance := range instances {
		specs = append(specs, SpecFromInstance(instance))
	}
	command := &agentv1.Command{Id: s.hub.nextID.Add(1)}
	command.Payload = &agentv1.Command_Sync{Sync: &agentv1.SyncInstances{Specs: specs}}
	s.enqueue(&agentv1.ServerMessage{Payload: &agentv1.ServerMessage_Command{Command: command}})
}

func (s *session) ping() {
	nonce := time.Now().UnixNano()
	s.lastPing.Store(time.Now().UnixMilli())
	s.mu.Lock()
	s.pings[nonce] = time.Now()
	if len(s.pings) > 8 {
		for key := range s.pings {
			if len(s.pings) <= 8 {
				break
			}
			delete(s.pings, key)
		}
	}
	s.mu.Unlock()
	s.hub.log.Debug("agent ping sent", "agent_id", s.agent.ID, "nonce", nonce)
	s.enqueue(&agentv1.ServerMessage{Payload: &agentv1.ServerMessage_Ping{Ping: &agentv1.Ping{Nonce: nonce}}})
}

func (s *session) handlePong(pong *agentv1.Pong) {
	s.mu.Lock()
	sent, ok := s.pings[pong.Nonce]
	delete(s.pings, pong.Nonce)
	s.mu.Unlock()
	if !ok {
		return
	}
	latency := time.Since(sent)
	if latency > 5*time.Second {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := s.hub.store.SetAgentLatency(ctx, s.agent.ID, uint32(latency.Milliseconds())); err != nil {
		s.hub.log.Warn("set agent latency", "error", err)
		return
	}
	s.hub.log.Debug("agent pong", "agent_id", s.agent.ID, "latency_ms", latency.Milliseconds())
	s.hub.publishAgent(s.agent.ID)
}

func (s *session) close() {
	s.closeOnce.Do(func() {
		close(s.done)
		s.cancel()
		s.hub.remove(s)
	})
}

func (h *Hub) remove(connection *session) {
	h.mu.Lock()
	current := h.sessions[connection.agent.ID] == connection
	if current {
		delete(h.sessions, connection.agent.ID)
	}
	streams := make([]*Stream, 0)
	for id, stream := range h.streams {
		if stream.AgentID == connection.agent.ID {
			delete(h.streams, id)
			streams = append(streams, stream)
		}
	}
	h.mu.Unlock()
	for _, stream := range streams {
		stream.once.Do(func() { close(stream.done) })
	}
	connection.mu.Lock()
	connection.pending = make(map[int64]chan *agentv1.CommandResult)
	connection.pings = make(map[int64]time.Time)
	connection.mu.Unlock()
	if !current {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.store.MarkAgentOffline(ctx, connection.agent.ID); err != nil {
		h.log.Error("mark agent offline", "agent_id", connection.agent.ID, "error", err)
	}
	h.publishAgent(connection.agent.ID)
	h.publishInstances(connection.agent.ID)
}

func (h *Hub) handleHeartbeat(connection *session, heartbeat *agentv1.Heartbeat) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	agent, err := h.store.AgentByID(ctx, connection.agent.ID)
	if err != nil {
		return
	}
	telemetry := db.AgentTelemetry{
		Version:       agent.Version,
		Hostname:      agent.Hostname,
		Platform:      agent.Platform,
		Arch:          agent.Arch,
		DockerVersion: agent.DockerVersion,
		DockerOK:      heartbeat.DockerOk,
		DockerError:   heartbeat.DockerError,
		CPUModel:      agent.CPUModel,
		CPUCores:      agent.CPUCores,
		LatencyMS:     agent.LatencyMS,
	}
	if heartbeat.Host != nil {
		telemetry.CPUPercent = heartbeat.Host.CpuPercent
		telemetry.MemUsed = heartbeat.Host.MemoryUsedBytes
		telemetry.MemTotal = heartbeat.Host.MemoryTotalBytes
		telemetry.DiskUsed = heartbeat.Host.DiskUsedBytes
		telemetry.DiskTotal = heartbeat.Host.DiskTotalBytes
		telemetry.NetInBPS = heartbeat.Host.NetworkInBps
		telemetry.NetOutBPS = heartbeat.Host.NetworkOutBps
	}
	seenAt := time.Now().UnixMilli()
	if err := h.store.ApplyAgentTelemetry(ctx, agent.ID, telemetry, seenAt); err != nil {
		h.log.Error("apply agent telemetry", "agent_id", agent.ID, "error", err)
	}
	instances, err := h.store.ListInstancesByAgent(ctx, agent.ID)
	if err != nil {
		return
	}
	byID := make(map[int64]*db.Instance, len(instances))
	for _, instance := range instances {
		byID[instance.ID] = instance
	}
	for _, state := range heartbeat.Instances {
		instance, ok := byID[state.Id]
		if !ok {
			continue
		}
		_ = instance
		usage := db.InstanceUsageUpdate{
			CPUPercent:    state.CpuPercent,
			MemBytes:      state.MemoryBytes,
			MemLimitBytes: state.MemoryLimitBytes,
			NetInBPS:      state.NetworkInBps,
			NetOutBPS:     state.NetworkOutBps,
		}
		_ = h.store.ApplyInstanceState(ctx, state.Id, present.StatusFromAgent(int32(state.Status)), state.Error, state.ContainerId, state.ExitCode, state.StartedAt, usage)
	}
	h.publishAgent(agent.ID)
	h.publishInstances(agent.ID)
}

func (h *Hub) route(message *agentv1.AgentMessage) {
	streamID := ""
	switch payload := message.Payload.(type) {
	case *agentv1.AgentMessage_Console:
		streamID = payload.Console.StreamId
	case *agentv1.AgentMessage_Stream:
		streamID = payload.Stream.StreamId
	case *agentv1.AgentMessage_FileChunk:
		streamID = payload.FileChunk.StreamId
	}
	if streamID == "" {
		return
	}
	h.mu.RLock()
	stream := h.streams[streamID]
	h.mu.RUnlock()
	if stream == nil {
		return
	}
	switch message.Payload.(type) {
	case *agentv1.AgentMessage_FileChunk:
		select {
		case stream.ch <- message:
		case <-stream.done:
		case <-time.After(15 * time.Second):
			stream.Close()
		}
	default:
		select {
		case stream.ch <- message:
		default:
			select {
			case <-stream.ch:
			default:
			}
			select {
			case stream.ch <- message:
			default:
			}
		}
	}
}

func (h *Hub) publishAgent(agentID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	agent, err := h.store.AgentByID(ctx, agentID)
	if err != nil {
		return
	}
	total, running, err := h.store.InstanceCounts(ctx, agentID)
	if err != nil {
		return
	}
	h.publisher.AgentUpdated(agent, total, running)
}

func (h *Hub) publishInstances(agentID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	instances, err := h.store.ListInstancesByAgent(ctx, agentID)
	if err != nil {
		return
	}
	for _, instance := range instances {
		h.publisher.InstanceUpdated(instance)
	}
}

func SpecFromInstance(instance *db.Instance) *agentv1.InstanceSpec {
	return &agentv1.InstanceSpec{
		Id:              instance.ID,
		Name:            instance.Name,
		Image:           instance.Image,
		ContainerName:   instance.ContainerName,
		Command:         instance.Command,
		Environment:     specEnv(instance.Environment),
		Ports:           specPorts(instance.Ports),
		Volumes:         specVolumes(instance.Volumes),
		NetworkMode:     instance.NetworkMode,
		RestartPolicy:   instance.RestartPolicy,
		MemoryMb:        instance.MemoryMB,
		Cpus:            instance.CPUs,
		Tty:             instance.TTY,
		DataMount:       instance.DataMount,
		ConsoleEncoding: instance.Encoding,
		AutoStart:       instance.AutoStart,
		AutoRestart:     instance.AutoRestart,
	}
}

func specEnv(items []db.EnvVar) []*agentv1.EnvVar {
	result := make([]*agentv1.EnvVar, 0, len(items))
	for _, item := range items {
		result = append(result, &agentv1.EnvVar{Key: item.Key, Value: item.Value})
	}
	return result
}

func specPorts(items []db.PortMapping) []*agentv1.PortMapping {
	result := make([]*agentv1.PortMapping, 0, len(items))
	for _, item := range items {
		result = append(result, &agentv1.PortMapping{HostPort: item.HostPort, ContainerPort: item.ContainerPort, Protocol: item.Protocol})
	}
	return result
}

func specVolumes(items []db.VolumeMount) []*agentv1.VolumeMount {
	result := make([]*agentv1.VolumeMount, 0, len(items))
	for _, item := range items {
		result = append(result, &agentv1.VolumeMount{HostPath: item.HostPath, ContainerPath: item.ContainerPath, ReadOnly: item.ReadOnly})
	}
	return result
}

func receiveFirst(stream agentv1.AgentService_ConnectServer, timeout time.Duration) (*agentv1.AgentMessage, error) {
	type outcome struct {
		message *agentv1.AgentMessage
		err     error
	}
	channel := make(chan outcome, 1)
	go func() {
		message, err := stream.Recv()
		channel <- outcome{message: message, err: err}
	}()
	select {
	case result := <-channel:
		if result.err != nil {
			return nil, status.Error(codes.InvalidArgument, "соединение закрыто до приветствия")
		}
		return result.message, nil
	case <-time.After(timeout):
		return nil, status.Error(codes.DeadlineExceeded, "агент не отправил приветствие")
	}
}

func peerAddress(stream agentv1.AgentService_ConnectServer) string {
	info, ok := peer.FromContext(stream.Context())
	if !ok {
		return ""
	}
	return info.Addr.String()
}
