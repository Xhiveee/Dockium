package agent

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	agentv1 "dockium/gen/agent/v1"
)

type inputSession struct {
	writer io.WriteCloser
}

type consoleManager struct {
	docker  *Docker
	log     *slog.Logger
	manager *Manager

	mu       sync.Mutex
	sessions map[string]context.CancelFunc
	inputs   map[int64]*inputSession
}

func newConsoleManager(docker *Docker, log *slog.Logger) *consoleManager {
	return &consoleManager{
		docker:   docker,
		log:      log,
		sessions: make(map[string]context.CancelFunc),
		inputs:   make(map[int64]*inputSession),
	}
}

func (c *consoleManager) Attach(streamID string, instanceID int64, spec *agentv1.InstanceSpec, tail uint32) {
	ctx, cancel := context.WithCancel(context.Background())
	c.mu.Lock()
	if previous := c.sessions[streamID]; previous != nil {
		previous()
	}
	c.sessions[streamID] = cancel
	c.mu.Unlock()
	go c.stream(ctx, streamID, spec, tail)
}

func (c *consoleManager) Detach(streamID string) {
	c.mu.Lock()
	cancel := c.sessions[streamID]
	delete(c.sessions, streamID)
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (c *consoleManager) stream(ctx context.Context, streamID string, spec *agentv1.InstanceSpec, tail uint32) {
	defer func() {
		c.mu.Lock()
		delete(c.sessions, streamID)
		c.mu.Unlock()
	}()
	var sequence atomic.Int64
	emit := func(kind int32, text string) {
		line := &agentv1.ConsoleLine{
			Seq:       sequence.Add(1),
			Kind:      kind,
			Text:      text,
			Timestamp: time.Now().UnixMilli(),
		}
		c.manager.send(&agentv1.AgentMessage{Payload: &agentv1.AgentMessage_Console{Console: &agentv1.ConsoleData{
			StreamId: streamID,
			Lines:    []*agentv1.ConsoleLine{line},
		}}})
	}
	emit(3, "Подключение к логам контейнера "+spec.ContainerName)
	command, output, err := c.docker.LogsCommand(ctx, spec.ContainerName, tail)
	if err != nil {
		emit(3, "Не удалось открыть логи: "+err.Error())
		c.done(streamID, err)
		return
	}
	lines := make(chan string, 256)
	go func() {
		defer close(lines)
		reader := bufio.NewReaderSize(output, 64*1024)
		for {
			chunk, readErr := reader.ReadBytes('\n')
			if len(chunk) > 0 {
				if len(chunk) > 256*1024 {
					chunk = chunk[:256*1024]
				}
				select {
				case lines <- decodeEncoding(chunk, spec.ConsoleEncoding):
				case <-ctx.Done():
					return
				}
			}
			if readErr != nil {
				return
			}
		}
	}()
	pending := make([]*agentv1.ConsoleLine, 0, 32)
	flush := func() {
		if len(pending) == 0 {
			return
		}
		batch := pending
		pending = make([]*agentv1.ConsoleLine, 0, 32)
		c.manager.send(&agentv1.AgentMessage{Payload: &agentv1.AgentMessage_Console{Console: &agentv1.ConsoleData{
			StreamId: streamID,
			Lines:    batch,
		}}})
	}
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case <-ticker.C:
			flush()
		case text, ok := <-lines:
			if !ok {
				flush()
				_ = command.Wait()
				c.done(streamID, nil)
				return
			}
			pending = append(pending, &agentv1.ConsoleLine{
				Seq:       sequence.Add(1),
				Kind:      1,
				Text:      text,
				Timestamp: time.Now().UnixMilli(),
			})
			if len(pending) >= 32 {
				flush()
			}
		}
	}
}

func (c *consoleManager) done(streamID string, err error) {
	message := ""
	if err != nil {
		message = err.Error()
	}
	c.manager.send(&agentv1.AgentMessage{Payload: &agentv1.AgentMessage_Console{Console: &agentv1.ConsoleData{
		StreamId: streamID,
		Done:     true,
		Error:    message,
	}}})
}

func (c *consoleManager) Input(instanceID int64, text string) error {
	spec := c.manager.spec(instanceID)
	if spec == nil {
		return errors.New("экземпляр не найден на агенте")
	}
	c.mu.Lock()
	session := c.inputs[instanceID]
	c.mu.Unlock()
	if session == nil {
		writer, err := c.docker.AttachInput(context.Background(), spec.ContainerName)
		if err != nil {
			return errors.New("не удалось подключиться к контейнеру: " + err.Error())
		}
		session = &inputSession{writer: writer}
		c.mu.Lock()
		c.inputs[instanceID] = session
		c.mu.Unlock()
	}
	payload := strings.TrimRight(text, "\r\n")
	if spec.Tty {
		payload += "\r"
	} else {
		payload += "\n"
	}
	if _, err := io.WriteString(session.writer, payload); err != nil {
		c.dropInput(instanceID)
		return errors.New("контейнер не принимает команды: " + err.Error())
	}
	return nil
}

func (c *consoleManager) StopInput(instanceID int64) {
	c.dropInput(instanceID)
}

func (c *consoleManager) dropInput(instanceID int64) {
	c.mu.Lock()
	session := c.inputs[instanceID]
	delete(c.inputs, instanceID)
	c.mu.Unlock()
	if session == nil {
		return
	}
	_ = session.writer.Close()
}

func (c *consoleManager) Shutdown() {
	c.mu.Lock()
	sessions := c.sessions
	inputs := c.inputs
	c.sessions = make(map[string]context.CancelFunc)
	c.inputs = make(map[int64]*inputSession)
	c.mu.Unlock()
	for _, cancel := range sessions {
		cancel()
	}
	for _, session := range inputs {
		_ = session.writer.Close()
	}
}
