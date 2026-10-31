package panelapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	emptypb "google.golang.org/protobuf/types/known/emptypb"

	panelv1 "dockium/gen/panel/v1"
	"dockium/internal/db"
	"dockium/internal/hub"
	"dockium/internal/present"
	"dockium/internal/realtime"
)

func (s *Server) ListInstances(ctx context.Context, _ *connect.Request[emptypb.Empty]) (*connect.Response[panelv1.InstanceList], error) {
	state, err := s.requireUser(ctx)
	if err != nil {
		return nil, err
	}
	instances, permissions, err := s.visibleInstancesWithPermissions(ctx, state.user)
	if err != nil {
		return nil, rpcError(err, "не удалось получить экземпляры")
	}
	items := make([]*panelv1.Instance, 0, len(instances))
	for _, instance := range instances {
		items = append(items, present.Instance(instance, permissions[instance.ID]))
	}
	return connect.NewResponse(&panelv1.InstanceList{Items: items}), nil
}

func (s *Server) GetInstance(ctx context.Context, request *connect.Request[panelv1.InstanceIdRequest]) (*connect.Response[panelv1.Instance], error) {
	instance, permission, err := s.instanceFor(ctx, request.Msg.Id, nil, "")
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(present.Instance(instance, permission)), nil
}

func (s *Server) WatchInstances(ctx context.Context, _ *connect.Request[emptypb.Empty], stream *connect.ServerStream[panelv1.InstanceEvent]) error {
	state, err := s.requireUser(ctx)
	if err != nil {
		return err
	}
	instances, permissions, err := s.visibleInstancesWithPermissions(ctx, state.user)
	if err != nil {
		return rpcError(err, "не удалось получить экземпляры")
	}
	allowed := make(map[int64]db.Permission, len(permissions))
	for id, permission := range permissions {
		allowed[id] = permission
	}
	events, unsubscribe := s.publisher.SubscribeInstances(128)
	defer unsubscribe()
	snapshot := make([]*panelv1.Instance, 0, len(instances))
	for _, instance := range instances {
		snapshot = append(snapshot, present.Instance(instance, permissions[instance.ID]))
	}
	if err := stream.Send(&panelv1.InstanceEvent{Payload: &panelv1.InstanceEvent_Snapshot{Snapshot: &panelv1.InstanceSnapshot{Items: snapshot}}}); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-events:
			if !ok {
				return nil
			}
			payload := &panelv1.InstanceEvent{}
			switch {
			case event.RemovedID > 0:
				if _, visible := allowed[event.RemovedID]; !visible && state.user.Role != db.RoleAdmin {
					continue
				}
				payload.Payload = &panelv1.InstanceEvent_Removed{Removed: &panelv1.InstanceRemoved{Id: event.RemovedID}}
			case event.Snapshot != nil:
				items := make([]*panelv1.Instance, 0, len(event.Snapshot))
				for _, instance := range event.Snapshot {
					permission, visible := allowed[instance.ID]
					if state.user.Role == db.RoleAdmin {
						permission = db.Permission{Control: true, Console: true, Files: true}
						visible = true
					} else if !visible {
						continue
					}
					items = append(items, present.Instance(instance, permission))
				}
				payload.Payload = &panelv1.InstanceEvent_Snapshot{Snapshot: &panelv1.InstanceSnapshot{Items: items}}
			case event.Instance != nil:
				permission, visible := allowed[event.Instance.ID]
				if state.user.Role == db.RoleAdmin {
					permission = db.Permission{Control: true, Console: true, Files: true}
					visible = true
				}
				if !visible {
					continue
				}
				payload.Payload = &panelv1.InstanceEvent_Instance{Instance: present.Instance(event.Instance, permission)}
			default:
				continue
			}
			if err := stream.Send(payload); err != nil {
				return err
			}
		}
	}
}

