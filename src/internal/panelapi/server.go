package panelapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"connectrpc.com/connect"
	emptypb "google.golang.org/protobuf/types/known/emptypb"

	panelv1 "dockium/gen/panel/v1"
	"dockium/gen/panel/v1/panelv1connect"
	"dockium/internal/auth"
	"dockium/internal/control"
	"dockium/internal/db"
	"dockium/internal/hub"
	"dockium/internal/present"
	"dockium/internal/realtime"
	"dockium/internal/scheduler"
)

const (
	secondTimeout = time.Second
	minuteTimeout = time.Minute
)

type Options struct {
	Store         *db.DB
	Hub           *hub.Hub
	Publisher     *realtime.Publisher
	Control       *control.Controller
	Scheduler     *scheduler.Scheduler
	Log           *slog.Logger
	Version       string
	SecureCookies bool
	TrustProxy    bool
	AgentEndpoint string
}

type Server struct {
	store         *db.DB
	hub           *hub.Hub
	publisher     *realtime.Publisher
	control       *control.Controller
	scheduler     *scheduler.Scheduler
	log           *slog.Logger
	version       string
	secureCookies bool
	trustProxy    bool
	agentEndpoint string
}

func New(options Options) *Server {
	return &Server{
		store:         options.Store,
		hub:           options.Hub,
		publisher:     options.Publisher,
		control:       options.Control,
		scheduler:     options.Scheduler,
		log:           options.Log,
		version:       options.Version,
		secureCookies: options.SecureCookies,
		trustProxy:    options.TrustProxy,
		agentEndpoint: options.AgentEndpoint,
	}
}

func (s *Server) Handler() (http.Handler, error) {
	guard := &guard{server: s}
	options := []connect.HandlerOption{
		connect.WithInterceptors(guard),
		connect.WithReadMaxBytes(34 << 20),
		connect.WithSendMaxBytes(16 << 20),
	}
	mux := http.NewServeMux()
	mount := func(path string, handler http.Handler) {
		mux.Handle(path, handler)
	}
	paths := []struct {
		path    string
		handler http.Handler
	}{}
	add := func(path string, handler http.Handler) {
		paths = append(paths, struct {
			path    string
			handler http.Handler
		}{path, handler})
	}
	path, handler := panelv1connect.NewSetupServiceHandler(&setupService{s}, options...)
	add(path, handler)
	path, handler = panelv1connect.NewAuthServiceHandler(&authService{s}, options...)
	add(path, handler)
	path, handler = panelv1connect.NewUserServiceHandler(&userService{s}, options...)
	add(path, handler)
	path, handler = panelv1connect.NewAgentServiceHandler(&agentService{s}, options...)
	add(path, handler)
	path, handler = panelv1connect.NewInstanceServiceHandler(&instanceService{s}, options...)
	add(path, handler)
	path, handler = panelv1connect.NewConsoleServiceHandler(&consoleService{s}, options...)
	add(path, handler)
	path, handler = panelv1connect.NewFileServiceHandler(&fileService{s}, options...)
	add(path, handler)
	path, handler = panelv1connect.NewDockerServiceHandler(&dockerService{s}, options...)
	add(path, handler)
	path, handler = panelv1connect.NewScheduleServiceHandler(&scheduleService{s}, options...)
	add(path, handler)
	path, handler = panelv1connect.NewSecurityServiceHandler(&securityService{s}, options...)
	add(path, handler)
	path, handler = panelv1connect.NewSecurityEventsServiceHandler(&securityEventsService{s}, options...)
	add(path, handler)
	path, handler = panelv1connect.NewDashboardServiceHandler(&dashboardService{s}, options...)
	add(path, handler)
	path, handler = panelv1connect.NewSettingsServiceHandler(&settingsService{s}, options...)
	add(path, handler)
	for _, item := range paths {
		mount(item.path, item.handler)
	}
	return mux, nil
}

type principal struct {
	user      *db.User
	sessionID string
	ip        string
}

type contextKey int

const principalKey contextKey = 0

func principalFrom(ctx context.Context) (*principal, bool) {
	value, ok := ctx.Value(principalKey).(*principal)
	return value, ok
}

type guard struct {
	server *Server
}

