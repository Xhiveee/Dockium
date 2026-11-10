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
	"dockium/internal/auth"
	"dockium/internal/db"
	"dockium/internal/geoip"
)

func (s *Server) GetStatus(ctx context.Context, _ *connect.Request[emptypb.Empty]) (*connect.Response[panelv1.SetupStatus], error) {
	initialized, err := s.store.SetupInitialized(ctx)
	if err != nil {
		return nil, rpcError(err, "не удалось проверить состояние панели")
	}
	return connect.NewResponse(&panelv1.SetupStatus{Initialized: initialized, Version: s.version}), nil
}

func (s *Server) CreateAdmin(ctx context.Context, request *connect.Request[panelv1.CreateAdminRequest]) (*connect.Response[panelv1.User], error) {
	credentials := request.Msg.GetCredentials()
	if credentials == nil {
		return nil, invalid("укажите логин и пароль")
	}
	login := strings.TrimSpace(credentials.Login)
	if err := auth.ValidateLogin(login); err != nil {
		return nil, invalid(err.Error())
	}
	if err := auth.ValidatePassword(credentials.Password, login); err != nil {
		return nil, invalid(err.Error())
	}
	hash, err := auth.HashPassword(credentials.Password)
	if err != nil {
		return nil, rpcError(err, "не удалось создать администратора")
	}
	user, err := s.store.CreateFirstAdmin(ctx, login, hash)
	if err != nil {
		if errors.Is(err, db.ErrConflict) {
			return nil, failedPrecondition("панель уже настроена")
		}
		return nil, rpcError(err, "не удалось создать администратора")
	}
	state, _ := principalFrom(ctx)
	ip := ""
	if state != nil {
		ip = state.ip
	}
	userAgent := request.Header().Get("User-Agent")
	cookie, err := s.sessionCookie(ctx, user, ip, userAgent)
	if err != nil {
		return nil, rpcError(err, "не удалось создать сессию")
	}
	_ = s.store.MarkLogin(ctx, user.ID)
	s.loginLog(ctx, user.Login, true, "создан администратор", ip, userAgent)
	response := connect.NewResponse(protoUser(user))
	response.Header().Set("Set-Cookie", cookie.String())
	return response, nil
}

func (s *Server) Login(ctx context.Context, request *connect.Request[panelv1.Credentials]) (*connect.Response[panelv1.LoginResponse], error) {
	username := strings.TrimSpace(request.Msg.Login)
	password := request.Msg.Password
	state, _ := principalFrom(ctx)
	ip := ""
	if state != nil {
		ip = state.ip
	}
	userAgent := request.Header().Get("User-Agent")
	if username == "" || password == "" {
		return nil, invalid("введите логин и пароль")
	}
	if ban, err := s.store.ActiveBan(ctx, ip, db.Now()); err == nil {
		s.loginLog(ctx, username, false, "ip заблокирован", ip, userAgent)
		until := timeMilliLabel(ban.ExpiresAt)
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("доступ с этого адреса заблокирован до %s", until))
	}
	user, lookupErr := s.store.UserByLogin(ctx, username)
	if lookupErr != nil {
		auth.VerifyDummyPassword(password)
		s.loginFailure(ctx, username, ip, userAgent, "неверный логин или пароль")
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("неверный логин или пароль"))
	}
	if !auth.VerifyPassword(user.PasswordHash, password) {
		s.loginFailure(ctx, username, ip, userAgent, "неверный пароль")
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("неверный логин или пароль"))
	}
	cookie, err := s.sessionCookie(ctx, user, ip, userAgent)
	if err != nil {
		return nil, rpcError(err, "не удалось создать сессию")
	}
	_ = s.store.MarkLogin(ctx, user.ID)
	user.LastLoginAt = db.Now()
	s.loginLog(ctx, user.Login, true, "", ip, userAgent)
	response := connect.NewResponse(&panelv1.LoginResponse{User: protoUser(user)})
	response.Header().Set("Set-Cookie", cookie.String())
	return response, nil
}