func (s *Server) CreateInstance(ctx context.Context, request *connect.Request[panelv1.CreateInstanceRequest]) (*connect.Response[panelv1.Instance], error) {
	if _, err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	message := request.Msg
	if err := validateInstanceInput(message.Name, message.Image, message.ContainerName, message.NetworkMode,
		message.RestartPolicy, message.DataMount, message.ConsoleEncoding, message.Command, message.Environment,
		message.Ports, message.Volumes, message.MemoryMb, message.Cpus); err != nil {
		return nil, invalid(err.Error())
	}
	agent, err := s.store.AgentByID(ctx, message.AgentId)
	if err != nil {
		return nil, invalid("агент не найден")
	}
	instance := instanceFromCreate(message)
	taken, err := s.store.InstanceNameTaken(ctx, instance.Name, 0)
	if err != nil {
		return nil, rpcError(err, "не удалось проверить имя")
	}
	if taken {
		return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("экземпляр с таким названием уже существует"))
	}
	if err := s.checkContainerConflicts(ctx, instance.AgentID, 0, instance.ContainerName, instance.Ports); err != nil {
		return nil, err
	}
	created, err := s.store.CreateInstance(ctx, instance)
	if err != nil {
		if errors.Is(err, db.ErrConflict) {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("экземпляр с таким названием уже существует"))
		}
		return nil, rpcError(err, "не удалось создать экземпляр")
	}
	s.publishInstance(created)
	if s.hub.Online(created.AgentID) {
		created, err = s.control.SetStatus(ctx, created, db.StatusStarting, "", 0)
		if err != nil {
			return nil, rpcError(err, "не удалось создать экземпляр")
		}
		go s.prepareInstance(created.ID, created.AutoStart)
	} else {
		updated, err := s.control.SetStatus(ctx, created, db.StatusStopped, "Агент офлайн: контейнер будет создан при подключении", 0)
		if err == nil {
			created = updated
		}
	}
	permission := db.Permission{Control: true, Console: true, Files: true}
	_ = agent
	return connect.NewResponse(present.Instance(created, permission)), nil
}

func (s *Server) UpdateInstance(ctx context.Context, request *connect.Request[panelv1.UpdateInstanceRequest]) (*connect.Response[panelv1.Instance], error) {
	if _, err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	message := request.Msg
	existing, err := s.store.InstanceByID(ctx, message.Id)
	if err != nil {
		return nil, rpcError(err, "экземпляр не найден")
	}
	if err := validateInstanceInput(message.Name, message.Image, message.ContainerName, message.NetworkMode,
		message.RestartPolicy, message.DataMount, message.ConsoleEncoding, message.Command, message.Environment,
		message.Ports, message.Volumes, message.MemoryMb, message.Cpus); err != nil {
		return nil, invalid(err.Error())
	}
	taken, err := s.store.InstanceNameTaken(ctx, strings.TrimSpace(message.Name), existing.ID)
	if err != nil {
		return nil, rpcError(err, "не удалось проверить имя")
	}
	if taken {
		return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("экземпляр с таким названием уже существует"))
	}
	wasRunning := existing.Status == db.StatusRunning || existing.Status == db.StatusStarting ||
		existing.Status == db.StatusStopping
	applyUpdate(existing, message)
	if err := s.checkContainerConflicts(ctx, existing.AgentID, existing.ID, existing.ContainerName, existing.Ports); err != nil {
		return nil, err
	}
	if err := s.store.UpdateInstance(ctx, existing); err != nil {
		return nil, rpcError(err, "не удалось сохранить экземпляр")
	}
	s.publishInstance(existing)
	if s.hub.Online(existing.AgentID) {
		updated, err := s.control.SetStatus(ctx, existing, db.StatusStarting, "", existing.ExitCode)
		if err == nil {
			existing = updated
		}
		go s.reconcileInstance(existing.ID, wasRunning || existing.AutoStart)
	} else {
		updated, err := s.control.SetStatus(ctx, existing, db.StatusStopped, "Агент офлайн: изменения применятся при подключении", 0)
		if err == nil {
			existing = updated
		}
	}
	permission := db.Permission{Control: true, Console: true, Files: true}
	return connect.NewResponse(present.Instance(existing, permission)), nil
}

