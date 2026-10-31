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

func (s *Server) fileInstance(ctx context.Context, id int64) (*db.Instance, error) {
	instance, _, err := s.instanceFor(ctx, id, func(permission db.Permission) bool { return permission.Files }, "нет доступа к файлам экземпляра")
	return instance, err
}

func (s *Server) ListFiles(ctx context.Context, request *connect.Request[panelv1.ListFilesRequest]) (*connect.Response[panelv1.ListFilesResponse], error) {
	instance, err := s.fileInstance(ctx, request.Msg.InstanceId)
	if err != nil {
		return nil, err
	}
	command := &agentv1.Command{Payload: &agentv1.Command_ListFiles{ListFiles: &agentv1.ListFiles{
		InstanceId: instance.ID, Path: request.Msg.Path,
	}}}
	result, err := s.hub.Execute(ctx, instance.AgentID, command, 60*time.Second)
	if err != nil {
		return nil, rpcError(err, "не удалось прочитать каталог")
	}
	list := result.GetFiles()
	if list == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("агент вернул некорректный ответ"))
	}
	items := make([]*panelv1.FileEntry, 0, len(list.Items))
	for _, entry := range list.Items {
		items = append(items, &panelv1.FileEntry{
			Name:       entry.Name,
			Path:       entry.Path,
			IsDir:      entry.IsDir,
			Size:       entry.Size,
			ModifiedAt: entry.ModifiedAt,
			Mode:       entry.Mode,
		})
	}
	return connect.NewResponse(&panelv1.ListFilesResponse{Items: items, Path: list.Path}), nil
}

func (s *Server) ReadFile(ctx context.Context, request *connect.Request[panelv1.ReadFileRequest]) (*connect.Response[panelv1.ReadFileResponse], error) {
	instance, err := s.fileInstance(ctx, request.Msg.InstanceId)
	if err != nil {
		return nil, err
	}
	command := &agentv1.Command{Payload: &agentv1.Command_ReadFile{ReadFile: &agentv1.ReadFile{
		InstanceId: instance.ID, Path: request.Msg.Path,
	}}}
	result, err := s.hub.Execute(ctx, instance.AgentID, command, 60*time.Second)
	if err != nil {
		return nil, rpcError(err, "не удалось прочитать файл")
	}
	file := result.GetFile()
	if file == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("агент вернул некорректный ответ"))
	}
	return connect.NewResponse(&panelv1.ReadFileResponse{
		Content:    file.Content,
		Size:       file.Size,
		ModifiedAt: file.ModifiedAt,
		Encoding:   "utf-8",
		Binary:     file.Binary,
	}), nil
}

func (s *Server) WriteFile(ctx context.Context, request *connect.Request[panelv1.WriteFileRequest]) (*connect.Response[emptypb.Empty], error) {
	instance, err := s.fileInstance(ctx, request.Msg.InstanceId)
	if err != nil {
		return nil, err
	}
	if len(request.Msg.Content) > 4<<20 {
		return nil, invalid("файл слишком большой для редактора (максимум 4 МБ)")
	}
	command := &agentv1.Command{Payload: &agentv1.Command_WriteFile{WriteFile: &agentv1.WriteFile{
		InstanceId: instance.ID, Path: request.Msg.Path, Content: request.Msg.Content, Create: request.Msg.Create,
	}}}
	if _, err := s.hub.Execute(ctx, instance.AgentID, command, 60*time.Second); err != nil {
		return nil, rpcError(err, "не удалось сохранить файл")
	}
	return emptyResponse(), nil
}

func (s *Server) CreateDirectory(ctx context.Context, request *connect.Request[panelv1.CreateDirectoryRequest]) (*connect.Response[emptypb.Empty], error) {
	instance, err := s.fileInstance(ctx, request.Msg.InstanceId)
	if err != nil {
		return nil, err
	}
	command := &agentv1.Command{Payload: &agentv1.Command_Mkdir{Mkdir: &agentv1.CreateDirectory{
		InstanceId: instance.ID, Path: request.Msg.Path,
	}}}
	if _, err := s.hub.Execute(ctx, instance.AgentID, command, 60*time.Second); err != nil {
		return nil, rpcError(err, "не удалось создать каталог")
	}
	return emptyResponse(), nil
}

func (s *Server) DeleteFile(ctx context.Context, request *connect.Request[panelv1.DeleteFileRequest]) (*connect.Response[emptypb.Empty], error) {
	instance, err := s.fileInstance(ctx, request.Msg.InstanceId)
	if err != nil {
		return nil, err
	}
	command := &agentv1.Command{Payload: &agentv1.Command_DeletePath{DeletePath: &agentv1.DeletePath{
		InstanceId: instance.ID, Path: request.Msg.Path, Recursive: request.Msg.Recursive,
	}}}
	if _, err := s.hub.Execute(ctx, instance.AgentID, command, 60*time.Second); err != nil {
		return nil, rpcError(err, "не удалось удалить файл")
	}
	return emptyResponse(), nil
}