func (s *Server) Logout(ctx context.Context, _ *connect.Request[emptypb.Empty]) (*connect.Response[emptypb.Empty], error) {
	state, err := s.requireUser(ctx)
	if err != nil {
		return nil, err
	}
	_ = s.store.DeleteSession(ctx, state.sessionID)
	response := emptyResponse()
	response.Header().Set("Set-Cookie", s.clearCookie().String())
	return response, nil
}

func (s *Server) Me(ctx context.Context, _ *connect.Request[emptypb.Empty]) (*connect.Response[panelv1.MeResponse], error) {
	state, err := s.requireUser(ctx)
	if err != nil {
		return nil, err
	}
	user := protoUser(state.user)
	user.Online = true
	return connect.NewResponse(&panelv1.MeResponse{
		User:           user,
		SecurityAccess: state.user.Role == db.RoleAdmin,
		AgentsAccess:   state.user.Role == db.RoleAdmin,
	}), nil
}

func (s *Server) ChangePassword(ctx context.Context, request *connect.Request[panelv1.ChangePasswordRequest]) (*connect.Response[emptypb.Empty], error) {
	state, err := s.requireUser(ctx)
	if err != nil {
		return nil, err
	}
	if !auth.VerifyPassword(state.user.PasswordHash, request.Msg.CurrentPassword) {
		return nil, invalid("текущий пароль указан неверно")
	}
	if err := auth.ValidatePassword(request.Msg.NewPassword, state.user.Login); err != nil {
		return nil, invalid(err.Error())
	}
	hash, err := auth.HashPassword(request.Msg.NewPassword)
	if err != nil {
		return nil, rpcError(err, "не удалось изменить пароль")
	}
	if err := s.store.UpdateUser(ctx, state.user.ID, state.user.Role, &hash); err != nil {
		return nil, rpcError(err, "не удалось изменить пароль")
	}
	if err := s.store.DeleteUserSessions(ctx, state.user.ID, state.sessionID); err != nil {
		s.log.Warn("delete other sessions", "error", err)
	}
	return emptyResponse(), nil
}

func (s *Server) UpdateProfile(ctx context.Context, request *connect.Request[panelv1.UpdateProfileRequest]) (*connect.Response[panelv1.User], error) {
	state, err := s.requireUser(ctx)
	if err != nil {
		return nil, err
	}
	login := strings.TrimSpace(request.Msg.Login)
	if err := auth.ValidateLogin(login); err != nil {
		return nil, invalid(err.Error())
	}
	if login == state.user.Login {
		return connect.NewResponse(protoUser(state.user)), nil
	}
	if err := s.store.UpdateLogin(ctx, state.user.ID, login); err != nil {
		if errors.Is(err, db.ErrConflict) {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("такой логин уже занят"))
		}
		return nil, rpcError(err, "не удалось изменить логин")
	}
	state.user.Login = login
	return connect.NewResponse(protoUser(state.user)), nil
}

func (s *Server) loginFailure(ctx context.Context, username, ip, userAgent, reason string) {
	s.loginLog(ctx, username, false, reason, ip, userAgent)
	count, err := s.store.CountFailedLogins(ctx, ip, db.Now()-10*60*1000)
	if err != nil || count < 5 {
		return
	}
	if _, err := s.store.ActiveBan(ctx, ip, db.Now()); err == nil {
		return
	}
	country, resolved := geoip.Classify(ip)
	ban, err := s.store.CreateBan(ctx, &db.Ban{
		IP:          ip,
		Reason:      "5 неудачных попыток входа за 10 минут",
		Source:      "auto",
		Country:     country.Name,
		CountryCode: country.Code,
		BannedAt:    db.Now(),
		ExpiresAt:   db.Now() + 60*60*1000,
	})
	if err != nil {
		s.log.Warn("create auto ban", "error", err)
		return
	}
	s.publisher.BanChanged(ban)
	if !resolved {
		pending := *ban
		go s.resolveBanCountry(&pending)
	}
}

func timeMilliLabel(millis int64) string {
	return time.UnixMilli(millis).Local().Format("02.01.2006 15:04")
}