func (s *Server) DeleteInstance(ctx context.Context, request *connect.Request[panelv1.DeleteInstanceRequest]) (*connect.Response[emptypb.Empty], error) {
	if _, err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	instance, err := s.store.InstanceByID(ctx, request.Msg.Id)
	if err != nil {
		return nil, rpcError(err, "экземпляр не найден")
	}
	if s.hub.Online(instance.AgentID) {
		removeCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		err := s.control.RemoveFromAgent(removeCtx, instance, request.Msg.RemoveData)
		cancel()
		if err != nil {
			return nil, rpcError(err, "не удалось удалить контейнер")
		}
	}
	if err := s.store.DeleteInstance(ctx, instance.ID); err != nil {
		return nil, rpcError(err, "не удалось удалить экземпляр")
	}
	s.publisher.InstanceRemoved(instance.ID)
	return emptyResponse(), nil
}

func (s *Server) StartInstance(ctx context.Context, request *connect.Request[panelv1.InstanceIdRequest]) (*connect.Response[panelv1.Instance], error) {
	return s.instanceAction(ctx, request.Msg.Id, "start")
}

func (s *Server) StopInstance(ctx context.Context, request *connect.Request[panelv1.InstanceIdRequest]) (*connect.Response[panelv1.Instance], error) {
	return s.instanceAction(ctx, request.Msg.Id, "stop")
}

func (s *Server) RestartInstance(ctx context.Context, request *connect.Request[panelv1.InstanceIdRequest]) (*connect.Response[panelv1.Instance], error) {
	return s.instanceAction(ctx, request.Msg.Id, "restart")
}

func (s *Server) KillInstance(ctx context.Context, request *connect.Request[panelv1.InstanceIdRequest]) (*connect.Response[panelv1.Instance], error) {
	return s.instanceAction(ctx, request.Msg.Id, "kill")
}

func (s *Server) instanceAction(ctx context.Context, id int64, action string) (*connect.Response[panelv1.Instance], error) {
	instance, permission, err := s.instanceFor(ctx, id, func(permission db.Permission) bool { return permission.Control }, "нет прав на управление экземпляром")
	if err != nil {
		return nil, err
	}
	var updated *db.Instance
	switch action {
	case "start":
		updated, err = s.control.Start(ctx, instance)
	case "stop":
		updated, err = s.control.Stop(ctx, instance, false)
	case "restart":
		updated, err = s.control.Restart(ctx, instance)
	case "kill":
		updated, err = s.control.Stop(ctx, instance, true)
	default:
		return nil, invalid("неизвестное действие")
	}
	if err != nil {
		if updated == nil {
			return nil, rpcError(err, "не удалось выполнить действие")
		}
		return nil, rpcError(err, "не удалось выполнить действие")
	}
	if updated == nil {
		updated, err = s.store.InstanceByID(ctx, id)
		if err != nil {
			return nil, rpcError(err, "экземпляр не найден")
		}
	}
	return connect.NewResponse(present.Instance(updated, permission)), nil
}

func (s *Server) SetAutomation(ctx context.Context, request *connect.Request[panelv1.SetAutomationRequest]) (*connect.Response[panelv1.Instance], error) {
	if _, err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	instance, err := s.store.InstanceByID(ctx, request.Msg.Id)
	if err != nil {
		return nil, rpcError(err, "экземпляр не найден")
	}
	if err := s.store.SetInstanceAutomation(ctx, instance.ID, request.Msg.AutoStart, request.Msg.AutoRestart); err != nil {
		return nil, rpcError(err, "не удалось сохранить автоматизацию")
	}
	instance, err = s.store.InstanceByID(ctx, instance.ID)
	if err != nil {
		return nil, rpcError(err, "экземпляр не найден")
	}
	s.publishInstance(instance)
	if s.hub.Online(instance.AgentID) {
		go func(instance *db.Instance) {
			ensureCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			if _, err := s.control.Ensure(ensureCtx, instance); err != nil {
				s.log.Warn("sync automation with agent", "instance_id", instance.ID, "error", err)
			}
		}(instance)
	}
	permission := db.Permission{Control: true, Console: true, Files: true}
	return connect.NewResponse(present.Instance(instance, permission)), nil
}

