package panelapi

import (
	"context"
	"errors"
	"net"
	"strings"

	"connectrpc.com/connect"
	emptypb "google.golang.org/protobuf/types/known/emptypb"

	panelv1 "dockium/gen/panel/v1"
	"dockium/internal/db"
	"dockium/internal/geoip"
	"dockium/internal/present"
)

func (s *Server) ListLoginLogs(ctx context.Context, request *connect.Request[panelv1.Page]) (*connect.Response[panelv1.LoginLogList], error) {
	if _, err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	limit, offset := normalizePage(request.Msg)
	entries, err := s.store.ListLoginLogs(ctx, limit, offset)
	if err != nil {
		return nil, rpcError(err, "не удалось получить журнал входов")
	}
	total, err := s.store.CountLoginLogs(ctx)
	if err != nil {
		return nil, rpcError(err, "не удалось получить журнал входов")
	}
	items := make([]*panelv1.LoginLog, 0, len(entries))
	for _, entry := range entries {
		if countryUnresolved(entry.IP, entry.Country, entry.CountryCode) {
			pending := *entry
			go s.resolveLoginCountry(&pending)
		}
		items = append(items, present.LoginLog(entry))
	}
	return connect.NewResponse(&panelv1.LoginLogList{Items: items, Total: uint32(total)}), nil
}

func (s *Server) ListBans(ctx context.Context, request *connect.Request[panelv1.Page]) (*connect.Response[panelv1.BanList], error) {
	if _, err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	bans, err := s.store.ListBans(ctx, db.Now())
	if err != nil {
		return nil, rpcError(err, "не удалось получить список блокировок")
	}
	total, err := s.store.CountBans(ctx, db.Now())
	if err != nil {
		return nil, rpcError(err, "не удалось получить список блокировок")
	}
	items := make([]*panelv1.Ban, 0, len(bans))
	for _, ban := range bans {
		if countryUnresolved(ban.IP, ban.Country, ban.CountryCode) {
			pending := *ban
			go s.resolveBanCountry(&pending)
		}
		items = append(items, present.Ban(ban))
	}
	return connect.NewResponse(&panelv1.BanList{Items: items, Total: uint32(total)}), nil
}

func (s *Server) CreateBan(ctx context.Context, request *connect.Request[panelv1.CreateBanRequest]) (*connect.Response[panelv1.Ban], error) {
	if _, err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	ip := strings.TrimSpace(request.Msg.Ip)
	if net.ParseIP(ip) == nil {
		return nil, invalid("укажите корректный IP-адрес")
	}
	hours := request.Msg.Hours
	if hours == 0 {
		hours = 24
	}
	if hours > 8760 {
		return nil, invalid("максимальный срок блокировки — 1 год")
	}
	reason := strings.TrimSpace(request.Msg.Reason)
	if len(reason) > 256 {
		return nil, invalid("слишком длинная причина")
	}
	country, resolved := geoip.Classify(ip)
	ban, err := s.store.CreateBan(ctx, &db.Ban{
		IP:          ip,
		Reason:      reason,
		Source:      "manual",
		Country:     country.Name,
		CountryCode: country.Code,
		BannedAt:    db.Now(),
		ExpiresAt:   db.Now() + int64(hours)*3600*1000,
	})
	if err != nil {
		return nil, rpcError(err, "не удалось добавить блокировку")
	}
	s.publisher.BanChanged(ban)
	if !resolved {
		pending := *ban
		go s.resolveBanCountry(&pending)
	}
	return connect.NewResponse(present.Ban(ban)), nil
}

func (s *Server) DeleteBan(ctx context.Context, request *connect.Request[panelv1.UserIdRequest]) (*connect.Response[emptypb.Empty], error) {
	if _, err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	if err := s.store.DeleteBan(ctx, request.Msg.Id); err != nil {
		return nil, rpcError(err, "не удалось снять блокировку")
	}
	s.publisher.BanRemoved(request.Msg.Id)
	return emptyResponse(), nil
}

func (s *Server) ListApiRequests(ctx context.Context, request *connect.Request[panelv1.Page]) (*connect.Response[panelv1.ApiRequestList], error) {
	if _, err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	limit, offset := normalizePage(request.Msg)
	entries, err := s.store.ListApiRequests(ctx, limit, offset)
	if err != nil {
		return nil, rpcError(err, "не удалось получить журнал запросов")
	}
	total, err := s.store.CountApiRequests(ctx)
	if err != nil {
		return nil, rpcError(err, "не удалось получить журнал запросов")
	}
	items := make([]*panelv1.ApiRequest, 0, len(entries))
	for _, entry := range entries {
		items = append(items, present.ApiRequest(entry))
	}
	return connect.NewResponse(&panelv1.ApiRequestList{Items: items, Total: uint32(total)}), nil
}

func (s *Server) ListSessions(ctx context.Context, _ *connect.Request[emptypb.Empty]) (*connect.Response[panelv1.SessionList], error) {
	state, err := s.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	sessions, err := s.store.ListSessions(ctx, db.Now())
	if err != nil {
		return nil, rpcError(err, "не удалось получить сессии")
	}
	items := make([]*panelv1.SessionInfo, 0, len(sessions))
	for _, session := range sessions {
		items = append(items, present.SessionInfo(session, session.ID == state.sessionID))
	}
	return connect.NewResponse(&panelv1.SessionList{Items: items}), nil
}

func (s *Server) RevokeSession(ctx context.Context, request *connect.Request[panelv1.RevokeSessionRequest]) (*connect.Response[emptypb.Empty], error) {
	state, err := s.requireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	id := strings.TrimSpace(request.Msg.Id)
	if id == "" {
		return nil, invalid("укажите сессию")
	}
	sessions, err := s.store.ListSessions(ctx, db.Now())
	if err != nil {
		return nil, rpcError(err, "не удалось получить сессии")
	}
	var target *db.SessionRow
	for _, session := range sessions {
		if session.ID == id {
			target = session
			break
		}
	}
	if target == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("сессия не найдена"))
	}
	if err := s.store.DeleteSession(ctx, id); err != nil {
		return nil, rpcError(err, "не удалось завершить сессию")
	}
	s.publisher.SessionRevoked(target)
	response := emptyResponse()
	if id == state.sessionID {
		response.Header().Set("Set-Cookie", s.clearCookie().String())
	}
	return response, nil
}

func countryUnresolved(ip, country, code string) bool {
	return !geoip.IsLocal(ip) && (country == "" || code == "")
}

func (s *Server) WatchSecurity(ctx context.Context, _ *connect.Request[emptypb.Empty], stream *connect.ServerStream[panelv1.SecurityEvent]) error {
	if _, err := s.requireAdmin(ctx); err != nil {
		return err
	}
	events, unsubscribe := s.publisher.SubscribeSecurity(128)
	defer unsubscribe()
	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-events:
			if !ok {
				return nil
			}
			if err := stream.Send(event); err != nil {
				return err
			}
		}
	}
}
