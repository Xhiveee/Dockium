package panelapi

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect"
	emptypb "google.golang.org/protobuf/types/known/emptypb"

	agentv1 "dockium/gen/agent/v1"
	panelv1 "dockium/gen/panel/v1"
	"dockium/internal/db"
)

func (s *Server) AttachConsole(ctx context.Context, request *connect.Request[panelv1.AttachConsoleRequest], stream *connect.ServerStream[panelv1.ConsoleLine]) error {
	instance, _, err := s.instanceFor(ctx, request.Msg.InstanceId, func(permission db.Permission) bool { return permission.Console }, "нет доступа к консоли")
	if err != nil {
		return err
	}
	if !s.hub.Online(instance.AgentID) {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("агент офлайн: консоль недоступна"))
	}
	streamID := newStreamID()
	subscription, err := s.hub.Subscribe(instance.AgentID, streamID)
	if err != nil {
		return rpcError(err, "не удалось подключиться к консоли")
	}
	defer func() {
		subscription.Close()
		_ = s.hub.Send(instance.AgentID, &agentv1.Command{Payload: &agentv1.Command_ConsoleDetach{
			ConsoleDetach: &agentv1.ConsoleDetach{StreamId: streamID},
		}})
	}()
	if err := s.hub.Send(instance.AgentID, &agentv1.Command{Payload: &agentv1.Command_ConsoleAttach{
		ConsoleAttach: &agentv1.ConsoleAttach{StreamId: streamID, InstanceId: instance.ID, TailLines: request.Msg.TailLines},
	}}); err != nil {
		return rpcError(err, "не удалось подключиться к консоли")
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-subscription.Done():
			return connect.NewError(connect.CodeUnavailable, errors.New("агент отключился, консоль закрыта"))
		case message, ok := <-subscription.Messages():
			if !ok {
				return connect.NewError(connect.CodeUnavailable, errors.New("поток консоли закрыт"))
			}
			data := message.GetConsole()
			if data == nil {
				continue
			}
			for _, line := range data.Lines {
				kind := panelv1.ConsoleLineKind_CONSOLE_LINE_KIND_STDOUT
				switch line.Kind {
				case 2:
					kind = panelv1.ConsoleLineKind_CONSOLE_LINE_KIND_STDERR
				case 3:
					kind = panelv1.ConsoleLineKind_CONSOLE_LINE_KIND_SYSTEM
				}
				if err := stream.Send(&panelv1.ConsoleLine{
					Seq:       line.Seq,
					Kind:      kind,
					Text:      line.Text,
					Timestamp: line.Timestamp,
				}); err != nil {
					return err
				}
			}
			if data.Done {
				if data.Error != "" {
					return connect.NewError(connect.CodeFailedPrecondition, errors.New(data.Error))
				}
				return nil
			}
		}
	}
}

func (s *Server) SendConsoleCommand(ctx context.Context, request *connect.Request[panelv1.SendConsoleCommandRequest]) (*connect.Response[emptypb.Empty], error) {
	instance, _, err := s.instanceFor(ctx, request.Msg.InstanceId, func(permission db.Permission) bool { return permission.Console }, "нет доступа к консоли")
	if err != nil {
		return nil, err
	}
	if len(request.Msg.Command) == 0 {
		return nil, invalid("введите команду")
	}
	if len(request.Msg.Command) > 4096 {
		return nil, invalid("слишком длинная команда")
	}
	command := &agentv1.Command{Payload: &agentv1.Command_ConsoleInput{ConsoleInput: &agentv1.ConsoleInput{
		InstanceId: instance.ID,
		Text:       request.Msg.Command,
	}}}
	if _, err := s.hub.Execute(ctx, instance.AgentID, command, 30*time.Second); err != nil {
		return nil, rpcError(err, "не удалось отправить команду")
	}
	return emptyResponse(), nil
}
