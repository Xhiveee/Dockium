package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	agentv1 "dockium/gen/agent/v1"
)

type restartState struct {
	attempts int
	last     time.Time
}

type netSample struct {
	in  uint64
	out uint64
	at  time.Time
}

type Manager struct {
	dataRoot string
	docker   *Docker
	log      *slog.Logger

	mu           sync.RWMutex
	specs        map[int64]*agentv1.InstanceSpec
	expectedStop map[int64]bool
	restarts     map[int64]*restartState
	netSamples   map[int64]netSample

	senderMu sync.RWMutex
	sender   func(message *agentv1.AgentMessage) bool

	console *consoleManager
	files   *fileService
}

func NewManager(dataRoot string, docker *Docker, log *slog.Logger) (*Manager, error) {
	absolute, err := filepath.Abs(dataRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve data directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(absolute, "instances"), 0o755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	manager := &Manager{
		dataRoot:     absolute,
		docker:       docker,
		log:          log,
		specs:        make(map[int64]*agentv1.InstanceSpec),
		expectedStop: make(map[int64]bool),
		restarts:     make(map[int64]*restartState),
		netSamples:   make(map[int64]netSample),
	}
	manager.console = newConsoleManager(docker, log)
	manager.console.manager = manager
	manager.files = newFileService(manager)
	return manager, nil
}

func (m *Manager) SetSender(sender func(message *agentv1.AgentMessage) bool) {
	m.senderMu.Lock()
	m.sender = sender
	m.senderMu.Unlock()
}

func (m *Manager) send(message *agentv1.AgentMessage) bool {
	m.senderMu.RLock()
	sender := m.sender
	m.senderMu.RUnlock()
	if sender == nil {
		return false
	}
	return sender(message)
}

func (m *Manager) InstanceDirectory(id int64) string {
	return filepath.Join(m.dataRoot, "instances", strconv.FormatInt(id, 10), "data")
}

func (m *Manager) spec(id int64) *agentv1.InstanceSpec {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.specs[id]
}

func (m *Manager) HandleCommand(ctx context.Context, command *agentv1.Command) *agentv1.CommandResult {
	switch payload := command.Payload.(type) {
	case *agentv1.Command_Sync:
		return m.handleSync(ctx, command.Id, payload.Sync)
	case *agentv1.Command_Ensure:
		state, err := m.Ensure(ctx, payload.Ensure.Spec, false)
		return instanceResult(command.Id, state, err)
	case *agentv1.Command_Start:
		state, err := m.Start(ctx, payload.Start.Id)
		return instanceResult(command.Id, state, err)
	case *agentv1.Command_Stop:
		state, err := m.Stop(ctx, payload.Stop.Id, time.Duration(payload.Stop.TimeoutSeconds)*time.Second, payload.Stop.Force)
		return instanceResult(command.Id, state, err)
	case *agentv1.Command_Restart:
		state, err := m.Restart(ctx, payload.Restart.Id)
		return instanceResult(command.Id, state, err)
	case *agentv1.Command_Remove:
		err := m.Remove(ctx, payload.Remove.Id, payload.Remove.RemoveData)
		return result(command.Id, &agentv1.CommandResult_Empty{Empty: &agentv1.Empty{}}, err)
	case *agentv1.Command_ListImages:
		images, err := m.docker.ListImages(ctx)
		if err != nil {
			return result(command.Id, nil, err)
		}
		return result(command.Id, &agentv1.CommandResult_Images{Images: &agentv1.ImageList{Items: images}}, nil)
	case *agentv1.Command_ListContainers:
		containers, err := m.docker.ListContainers(ctx)
		if err != nil {
			return result(command.Id, nil, err)
		}
		return result(command.Id, &agentv1.CommandResult_Containers{Containers: &agentv1.ContainerList{Items: containers}}, nil)
	case *agentv1.Command_Inspect:
		payloadJSON, err := m.docker.Inspect(ctx, payload.Inspect.Kind, payload.Inspect.Id)
		if err != nil {
			return result(command.Id, nil, err)
		}
		return result(command.Id, &agentv1.CommandResult_Inspect{Inspect: &agentv1.InspectResponse{Json: payloadJSON}}, nil)
	case *agentv1.Command_RemoveImage:
		err := m.docker.RemoveImage(ctx, payload.RemoveImage.Image)
		return result(command.Id, &agentv1.CommandResult_Empty{Empty: &agentv1.Empty{}}, err)
	case *agentv1.Command_PullImage:
		return m.handlePull(ctx, command.Id, payload.PullImage)
	case *agentv1.Command_BuildImage:
		return m.handleBuild(ctx, command.Id, payload.BuildImage)
	case *agentv1.Command_ListFiles:
		return m.files.handleList(command.Id, payload.ListFiles)
	case *agentv1.Command_ReadFile:
		return m.files.handleRead(command.Id, payload.ReadFile)
	case *agentv1.Command_WriteFile:
		return m.files.handleWrite(command.Id, payload.WriteFile)
	case *agentv1.Command_Mkdir:
		return m.files.handleMkdir(command.Id, payload.Mkdir)
	case *agentv1.Command_DeletePath:
		return m.files.handleDelete(command.Id, payload.DeletePath)
	case *agentv1.Command_RenamePath:
		return m.files.handleRename(command.Id, payload.RenamePath)
	case *agentv1.Command_UploadFile:
		return m.files.handleUpload(command.Id, payload.UploadFile)
	case *agentv1.Command_DownloadFile:
		return m.files.handleDownload(command.Id, payload.DownloadFile)
	case *agentv1.Command_Chmod:
		return m.files.handleChmod(command.Id, payload.Chmod)
	case *agentv1.Command_Compress:
		return m.files.handleCompress(command.Id, payload.Compress)
	case *agentv1.Command_ConsoleAttach:
		return m.handleAttach(command.Id, payload.ConsoleAttach)
	case *agentv1.Command_ConsoleDetach:
		m.console.Detach(payload.ConsoleDetach.StreamId)
		return result(command.Id, &agentv1.CommandResult_Empty{Empty: &agentv1.Empty{}}, nil)
	case *agentv1.Command_ConsoleInput:
		err := m.console.Input(payload.ConsoleInput.InstanceId, payload.ConsoleInput.Text)
		return result(command.Id, &agentv1.CommandResult_Empty{Empty: &agentv1.Empty{}}, err)
	default:
		return result(command.Id, nil, errors.New("неизвестная команда агента"))
	}
}

func instanceResult(id int64, state *agentv1.InstanceState, err error) *agentv1.CommandResult {
	if err != nil {
		return result(id, nil, err)
	}
	return &agentv1.CommandResult{Id: id, Payload: &agentv1.CommandResult_Instance{Instance: state}}
}

func result(id int64, payload any, err error) *agentv1.CommandResult {
	out := &agentv1.CommandResult{Id: id}
	if err != nil {
		out.Error = err.Error()
		return out
	}
	switch value := payload.(type) {
	case *agentv1.CommandResult_Empty:
		out.Payload = value
	case *agentv1.CommandResult_Images:
		out.Payload = value
	case *agentv1.CommandResult_Containers:
		out.Payload = value
	case *agentv1.CommandResult_Inspect:
		out.Payload = value
	case *agentv1.CommandResult_Files:
		out.Payload = value
	case *agentv1.CommandResult_File:
		out.Payload = value
	case *agentv1.CommandResult_Path:
		out.Payload = value
	case nil:
		out.Payload = &agentv1.CommandResult_Empty{Empty: &agentv1.Empty{}}
	}
	return out
}

func (m *Manager) handleSync(ctx context.Context, id int64, sync *agentv1.SyncInstances) *agentv1.CommandResult {
	keep := make(map[int64]bool, len(sync.Specs))
	m.mu.Lock()
	m.specs = make(map[int64]*agentv1.InstanceSpec, len(sync.Specs))
	m.mu.Unlock()
	for _, spec := range sync.Specs {
		keep[spec.Id] = true
		m.mu.Lock()
		m.specs[spec.Id] = spec
		m.mu.Unlock()
	}
	var firstError error
	for _, spec := range sync.Specs {
		state, err := m.Ensure(ctx, spec, true)
		if err != nil {
			m.log.Warn("ensure instance on sync failed", "instance_id", spec.Id, "error", err)
			if firstError == nil {
				firstError = fmt.Errorf("экземпляр %s: %w", spec.Name, err)
			}
			continue
		}
		if spec.AutoStart && state.Status != agentv1.InstanceStatus_INSTANCE_STATUS_RUNNING {
			if _, err := m.Start(ctx, spec.Id); err != nil && firstError == nil {
				firstError = fmt.Errorf("автозапуск %s: %w", spec.Name, err)
			}
		}
	}
	if containers, err := m.docker.ListContainers(ctx); err == nil {
		for _, container := range containers {
			if !container.Managed || container.InstanceId == 0 || keep[container.InstanceId] {
				continue
			}
			if err := m.docker.Remove(ctx, container.Name); err != nil {
				m.log.Warn("prune orphan container", "container", container.Name, "error", err)
			}
		}
	}
	return result(id, &agentv1.CommandResult_Empty{Empty: &agentv1.Empty{}}, firstError)
}

func (m *Manager) Ensure(ctx context.Context, spec *agentv1.InstanceSpec, allowRecreate bool) (*agentv1.InstanceState, error) {
	if spec == nil || spec.Id == 0 || spec.Image == "" || spec.ContainerName == "" {
		return nil, errors.New("некорректная конфигурация экземпляра")
	}
	m.mu.Lock()
	previous := m.specs[spec.Id]
	m.specs[spec.Id] = spec
	m.mu.Unlock()
	_ = previous
	directory := m.InstanceDirectory(spec.Id)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return nil, fmt.Errorf("не удалось создать каталог данных: %w", err)
	}
	hash := specHash(spec)
	exists := m.docker.ContainerExists(ctx, spec.ContainerName)
	if exists {
		currentHash := m.docker.ContainerLabel(ctx, spec.ContainerName, "dockium.spec")
		if currentHash == hash {
			return m.State(ctx, spec.Id)
		}
		states, _ := m.docker.ContainerStates(ctx, []string{spec.ContainerName})
		running := states[spec.ContainerName].Status == "running"
		if running && !allowRecreate {
			return nil, errors.New("контейнер запущен: остановите экземпляр перед изменением конфигурации")
		}
		stopCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		_ = m.docker.Stop(stopCtx, spec.ContainerName, 10*time.Second, false)
		cancel()
		if err := m.docker.Remove(ctx, spec.ContainerName); err != nil {
			return nil, fmt.Errorf("не удалось удалить старый контейнер: %w", err)
		}
	}
	if !m.docker.ImageExists(ctx, spec.Image) {
		pullCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
		defer cancel()
		if err := m.docker.Pull(pullCtx, spec.Image, func(string) {}); err != nil {
			return nil, fmt.Errorf("не удалось загрузить образ %s: %w", spec.Image, err)
		}
	}
	if err := m.docker.Create(ctx, spec, directory, hash); err != nil {
		return nil, fmt.Errorf("не удалось создать контейнер: %w", err)
	}
	return m.State(ctx, spec.Id)
}

