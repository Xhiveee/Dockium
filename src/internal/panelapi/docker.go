package panelapi

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	emptypb "google.golang.org/protobuf/types/known/emptypb"

	agentv1 "dockium/gen/agent/v1"
	panelv1 "dockium/gen/panel/v1"
	"dockium/internal/hub"
)

func (s *Server) dockerAgent(ctx context.Context, agentID int64) error {
	if _, err := s.requireAdmin(ctx); err != nil {
		return err
	}
	if !s.hub.Online(agentID) {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("агент офлайн"))
	}
	return nil
}

func (s *Server) ListDockerImages(ctx context.Context, request *connect.Request[panelv1.AgentIdRequest]) (*connect.Response[panelv1.DockerImageList], error) {
	if err := s.dockerAgent(ctx, request.Msg.AgentId); err != nil {
		return nil, err
	}
	command := &agentv1.Command{Payload: &agentv1.Command_ListImages{ListImages: &agentv1.ListImages{}}}
	result, err := s.hub.Execute(ctx, request.Msg.AgentId, command, 60*secondTimeout)
	if err != nil {
		return nil, rpcError(err, "не удалось получить образы")
	}
	images := result.GetImages()
	items := make([]*panelv1.DockerImage, 0)
	if images != nil {
		for _, image := range images.Items {
			items = append(items, &panelv1.DockerImage{
				Id:         image.Id,
				Repository: image.Repository,
				Tag:        image.Tag,
				SizeBytes:  image.SizeBytes,
				CreatedAt:  image.CreatedAt,
			})
		}
	}
	return connect.NewResponse(&panelv1.DockerImageList{Items: items}), nil
}

func (s *Server) ListDockerContainers(ctx context.Context, request *connect.Request[panelv1.AgentIdRequest]) (*connect.Response[panelv1.DockerContainerList], error) {
	if err := s.dockerAgent(ctx, request.Msg.AgentId); err != nil {
		return nil, err
	}
	command := &agentv1.Command{Payload: &agentv1.Command_ListContainers{ListContainers: &agentv1.ListContainers{}}}
	result, err := s.hub.Execute(ctx, request.Msg.AgentId, command, 60*secondTimeout)
	if err != nil {
		return nil, rpcError(err, "не удалось получить контейнеры")
	}
	containers := result.GetContainers()
	items := make([]*panelv1.DockerContainer, 0)
	if containers != nil {
		for _, container := range containers.Items {
			items = append(items, &panelv1.DockerContainer{
				Id:         container.Id,
				Name:       container.Name,
				Image:      container.Image,
				Command:    container.Command,
				Status:     container.Status,
				State:      container.State,
				CreatedAt:  container.CreatedAt,
				Managed:    container.Managed,
				InstanceId: container.InstanceId,
			})
		}
	}
	return connect.NewResponse(&panelv1.DockerContainerList{Items: items}), nil
}

func (s *Server) InspectDocker(ctx context.Context, request *connect.Request[panelv1.InspectDockerRequest]) (*connect.Response[panelv1.InspectDockerResponse], error) {
	if err := s.dockerAgent(ctx, request.Msg.AgentId); err != nil {
		return nil, err
	}
	if request.Msg.Kind != "image" && request.Msg.Kind != "container" {
		return nil, invalid("неизвестный тип объекта")
	}
	if strings.TrimSpace(request.Msg.Id) == "" {
		return nil, invalid("укажите объект Docker")
	}
	command := &agentv1.Command{Payload: &agentv1.Command_Inspect{Inspect: &agentv1.InspectObject{
		Kind: request.Msg.Kind, Id: request.Msg.Id,
	}}}
	result, err := s.hub.Execute(ctx, request.Msg.AgentId, command, 60*secondTimeout)
	if err != nil {
		return nil, rpcError(err, "не удалось получить информацию")
	}
	inspect := result.GetInspect()
	json := ""
	if inspect != nil {
		json = inspect.Json
	}
	return connect.NewResponse(&panelv1.InspectDockerResponse{Json: json}), nil
}