var publicProcedures = map[string]bool{
	panelv1connect.SetupServiceGetStatusProcedure:   true,
	panelv1connect.SetupServiceCreateAdminProcedure: true,
	panelv1connect.AuthServiceLoginProcedure:        true,
}

var adminPrefixes = []string{
	panelv1connect.UserServiceName,
	panelv1connect.AgentServiceName,
	panelv1connect.DockerServiceName,
	panelv1connect.SecurityServiceName,
	panelv1connect.SecurityEventsServiceName,
}

func adminProcedure(procedure string) bool {
	for _, prefix := range adminPrefixes {
		if strings.HasPrefix(procedure, "/"+prefix+"/") {
			return true
		}
	}
	return false
}

func (g *guard) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, request connect.AnyRequest) (response connect.AnyResponse, err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				g.server.log.Error("panic in request", "procedure", request.Spec().Procedure, "panic", recovered)
				err = connect.NewError(connect.CodeInternal, errors.New("внутренняя ошибка панели"))
			}
		}()
		if originErr := g.server.verifyOrigin(request.Header()); originErr != nil {
			return nil, originErr
		}
		ctx, state, authErr := g.server.prepare(ctx, request.Spec().Procedure, request.Header(), request.Peer().Addr)
		if authErr != nil {
			g.server.recordRequest(ctx, request.Spec().Procedure, state, authErr)
			return nil, authErr
		}
		response, err = next(ctx, request)
		g.server.recordRequest(ctx, request.Spec().Procedure, state, err)
		return response, err
	}
}

func (g *guard) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (g *guard) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, connection connect.StreamingHandlerConn) (err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				g.server.log.Error("panic in stream", "procedure", connection.Spec().Procedure, "panic", recovered)
				err = connect.NewError(connect.CodeInternal, errors.New("внутренняя ошибка панели"))
			}
		}()
		if originErr := g.server.verifyOrigin(connection.RequestHeader()); originErr != nil {
			return originErr
		}
		ctx, state, authErr := g.server.prepare(ctx, connection.Spec().Procedure, connection.RequestHeader(), connection.Peer().Addr)
		if authErr != nil {
			g.server.recordRequest(ctx, connection.Spec().Procedure, state, authErr)
			return authErr
		}
		err = next(ctx, connection)
		g.server.recordRequest(ctx, connection.Spec().Procedure, state, err)
		return err
	}
}

func (s *Server) prepare(ctx context.Context, procedure string, header http.Header, peer string) (context.Context, *principal, error) {
	ip := s.clientIP(header, peer)
	if publicProcedures[procedure] {
		state := &principal{ip: ip}
		return context.WithValue(ctx, principalKey, state), state, nil
	}
	state, err := s.authenticate(ctx, header, ip)
	if err != nil {
		return ctx, nil, err
	}
	if adminProcedure(procedure) && state.user.Role != db.RoleAdmin {
		return ctx, state, connect.NewError(connect.CodePermissionDenied, errors.New("недостаточно прав"))
	}
	return context.WithValue(ctx, principalKey, state), state, nil
}

func (s *Server) authenticate(ctx context.Context, header http.Header, ip string) (*principal, error) {
	if ban, err := s.store.ActiveBan(ctx, ip, db.Now()); err == nil {
		until := time.UnixMilli(ban.ExpiresAt).Format("02.01.2006 15:04")
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("доступ с адреса %s заблокирован до %s", ip, until))
	}
	request := &http.Request{Header: header}
	cookie, err := request.Cookie(auth.SessionCookieName)
	if err != nil || cookie.Value == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("требуется вход в панель"))
	}
	sessionID := auth.HashSessionID(cookie.Value)
	session, err := s.store.SessionByID(ctx, sessionID)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("сессия не найдена"))
	}
	now := db.Now()
	if session.ExpiresAt <= now {
		_ = s.store.DeleteSession(ctx, session.ID)
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("сессия истекла, войдите заново"))
	}
	user, err := s.store.UserByID(ctx, session.UserID)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("пользователь не найден"))
	}
	if now-session.LastSeenAt > 60_000 {
		_ = s.store.TouchSession(ctx, session.ID, now)
	}
	return &principal{user: user, sessionID: session.ID, ip: ip}, nil
}