func (m *Manager) Start(ctx context.Context, id int64) (*agentv1.InstanceState, error) {
	spec := m.spec(id)
	if spec == nil {
		return nil, errors.New("экземпляр ещё не синхронизирован с агентом")
	}
	m.mu.Lock()
	m.expectedStop[id] = false
	m.mu.Unlock()
	if _, err := m.Ensure(ctx, spec, false); err != nil {
		return nil, err
	}
	if err := m.docker.Start(ctx, spec.ContainerName); err != nil {
		return nil, fmt.Errorf("не удалось запустить контейнер: %w", err)
	}
	time.Sleep(150 * time.Millisecond)
	return m.State(ctx, id)
}

func (m *Manager) Stop(ctx context.Context, id int64, timeout time.Duration, force bool) (*agentv1.InstanceState, error) {
	spec := m.spec(id)
	if spec == nil {
		return nil, errors.New("экземпляр не найден на агенте")
	}
	m.mu.Lock()
	m.expectedStop[id] = true
	m.mu.Unlock()
	m.console.StopInput(id)
	if err := m.docker.Stop(ctx, spec.ContainerName, timeout, force); err != nil {
		return nil, fmt.Errorf("не удалось остановить контейнер: %w", err)
	}
	return m.State(ctx, id)
}

func (m *Manager) Restart(ctx context.Context, id int64) (*agentv1.InstanceState, error) {
	spec := m.spec(id)
	if spec == nil {
		return nil, errors.New("экземпляр не найден на агенте")
	}
	m.mu.Lock()
	m.expectedStop[id] = false
	m.mu.Unlock()
	m.console.StopInput(id)
	if _, err := m.Ensure(ctx, spec, false); err != nil {
		return nil, err
	}
	if err := m.docker.Restart(ctx, spec.ContainerName, 10*time.Second); err != nil {
		return nil, fmt.Errorf("не удалось перезапустить контейнер: %w", err)
	}
	time.Sleep(150 * time.Millisecond)
	return m.State(ctx, id)
}

