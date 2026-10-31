package panelapi

import (
	"context"
	"strings"

	"connectrpc.com/connect"
	emptypb "google.golang.org/protobuf/types/known/emptypb"

	panelv1 "dockium/gen/panel/v1"
	"dockium/internal/auth"
	"dockium/internal/present"
)

func (s *Server) ListAgents(ctx context.Context, _ *connect.Request[emptypb.Empty]) (*connect.Response[panelv1.AgentList], error) {
	if _, err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	items, err := s.agentsSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&panelv1.AgentList{Items: items}), nil
}

func (s *Server) WatchAgents(ctx context.Context, _ *connect.Request[emptypb.Empty], stream *connect.ServerStream[panelv1.AgentEvent]) error {
	if _, err := s.requireAdmin(ctx); err != nil {
		return err
	}
	events, unsubscribe := s.publisher.SubscribeAgents(64)
	defer unsubscribe()
	snapshot, err := s.agentsSnapshot(ctx)
	if err != nil {
		return err
	}
	if err := stream.Send(&panelv1.AgentEvent{Payload: &panelv1.AgentEvent_Snapshot{Snapshot: &panelv1.AgentSnapshot{Items: snapshot}}}); err != nil {
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
			payload := &panelv1.AgentEvent{}
			switch {
			case event.RemovedID > 0:
				payload.Payload = &panelv1.AgentEvent_Removed{Removed: &panelv1.AgentRemoved{Id: event.RemovedID}}
			case event.Snapshot != nil:
				items := make([]*panelv1.Agent, 0, len(event.Snapshot))
				for _, info := range event.Snapshot {
					items = append(items, present.Agent(info.Agent, info.InstancesTotal, info.Running))
				}
				payload.Payload = &panelv1.AgentEvent_Snapshot{Snapshot: &panelv1.AgentSnapshot{Items: items}}
			case event.Agent != nil:
				payload.Payload = &panelv1.AgentEvent_Agent{Agent: present.Agent(event.Agent.Agent, event.Agent.InstancesTotal, event.Agent.Running)}
			default:
				continue
			}
			if err := stream.Send(payload); err != nil {
				return err
			}
		}
	}
}

func (s *Server) CreateAgent(ctx context.Context, request *connect.Request[panelv1.CreateAgentRequest]) (*connect.Response[panelv1.AgentCreated], error) {
	if _, err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(request.Msg.Name)
	if len(name) < 2 || len(name) > 64 {
		return nil, invalid("название агента: от 2 до 64 символов")
	}
	country := strings.TrimSpace(request.Msg.Country)
	if len(country) > 64 {
		return nil, invalid("слишком длинное название страны")
	}
	raw, hash, err := auth.NewToken()
	if err != nil {
		return nil, rpcError(err, "не удалось создать токен агента")
	}
	agent, err := s.store.CreateAgent(ctx, name, country, hash)
	if err != nil {
		return nil, rpcError(err, "не удалось создать агента")
	}
	s.publisher.AgentUpdated(agent, 0, 0)
	instanceCount, runningCount := uint32(0), uint32(0)
	return connect.NewResponse(&panelv1.AgentCreated{
		Agent:    present.Agent(agent, instanceCount, runningCount),
		Token:    raw,
		Endpoint: s.agentEndpoint,
	}), nil
}

func (s *Server) UpdateAgent(ctx context.Context, request *connect.Request[panelv1.UpdateAgentRequest]) (*connect.Response[panelv1.Agent], error) {
	if _, err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(request.Msg.Name)
	if len(name) < 2 || len(name) > 64 {
		return nil, invalid("название агента: от 2 до 64 символов")
	}
	if len(request.Msg.Country) > 64 {
		return nil, invalid("слишком длинное название страны")
	}
	if _, err := s.store.AgentByID(ctx, request.Msg.Id); err != nil {
		return nil, rpcError(err, "агент не найден")
	}
	if err := s.store.UpdateAgent(ctx, request.Msg.Id, name, strings.TrimSpace(request.Msg.Country)); err != nil {
		return nil, rpcError(err, "не удалось обновить агента")
	}
	s.hub.PublishAgent(request.Msg.Id)
	agent, err := s.store.AgentByID(ctx, request.Msg.Id)
	if err != nil {
		return nil, rpcError(err, "агент не найден")
	}
	total, running, _ := s.store.InstanceCounts(ctx, agent.ID)
	return connect.NewResponse(present.Agent(agent, total, running)), nil
}

func (s *Server) DeleteAgent(ctx context.Context, request *connect.Request[panelv1.UserIdRequest]) (*connect.Response[emptypb.Empty], error) {
	if _, err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	agent, err := s.store.AgentByID(ctx, request.Msg.Id)
	if err != nil {
		return nil, rpcError(err, "агент не найден")
	}
	total, _, err := s.store.InstanceCounts(ctx, agent.ID)
	if err != nil {
		return nil, rpcError(err, "не удалось проверить экземпляры агента")
	}
	if total > 0 {
		return nil, failedPrecondition("сначала удалите экземпляры этого агента")
	}
	s.hub.CloseAgent(agent.ID)
	if err := s.store.DeleteAgent(ctx, agent.ID); err != nil {
		return nil, rpcError(err, "не удалось удалить агента")
	}
	s.publisher.AgentRemoved(agent.ID)
	return emptyResponse(), nil
}

func (s *Server) RotateToken(ctx context.Context, request *connect.Request[panelv1.UserIdRequest]) (*connect.Response[panelv1.AgentToken], error) {
	if _, err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	agent, err := s.store.AgentByID(ctx, request.Msg.Id)
	if err != nil {
		return nil, rpcError(err, "агент не найден")
	}
	raw, hash, err := auth.NewToken()
	if err != nil {
		return nil, rpcError(err, "не удалось создать токен")
	}
	if err := s.store.RotateAgentToken(ctx, agent.ID, hash); err != nil {
		return nil, rpcError(err, "не удалось обновить токен")
	}
	s.hub.CloseAgent(agent.ID)
	return connect.NewResponse(&panelv1.AgentToken{Id: agent.ID, Token: raw}), nil
}

func (s *Server) agentsSnapshot(ctx context.Context) ([]*panelv1.Agent, error) {
	agents, err := s.store.ListAgents(ctx)
	if err != nil {
		return nil, rpcError(err, "не удалось получить агентов")
	}
	counts, err := s.store.InstanceCountsByAgent(ctx)
	if err != nil {
		return nil, rpcError(err, "не удалось получить экземпляры")
	}
	items := make([]*panelv1.Agent, 0, len(agents))
	for _, agent := range agents {
		count := counts[agent.ID]
		items = append(items, present.Agent(agent, count[0], count[1]))
	}
	return items, nil
}