func (s *Server) RenameFile(ctx context.Context, request *connect.Request[panelv1.RenameFileRequest]) (*connect.Response[emptypb.Empty], error) {
	instance, err := s.fileInstance(ctx, request.Msg.InstanceId)
	if err != nil {
		return nil, err
	}
	command := &agentv1.Command{Payload: &agentv1.Command_RenamePath{RenamePath: &agentv1.RenamePath{
		InstanceId: instance.ID, OldPath: request.Msg.OldPath, NewPath: request.Msg.NewPath,
	}}}
	if _, err := s.hub.Execute(ctx, instance.AgentID, command, 60*time.Second); err != nil {
		return nil, rpcError(err, "не удалось переименовать")
	}
	return emptyResponse(), nil
}

func (s *Server) UploadFile(ctx context.Context, request *connect.Request[panelv1.UploadFileRequest]) (*connect.Response[emptypb.Empty], error) {
	instance, err := s.fileInstance(ctx, request.Msg.InstanceId)
	if err != nil {
		return nil, err
	}
	if len(request.Msg.Content) > 32<<20 {
		return nil, invalid("файл слишком большой (максимум 32 МБ)")
	}
	command := &agentv1.Command{Payload: &agentv1.Command_UploadFile{UploadFile: &agentv1.UploadFile{
		InstanceId: instance.ID, Path: request.Msg.Path, Content: request.Msg.Content, Overwrite: request.Msg.Overwrite,
	}}}
	if _, err := s.hub.Execute(ctx, instance.AgentID, command, 3*time.Minute); err != nil {
		return nil, rpcError(err, "не удалось загрузить файл")
	}
	return emptyResponse(), nil
}

func (s *Server) DownloadFile(ctx context.Context, request *connect.Request[panelv1.DownloadFileRequest], stream *connect.ServerStream[panelv1.FileChunk]) error {
	instance, err := s.fileInstance(ctx, request.Msg.InstanceId)
	if err != nil {
		return err
	}
	streamID := newStreamID()
	subscription, err := s.hub.Subscribe(instance.AgentID, streamID)
	if err != nil {
		return rpcError(err, "не удалось скачать файл")
	}
	defer subscription.Close()
	if err := s.hub.Send(instance.AgentID, &agentv1.Command{Payload: &agentv1.Command_DownloadFile{DownloadFile: &agentv1.DownloadFile{
		InstanceId: instance.ID, Path: request.Msg.Path, StreamId: streamID,
	}}}); err != nil {
		return rpcError(err, "не удалось скачать файл")
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-subscription.Done():
			return connect.NewError(connect.CodeUnavailable, errors.New("агент отключился"))
		case message, ok := <-subscription.Messages():
			if !ok {
				return connect.NewError(connect.CodeUnavailable, errors.New("поток файла закрыт"))
			}
			chunk := message.GetFileChunk()
			if chunk == nil {
				continue
			}
			if len(chunk.Data) > 0 {
				if err := stream.Send(&panelv1.FileChunk{Data: chunk.Data}); err != nil {
					return err
				}
			}
			if chunk.Eof {
				if chunk.Error != "" {
					return connect.NewError(connect.CodeFailedPrecondition, errors.New(chunk.Error))
				}
				return nil
			}
		}
	}
}

func (s *Server) ChmodFile(ctx context.Context, request *connect.Request[panelv1.ChmodFileRequest]) (*connect.Response[emptypb.Empty], error) {
	instance, err := s.fileInstance(ctx, request.Msg.InstanceId)
	if err != nil {
		return nil, err
	}
	command := &agentv1.Command{Payload: &agentv1.Command_Chmod{Chmod: &agentv1.ChmodPath{
		InstanceId: instance.ID, Path: request.Msg.Path, Mode: request.Msg.Mode,
	}}}
	if _, err := s.hub.Execute(ctx, instance.AgentID, command, 60*time.Second); err != nil {
		return nil, rpcError(err, "не удалось изменить права")
	}
	return emptyResponse(), nil
}

func (s *Server) CompressFile(ctx context.Context, request *connect.Request[panelv1.CompressFileRequest]) (*connect.Response[panelv1.CompressFileResponse], error) {
	instance, err := s.fileInstance(ctx, request.Msg.InstanceId)
	if err != nil {
		return nil, err
	}
	command := &agentv1.Command{Payload: &agentv1.Command_Compress{Compress: &agentv1.CompressPath{
		InstanceId: instance.ID, Path: request.Msg.Path, Target: request.Msg.Target, Overwrite: request.Msg.Overwrite,
	}}}
	result, err := s.hub.Execute(ctx, instance.AgentID, command, 2*time.Minute)
	if err != nil {
		return nil, rpcError(err, "не удалось создать архив")
	}
	pathInfo := result.GetPath()
	path := ""
	if pathInfo != nil {
		path = pathInfo.Path
	}
	return connect.NewResponse(&panelv1.CompressFileResponse{Path: path}), nil
}