func (s *Server) prepareInstance(id int64, start bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	instance, err := s.store.InstanceByID(ctx, id)
	if err != nil {
		return
	}
	if !s.hub.Online(instance.AgentID) {
		_, _ = s.control.SetStatus(ctx, instance, db.StatusStopped, "Агент офлайн: контейнер будет создан при подключении", 0)
		return
	}
	updated, err := s.control.Ensure(ctx, instance)
	if err != nil {
		s.log.Warn("prepare instance", "instance_id", id, "error", err)
		_, _ = s.control.SetStatus(ctx, instance, db.StatusFailed, err.Error(), instance.ExitCode)
		return
	}
	if start && updated != nil {
		if _, err := s.control.Start(ctx, updated); err != nil {
			s.log.Warn("start instance after prepare", "instance_id", id, "error", err)
			_, _ = s.control.SetStatus(ctx, updated, db.StatusFailed, err.Error(), updated.ExitCode)
		}
	}
}

func (s *Server) reconcileInstance(id int64, start bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	instance, err := s.store.InstanceByID(ctx, id)
	if err != nil {
		return
	}
	if !s.hub.Online(instance.AgentID) {
		_, _ = s.control.SetStatus(ctx, instance, db.StatusStopped, "Агент офлайн: изменения применятся при подключении", 0)
		return
	}
	if instance.Status == db.StatusRunning || instance.Status == db.StatusStarting || instance.Status == db.StatusStopping {
		if _, err := s.control.Stop(ctx, instance, false); err != nil && !errors.Is(err, hub.ErrOffline) {
			s.log.Warn("stop before update", "instance_id", id, "error", err)
		}
		instance, _ = s.store.InstanceByID(ctx, id)
	}
	updated, err := s.control.Ensure(ctx, instance)
	if err != nil {
		s.log.Warn("reconcile instance", "instance_id", id, "error", err)
		_, _ = s.control.SetStatus(ctx, instance, db.StatusFailed, err.Error(), instance.ExitCode)
		return
	}
	if start && updated != nil {
		if _, err := s.control.Start(ctx, updated); err != nil {
			s.log.Warn("start after reconcile", "instance_id", id, "error", err)
			_, _ = s.control.SetStatus(ctx, updated, db.StatusFailed, err.Error(), updated.ExitCode)
		}
	}
}

func (s *Server) publishInstance(instance *db.Instance) {
	if instance == nil {
		return
	}
	s.publisher.InstanceUpdated(instance)
}

func (s *Server) visibleInstancesWithPermissions(ctx context.Context, user *db.User) ([]*db.Instance, map[int64]db.Permission, error) {
	if user.Role == db.RoleAdmin {
		instances, err := s.store.ListInstances(ctx)
		if err != nil {
			return nil, nil, err
		}
		permissions := make(map[int64]db.Permission, len(instances))
		for _, instance := range instances {
			permissions[instance.ID] = db.Permission{Control: true, Console: true, Files: true}
		}
		return instances, permissions, nil
	}
	instances, err := s.store.ListUserInstances(ctx, user.ID)
	if err != nil {
		return nil, nil, err
	}
	access, err := s.store.ListAccess(ctx, user.ID)
	if err != nil {
		return nil, nil, err
	}
	permissions := make(map[int64]db.Permission, len(access))
	for _, item := range access {
		permissions[item.InstanceID] = db.Permission{Control: item.Control, Console: item.Console, Files: item.Files}
	}
	return instances, permissions, nil
}

func (s *Server) checkContainerConflicts(ctx context.Context, agentID, instanceID int64, containerName string, ports []db.PortMapping) error {
	if !containerNamePattern.MatchString(containerName) {
		return invalid("имя контейнера: латиница, цифры, дефис, точка и подчёркивание")
	}
	instances, err := s.store.ListInstancesByAgent(ctx, agentID)
	if err != nil {
		return rpcError(err, "не удалось проверить конфликты")
	}
	for _, existing := range instances {
		if existing.ID == instanceID {
			continue
		}
		if strings.EqualFold(existing.ContainerName, containerName) {
			return connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("имя контейнера %s уже используется", containerName))
		}
		for _, port := range ports {
			for _, taken := range existing.Ports {
				if port.HostPort == taken.HostPort && port.Protocol == taken.Protocol {
					return connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("порт %d/%s уже занят экземпляром %s", port.HostPort, port.Protocol, existing.Name))
				}
			}
		}
	}
	return nil
}

var _ = realtime.InstanceEvent{}
