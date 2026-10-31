package agent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	agentv1 "dockium/gen/agent/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

type Config struct {
	PanelURL      string
	Token         string
	DataDir       string
	TLSCA         string
	TLSSkipVerify bool
	TLSServerName string
	Version       string
}

type Client struct {
	cfg     Config
	log     *slog.Logger
	docker  *Docker
	manager *Manager

	mu            sync.Mutex
	dockerOK      bool
	dockerError   string
	dockerVer     string
	dockerChecked time.Time

	autoMu sync.Mutex

	outgoing chan *agentv1.AgentMessage
	done     chan struct{}
}

func Run(ctx context.Context, cfg Config, log *slog.Logger) error {
	docker := NewDocker()
	manager, err := NewManager(cfg.DataDir, docker, log)
	if err != nil {
		return err
	}
	client := &Client{cfg: cfg, log: log, docker: docker, manager: manager}
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			manager.console.Shutdown()
			return nil
		}
		started := time.Now()
		err := client.runSession(ctx)
		manager.console.Shutdown()
		if ctx.Err() != nil {
			return nil
		}
		if time.Since(started) > 30*time.Second {
			backoff = time.Second
		}
		log.Warn("agent connection lost", "error", err, "retry_in", backoff.String())
		jitter := time.Duration(rand.Int63n(int64(backoff / 2)))
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff + jitter):
		}
		backoff *= 2
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
	}
}