func (s *Server) verifyOrigin(header http.Header) error {
	origin := header.Get("Origin")
	if origin == "" {
		return nil
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return connect.NewError(connect.CodePermissionDenied, errors.New("запрос отклонён"))
	}
	host := header.Get("Host")
	if s.trustProxy {
		if forwarded := header.Get("X-Forwarded-Host"); forwarded != "" {
			host = forwarded
		}
	}
	if !strings.EqualFold(parsed.Host, host) {
		if loopbackHost(parsed.Host) && loopbackHost(host) {
			return nil
		}
		return connect.NewError(connect.CodePermissionDenied, errors.New("запрос с другого источника отклонён"))
	}
	return nil
}

func loopbackHost(value string) bool {
	name, _, err := net.SplitHostPort(value)
	if err != nil {
		name = value
	}
	if name == "localhost" {
		return true
	}
	ip := net.ParseIP(strings.Trim(name, "[]"))
	return ip != nil && ip.IsLoopback()
}

func (s *Server) clientIP(header http.Header, peer string) string {
	if s.trustProxy {
		if forwarded := header.Get("X-Forwarded-For"); forwarded != "" {
			first := strings.TrimSpace(strings.Split(forwarded, ",")[0])
			if net.ParseIP(first) != nil {
				return first
			}
		}
		if realIP := strings.TrimSpace(header.Get("X-Real-IP")); net.ParseIP(realIP) != nil {
			return realIP
		}
	}
	host, _, err := net.SplitHostPort(peer)
	if err == nil {
		return host
	}
	return peer
}

func (s *Server) recordRequest(ctx context.Context, procedure string, state *principal, callErr error) {
	if procedure == panelv1connect.SetupServiceGetStatusProcedure || procedure == panelv1connect.AuthServiceMeProcedure {
		return
	}
	login := "anonymous"
	ip := ""
	if state != nil {
		ip = state.ip
		if state.user != nil {
			login = state.user.Login
		}
	}
	status := 200
	if callErr != nil {
		status = httpStatus(connect.CodeOf(callErr))
	}
	entry := &db.ApiRequest{Login: login, Procedure: strings.TrimPrefix(procedure, "/"), Status: status, IP: ip, CreatedAt: db.Now()}
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	id, err := s.store.InsertApiRequest(saveCtx, entry)
	if err != nil {
		s.log.Warn("write api request log", "error", err)
		return
	}
	entry.ID = id
	s.publisher.ApiRequested(entry)
}

func httpStatus(code connect.Code) int {
	switch code {
	case connect.CodeInvalidArgument:
		return 400
	case connect.CodeUnauthenticated:
		return 401
	case connect.CodePermissionDenied:
		return 403
	case connect.CodeNotFound:
		return 404
	case connect.CodeAlreadyExists:
		return 409
	case connect.CodeFailedPrecondition:
		return 412
	case connect.CodeResourceExhausted:
		return 429
	case connect.CodeDeadlineExceeded:
		return 504
	case connect.CodeUnavailable:
		return 503
	case connect.CodeCanceled:
		return 499
	case connect.CodeInternal:
		return 500
	default:
		return 0
	}
}

func (s *Server) sessionCookie(ctx context.Context, user *db.User, ip, userAgent string) (*http.Cookie, error) {
	raw, err := auth.SessionID()
	if err != nil {
		return nil, err
	}
	now := db.Now()
	session := &db.Session{
		ID:         auth.HashSessionID(raw),
		UserID:     user.ID,
		CreatedAt:  now,
		LastSeenAt: now,
		ExpiresAt:  now + auth.SessionTTL.Milliseconds(),
		IP:         ip,
		UserAgent:  userAgent,
	}
	if err := s.store.CreateSession(ctx, session); err != nil {
		return nil, err
	}
	return &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    raw,
		Path:     "/",
		MaxAge:   int(auth.SessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteStrictMode,
	}, nil
}

func (s *Server) clearCookie() *http.Cookie {
	return &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteStrictMode,
	}
}

func (s *Server) loginLog(ctx context.Context, login string, success bool, reason, ip, userAgent string) *db.LoginLog {
	browser, osName := auth.ParseUserAgent(userAgent)
	entry := &db.LoginLog{
		Login:     login,
		Success:   success,
		Reason:    reason,
		IP:        ip,
		UserAgent: userAgent,
		Browser:   browser,
		OS:        osName,
		CreatedAt: db.Now(),
	}
	id, err := s.store.InsertLoginLog(ctx, entry)
	if err != nil {
		s.log.Warn("write login log", "error", err)
		return entry
	}
	entry.ID = id
	s.publisher.LoginLogged(entry)
	return entry
}