func (m *Manager) Remove(ctx context.Context, id int64, removeData bool) error {
	spec := m.spec(id)
	name := ""
	if spec != nil {
		name = spec.ContainerName
	}
	m.mu.Lock()
	if name == "" {
		if previous, ok := m.specs[id]; ok {
			name = previous.ContainerName
		}
	}
	m.expectedStop[id] = true
	delete(m.restarts, id)
	delete(m.netSamples, id)
	m.mu.Unlock()
	m.console.StopInput(id)
	if name != "" {
		if err := m.docker.Remove(ctx, name); err != nil {
			m.log.Warn("remove container", "instance_id", id, "error", err)
		}
	}
	if removeData {
		if err := os.RemoveAll(filepath.Join(m.dataRoot, "instances", strconv.FormatInt(id, 10))); err != nil {
			return fmt.Errorf("не удалось удалить данные экземпляра: %w", err)
		}
	}
	m.mu.Lock()
	delete(m.specs, id)
	delete(m.expectedStop, id)
	m.mu.Unlock()
	return nil
}

func (m *Manager) CollectStates(ctx context.Context) []*agentv1.InstanceState {
	m.mu.RLock()
	specs := make([]*agentv1.InstanceSpec, 0, len(m.specs))
	for _, spec := range m.specs {
		specs = append(specs, spec)
	}
	m.mu.RUnlock()
	if len(specs) == 0 {
		return nil
	}
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		names = append(names, spec.ContainerName)
	}
	states, err := m.docker.ContainerStates(ctx, names)
	if err != nil {
		m.log.Warn("collect container states", "error", err)
	}
	stats, err := m.docker.Stats(ctx)
	if err != nil {
		stats = nil
	}
	result := make([]*agentv1.InstanceState, 0, len(specs))
	for _, spec := range specs {
		state := states[spec.ContainerName]
		entry := &agentv1.InstanceState{
			Id:          spec.Id,
			Status:      statusFromDocker(state.Status),
			ExitCode:    state.ExitCode,
			Error:       state.Error,
			StartedAt:   state.StartedAt,
			FinishedAt:  state.FinishedAt,
			ContainerId: state.ID,
		}
		if entry.Status == agentv1.InstanceStatus_INSTANCE_STATUS_UNSPECIFIED {
			entry.Status = agentv1.InstanceStatus_INSTANCE_STATUS_STOPPED
		}
		if usage, ok := stats[spec.ContainerName]; ok {
			entry.CpuPercent = usage.CPUPercent
			entry.MemoryBytes = usage.MemUsage
			entry.MemoryLimitBytes = usage.MemLimit
			if entry.MemoryLimitBytes == 0 && spec.MemoryMb > 0 {
				entry.MemoryLimitBytes = uint64(spec.MemoryMb) * 1024 * 1024
			}
			entry.NetworkInBps, entry.NetworkOutBps = m.networkRates(spec.Id, usage.NetIn, usage.NetOut)
		}
		result = append(result, entry)
	}
	return result
}

