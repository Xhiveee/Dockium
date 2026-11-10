package panelapi

import (
	"context"

	"connectrpc.com/connect"
	emptypb "google.golang.org/protobuf/types/known/emptypb"

	panelv1 "dockium/gen/panel/v1"
	"dockium/internal/db"
)

func (s *Server) GetSettings(ctx context.Context, _ *connect.Request[emptypb.Empty]) (*connect.Response[panelv1.PanelSettings], error) {
	if _, err := s.requireUser(ctx); err != nil {
		return nil, err
	}
	settings, err := s.store.PanelSettings(ctx)
	if err != nil {
		return nil, rpcError(err, "не удалось получить настройки панели")
	}
	return connect.NewResponse(&panelv1.PanelSettings{MaxUploadMb: settings.MaxUploadMB}), nil
}

func (s *Server) UpdateSettings(ctx context.Context, request *connect.Request[panelv1.UpdatePanelSettingsRequest]) (*connect.Response[panelv1.PanelSettings], error) {
	if _, err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	maxUploadMB := request.Msg.GetMaxUploadMb()
	if maxUploadMB < 1 || maxUploadMB > 4096 {
		return nil, invalid("максимальный размер загрузки: от 1 до 4096 МБ")
	}
	if err := s.store.UpdatePanelSettings(ctx, &db.PanelSettings{MaxUploadMB: maxUploadMB}); err != nil {
		return nil, rpcError(err, "не удалось сохранить настройки панели")
	}
	return connect.NewResponse(&panelv1.PanelSettings{MaxUploadMb: maxUploadMB}), nil
}