func (s *Server) requireUser(ctx context.Context) (*principal, error) {
	state, ok := principalFrom(ctx)
	if !ok || state.user == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("требуется вход в панель"))
	}
	return state, nil
}

func (s *Server) requireAdmin(ctx context.Context) (*principal, error) {
	state, err := s.requireUser(ctx)
	if err != nil {
		return nil, err
	}
	if state.user.Role != db.RoleAdmin {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("недостаточно прав"))
	}
	return state, nil
}

func (s *Server) instanceFor(ctx context.Context, id int64, check func(db.Permission) bool, message string) (*db.Instance, db.Permission, error) {
	state, err := s.requireUser(ctx)
	if err != nil {
		return nil, db.Permission{}, err
	}
	instance, err := s.store.InstanceByID(ctx, id)
	if err != nil {
		return nil, db.Permission{}, rpcError(err, "экземпляр не найден")
	}
	if state.user.Role == db.RoleAdmin {
		permission := db.Permission{Control: true, Console: true, Files: true}
		return instance, permission, nil
	}
	permission, err := s.store.Permission(ctx, state.user.ID, id)
	if err != nil {
		return nil, db.Permission{}, connect.NewError(connect.CodeNotFound, errors.New("экземпляр не найден"))
	}
	if check != nil && !check(permission) {
		return nil, db.Permission{}, connect.NewError(connect.CodePermissionDenied, errors.New(message))
	}
	return instance, permission, nil
}

func rpcError(err error, fallback string) error {
	if err == nil {
		return nil
	}
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		return err
	}
	switch {
	case errors.Is(err, db.ErrNotFound):
		return connect.NewError(connect.CodeNotFound, errors.New(fallback))
	case errors.Is(err, db.ErrConflict):
		return connect.NewError(connect.CodeAlreadyExists, errors.New("запись уже существует"))
	case errors.Is(err, hub.ErrOffline):
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("агент офлайн: действие недоступно"))
	case errors.Is(err, context.DeadlineExceeded):
		return connect.NewError(connect.CodeDeadlineExceeded, errors.New("агент не ответил вовремя"))
	}
	var commandErr *hub.CommandError
	if errors.As(err, &commandErr) {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New(commandErr.Message))
	}
	var timeoutErr *hub.TimeoutError
	if errors.As(err, &timeoutErr) {
		return connect.NewError(connect.CodeDeadlineExceeded, errors.New("агент не ответил вовремя"))
	}
	return connect.NewError(connect.CodeInternal, errors.New(fallback))
}

func invalid(message string) error {
	return connect.NewError(connect.CodeInvalidArgument, errors.New(message))
}

func failedPrecondition(message string) error {
	return connect.NewError(connect.CodeFailedPrecondition, errors.New(message))
}

func normalizePage(page *panelv1.Page) (int, int) {
	limit := 50
	offset := 0
	if page != nil {
		if page.Limit > 0 {
			limit = int(page.Limit)
		}
		if page.Offset > 0 {
			offset = int(page.Offset)
		}
	}
	if limit > 200 {
		limit = 200
	}
	return limit, offset
}

func newStreamID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer)
}

func (s *Server) visibleInstances(ctx context.Context) ([]*db.Instance, db.Permission, error) {
	state, err := s.requireUser(ctx)
	if err != nil {
		return nil, db.Permission{}, err
	}
	if state.user.Role == db.RoleAdmin {
		instances, err := s.store.ListInstances(ctx)
		return instances, db.Permission{Control: true, Console: true, Files: true}, err
	}
	instances, err := s.store.ListUserInstances(ctx, state.user.ID)
	if err != nil {
		return nil, db.Permission{}, err
	}
	return instances, db.Permission{}, nil
}

func emptyResponse() *connect.Response[emptypb.Empty] {
	return connect.NewResponse(&emptypb.Empty{})
}

func protoUser(user *db.User) *panelv1.User {
	return present.User(user, false)
}