func (s *Server) RemoveDockerImage(ctx context.Context, request *connect.Request[panelv1.RemoveImageRequest]) (*connect.Response[emptypb.Empty], error) {
	if err := s.dockerAgent(ctx, request.Msg.AgentId); err != nil {
		return nil, err
	}
	if strings.TrimSpace(request.Msg.Image) == "" {
		return nil, invalid("укажите образ")
	}
	command := &agentv1.Command{Payload: &agentv1.Command_RemoveImage{RemoveImage: &agentv1.RemoveImage{Image: request.Msg.Image}}}
	if _, err := s.hub.Execute(ctx, request.Msg.AgentId, command, 2*minuteTimeout); err != nil {
		return nil, rpcError(err, "не удалось удалить образ")
	}
	return emptyResponse(), nil
}

func (s *Server) PullDockerImage(ctx context.Context, request *connect.Request[panelv1.PullImageRequest], stream *connect.ServerStream[panelv1.BuildOutput]) error {
	if err := s.dockerAgent(ctx, request.Msg.AgentId); err != nil {
		return err
	}
	image := strings.TrimSpace(request.Msg.Image)
	if image == "" || len(image) > 255 {
		return invalid("укажите корректное имя образа")
	}
	streamID := newStreamID()
	subscription, err := s.hub.Subscribe(request.Msg.AgentId, streamID)
	if err != nil {
		return rpcError(err, "не удалось загрузить образ")
	}
	defer subscription.Close()
	if err := s.hub.Send(request.Msg.AgentId, &agentv1.Command{Payload: &agentv1.Command_PullImage{PullImage: &agentv1.PullImage{
		Image: image, StreamId: streamID,
	}}}); err != nil {
		return rpcError(err, "не удалось загрузить образ")
	}
	return relayBuildStream(ctx, subscription, stream)
}

func (s *Server) BuildDockerImage(ctx context.Context, request *connect.Request[panelv1.BuildImageRequest], stream *connect.ServerStream[panelv1.BuildOutput]) error {
	if err := s.dockerAgent(ctx, request.Msg.AgentId); err != nil {
		return err
	}
	tag := strings.TrimSpace(request.Msg.Tag)
	if tag == "" || len(tag) > 255 {
		return invalid("укажите тег образа")
	}
	if strings.TrimSpace(request.Msg.Dockerfile) == "" {
		return invalid("Dockerfile пуст")
	}
	if len(request.Msg.Dockerfile) > 256<<10 {
		return invalid("Dockerfile слишком большой")
	}
	streamID := newStreamID()
	subscription, err := s.hub.Subscribe(request.Msg.AgentId, streamID)
	if err != nil {
		return rpcError(err, "не удалось собрать образ")
	}
	defer subscription.Close()
	if err := s.hub.Send(request.Msg.AgentId, &agentv1.Command{Payload: &agentv1.Command_BuildImage{BuildImage: &agentv1.BuildImage{
		Tag: tag, Dockerfile: request.Msg.Dockerfile, StreamId: streamID,
	}}}); err != nil {
		return rpcError(err, "не удалось собрать образ")
	}
	return relayBuildStream(ctx, subscription, stream)
}

func relayBuildStream(ctx context.Context, subscription *hub.Stream, stream *connect.ServerStream[panelv1.BuildOutput]) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-subscription.Done():
			return connect.NewError(connect.CodeUnavailable, errors.New("агент отключился"))
		case message, ok := <-subscription.Messages():
			if !ok {
				return connect.NewError(connect.CodeUnavailable, errors.New("поток сборки закрыт"))
			}
			event := message.GetStream()
			if event == nil {
				continue
			}
			output := &panelv1.BuildOutput{Line: event.Line, Done: event.Done, Error: event.Error}
			if err := stream.Send(output); err != nil {
				return err
			}
			if event.Done {
				if event.Error != "" {
					return connect.NewError(connect.CodeFailedPrecondition, errors.New(event.Error))
				}
				return nil
			}
		}
	}
}