func (m *Manager) networkRates(id int64, in, out uint64) (uint64, uint64) {
	now := time.Now()
	m.mu.Lock()
	previous, ok := m.netSamples[id]
	m.netSamples[id] = netSample{in: in, out: out, at: now}
	m.mu.Unlock()
	if !ok || now.Sub(previous.at) <= 0 {
		return 0, 0
	}
	seconds := now.Sub(previous.at).Seconds()
	var inRate, outRate uint64
	if in >= previous.in {
		inRate = uint64(float64(in-previous.in) / seconds)
	}
	if out >= previous.out {
		outRate = uint64(float64(out-previous.out) / seconds)
	}
	return inRate, outRate
}

func (m *Manager) State(ctx context.Context, id int64) (*agentv1.InstanceState, error) {
	spec := m.spec(id)
	if spec == nil {
		return nil, errors.New("экземпляр не найден на агенте")
	}
	states, _ := m.docker.ContainerStates(ctx, []string{spec.ContainerName})
	state := states[spec.ContainerName]
	entry := &agentv1.InstanceState{
		Id:          id,
		Status:      statusFromDocker(state.Status),
		ExitCode:    state.ExitCode,
		Error:       state.Error,
		StartedAt:   state.StartedAt,
		FinishedAt:  state.FinishedAt,
		ContainerId: state.ID,
	}
	if entry.Status == agentv1.InstanceStatus_INSTANCE_STATUS_UNSPECIFIED {
		entry.Status = agentv1.InstanceStatus_INSTANCE_STATUS_STOPPED
	}
	if stats, err := m.docker.Stats(ctx); err == nil {
		if usage, ok := stats[spec.ContainerName]; ok {
			entry.CpuPercent = usage.CPUPercent
			entry.MemoryBytes = usage.MemUsage
			entry.MemoryLimitBytes = usage.MemLimit
			entry.NetworkInBps, entry.NetworkOutBps = m.networkRates(id, usage.NetIn, usage.NetOut)
		}
	}
	return entry, nil
}