func (c *Client) runSession(parent context.Context) error {
	target, secure, err := parsePanelURL(c.cfg.PanelURL)
	if err != nil {
		return err
	}
	options := []grpc.DialOption{
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(64<<20), grpc.MaxCallSendMsgSize(64<<20)),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{Time: 15 * time.Second, Timeout: 5 * time.Second, PermitWithoutStream: true}),
	}
	if secure || c.cfg.TLSCA != "" {
		tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: c.cfg.TLSServerName}
		if c.cfg.TLSSkipVerify {
			tlsConfig.InsecureSkipVerify = true
		}
		if c.cfg.TLSCA != "" {
			data, err := os.ReadFile(c.cfg.TLSCA)
			if err != nil {
				return fmt.Errorf("read CA certificate: %w", err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(data) {
				return errors.New("не удалось прочитать CA сертификат")
			}
			tlsConfig.RootCAs = pool
		}
		options = append(options, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	} else {
		options = append(options, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	connection, err := grpc.NewClient(target, options...)
	if err != nil {
		return fmt.Errorf("не удалось создать соединение: %w", err)
	}
	defer connection.Close()

	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	client := agentv1.NewAgentServiceClient(connection)
	stream, err := client.Connect(ctx)
	if err != nil {
		return fmt.Errorf("не удалось подключиться к панели: %w", err)
	}

	host := CollectHost(c.manager.dataRoot)
	hello := &agentv1.Hello{
		Token:         c.cfg.Token,
		Version:       c.cfg.Version,
		Hostname:      hostname(),
		Platform:      runtime.GOOS,
		Arch:          runtime.GOARCH,
		DockerVersion: c.dockerVersion(ctx),
		CpuModel:      host.CPUModel,
		CpuCores:      host.CPUCores,
		StartedAt:     time.Now().UnixMilli(),
	}
	if err := stream.Send(&agentv1.AgentMessage{Payload: &agentv1.AgentMessage_Hello{Hello: hello}}); err != nil {
		return fmt.Errorf("не удалось отправить приветствие: %w", err)
	}
	welcome, err := receiveWelcome(stream, 20*time.Second)
	if err != nil {
		return err
	}
	c.log.Info("connected to panel", "agent", welcome.AgentName, "agent_id", welcome.AgentId, "panel", target)

	c.outgoing = make(chan *agentv1.AgentMessage, 512)
	c.done = make(chan struct{})
	c.manager.SetSender(c.enqueue)
	var closing sync.Once
	shutdown := func() {
		closing.Do(func() {
			close(c.done)
			c.manager.SetSender(nil)
			cancel()
		})
	}
	defer shutdown()

	senderDone := make(chan struct{})
	go func() {
		defer close(senderDone)
		for {
			select {
			case <-c.done:
				return
			case message := <-c.outgoing:
				if err := stream.Send(message); err != nil {
					c.log.Warn("agent stream send failed", "error", err)
					shutdown()
					return
				}
			}
		}
	}()

	heartbeatCtx, stopHeartbeat := context.WithCancel(ctx)
	defer stopHeartbeat()
	go c.heartbeatLoop(heartbeatCtx)

	semaphore := make(chan struct{}, 12)
	for {
		message, err := stream.Recv()
		if err != nil {
			shutdown()
			<-senderDone
			return err
		}
		switch payload := message.Payload.(type) {
		case *agentv1.ServerMessage_Welcome:
			continue
		case *agentv1.ServerMessage_Ping:
			c.log.Debug("panel ping received", "nonce", payload.Ping.Nonce)
			c.enqueue(&agentv1.AgentMessage{Payload: &agentv1.AgentMessage_Pong{Pong: &agentv1.Pong{Nonce: payload.Ping.Nonce}}})
		case *agentv1.ServerMessage_Command:
			command := payload.Command
			select {
			case semaphore <- struct{}{}:
			case <-c.done:
				return nil
			}
			go func() {
				defer func() { <-semaphore }()
				commandCtx, commandCancel := context.WithTimeout(context.Background(), 20*time.Minute)
				defer commandCancel()
				result := c.manager.HandleCommand(commandCtx, command)
				c.enqueue(&agentv1.AgentMessage{Payload: &agentv1.AgentMessage_Result{Result: result}})
			}()
		}
	}
}

func (c *Client) enqueue(message *agentv1.AgentMessage) bool {
	select {
	case <-c.done:
		return false
	default:
	}
	select {
	case c.outgoing <- message:
		return true
	case <-c.done:
		return false
	}
}

func (c *Client) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	startedAt := time.Now()
	for {
		c.sendHeartbeat(ctx, startedAt)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (c *Client) sendHeartbeat(ctx context.Context, startedAt time.Time) {
	collectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	host := CollectHost(c.manager.dataRoot)
	states := c.manager.CollectStates(collectCtx)
	dockerOK, dockerError, dockerVersion := c.checkDocker(collectCtx)
	heartbeat := &agentv1.Heartbeat{
		Host: &agentv1.HostStats{
			CpuPercent:       host.CPUPercent,
			MemoryUsedBytes:  host.MemoryUsed,
			MemoryTotalBytes: host.MemoryTotal,
			DiskUsedBytes:    host.DiskUsed,
			DiskTotalBytes:   host.DiskTotal,
			NetworkInBps:     host.NetworkInBPS,
			NetworkOutBps:    host.NetworkOutBPS,
		},
		Instances:     states,
		DockerOk:      dockerOK,
		DockerError:   dockerError,
		SentAt:        time.Now().UnixMilli(),
		UptimeSeconds: int64(time.Since(startedAt).Seconds()),
	}
	_ = dockerVersion
	c.enqueue(&agentv1.AgentMessage{Payload: &agentv1.AgentMessage_Heartbeat{Heartbeat: heartbeat}})
	go c.autoRestart(states)
}

func (c *Client) autoRestart(states []*agentv1.InstanceState) {
	if !c.autoMu.TryLock() {
		return
	}
	defer c.autoMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c.manager.AutoRestart(ctx, states)
}

func (c *Client) checkDocker(ctx context.Context) (bool, string, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.dockerChecked) < 5*time.Second && c.dockerOK {
		return c.dockerOK, c.dockerError, c.dockerVer
	}
	versionCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	version, err := c.docker.Version(versionCtx)
	c.dockerChecked = time.Now()
	if err != nil {
		c.dockerOK = false
		c.dockerError = err.Error()
		c.dockerVer = ""
		return c.dockerOK, c.dockerError, c.dockerVer
	}
	c.dockerOK = true
	c.dockerError = ""
	c.dockerVer = version
	return c.dockerOK, c.dockerError, c.dockerVer
}

func (c *Client) dockerVersion(ctx context.Context) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	versionCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	version, err := c.docker.Version(versionCtx)
	if err != nil {
		c.dockerOK = false
		c.dockerError = err.Error()
		return ""
	}
	c.dockerOK = true
	c.dockerVer = version
	c.dockerChecked = time.Now()
	return version
}

func receiveWelcome(stream agentv1.AgentService_ConnectClient, timeout time.Duration) (*agentv1.Welcome, error) {
	type outcome struct {
		message *agentv1.ServerMessage
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
			return nil, fmt.Errorf("панель отклонила подключение: %w", result.err)
		}
		if result.message.GetWelcome() == nil {
			return nil, errors.New("панель не отправила приветствие")
		}
		return result.message.GetWelcome(), nil
	case <-time.After(timeout):
		return nil, errors.New("панель не ответила вовремя")
	}
}

func parsePanelURL(value string) (target string, secure bool, err error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false, errors.New("не указан адрес панели")
	}
	if !strings.Contains(value, "://") {
		return value, false, nil
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", false, fmt.Errorf("некорректный адрес панели: %w", err)
	}
	if parsed.Host == "" {
		return "", false, errors.New("в адресе панели нет хоста")
	}
	return parsed.Host, parsed.Scheme == "https", nil
}

func hostname() string {
	name, err := os.Hostname()
	if err != nil || name == "" {
		return "dockium-agent"
	}
	return name
}