func (m *Manager) AutoRestart(ctx context.Context, states []*agentv1.InstanceState) {
	now := time.Now()
	for _, state := range states {
		m.mu.Lock()
		spec := m.specs[state.Id]
		expected := m.expectedStop[state.Id]
		restart := m.restarts[state.Id]
		m.mu.Unlock()
		if spec == nil || !spec.AutoRestart {
			continue
		}
		if state.Status == agentv1.InstanceStatus_INSTANCE_STATUS_RUNNING {
			if state.StartedAt > 0 && now.UnixMilli()-state.StartedAt > 60_000 {
				m.mu.Lock()
				delete(m.restarts, state.Id)
				m.mu.Unlock()
			}
			continue
		}
		if state.Status != agentv1.InstanceStatus_INSTANCE_STATUS_EXITED || expected {
			continue
		}
		if restart == nil {
			restart = &restartState{}
			m.mu.Lock()
			m.restarts[state.Id] = restart
			m.mu.Unlock()
		}
		delay := time.Duration(5*(1<<min(restart.attempts, 4))) * time.Second
		if delay > 60*time.Second {
			delay = 60 * time.Second
		}
		if !restart.last.IsZero() && now.Sub(restart.last) < delay {
			continue
		}
		m.mu.Lock()
		restart.last = now
		restart.attempts++
		m.mu.Unlock()
		go func(id int64) {
			restartCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			if _, err := m.Start(restartCtx, id); err != nil {
				m.log.Warn("auto restart failed", "instance_id", id, "error", err)
			} else {
				m.log.Info("instance auto restarted", "instance_id", id)
			}
		}(state.Id)
	}
}

func statusFromDocker(status string) agentv1.InstanceStatus {
	switch status {
	case "running":
		return agentv1.InstanceStatus_INSTANCE_STATUS_RUNNING
	case "restarting":
		return agentv1.InstanceStatus_INSTANCE_STATUS_STARTING
	case "paused":
		return agentv1.InstanceStatus_INSTANCE_STATUS_RUNNING
	case "created":
		return agentv1.InstanceStatus_INSTANCE_STATUS_STOPPED
	case "exited":
		return agentv1.InstanceStatus_INSTANCE_STATUS_EXITED
	case "removing":
		return agentv1.InstanceStatus_INSTANCE_STATUS_STOPPING
	case "dead":
		return agentv1.InstanceStatus_INSTANCE_STATUS_FAILED
	default:
		return agentv1.InstanceStatus_INSTANCE_STATUS_STOPPED
	}
}

func (m *Manager) handlePull(ctx context.Context, id int64, pull *agentv1.PullImage) *agentv1.CommandResult {
	err := m.docker.Pull(ctx, pull.Image, func(line string) {
		m.send(&agentv1.AgentMessage{Payload: &agentv1.AgentMessage_Stream{Stream: &agentv1.StreamEvent{
			StreamId: pull.StreamId, Line: line,
		}}})
	})
	m.send(&agentv1.AgentMessage{Payload: &agentv1.AgentMessage_Stream{Stream: &agentv1.StreamEvent{
		StreamId: pull.StreamId, Done: true, Error: errorText(err),
	}}})
	return result(id, &agentv1.CommandResult_Empty{Empty: &agentv1.Empty{}}, err)
}

func (m *Manager) handleBuild(ctx context.Context, id int64, build *agentv1.BuildImage) *agentv1.CommandResult {
	err := m.docker.Build(ctx, build.Tag, build.Dockerfile, func(line string) {
		m.send(&agentv1.AgentMessage{Payload: &agentv1.AgentMessage_Stream{Stream: &agentv1.StreamEvent{
			StreamId: build.StreamId, Line: line,
		}}})
	})
	m.send(&agentv1.AgentMessage{Payload: &agentv1.AgentMessage_Stream{Stream: &agentv1.StreamEvent{
		StreamId: build.StreamId, Done: true, Error: errorText(err),
	}}})
	return result(id, &agentv1.CommandResult_Empty{Empty: &agentv1.Empty{}}, err)
}

func (m *Manager) handleAttach(id int64, attach *agentv1.ConsoleAttach) *agentv1.CommandResult {
	spec := m.spec(attach.InstanceId)
	if spec == nil {
		return result(id, nil, errors.New("экземпляр не найден на агенте"))
	}
	m.console.Attach(attach.StreamId, attach.InstanceId, spec, attach.TailLines)
	return result(id, &agentv1.CommandResult_Empty{Empty: &agentv1.Empty{}}, nil)
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
