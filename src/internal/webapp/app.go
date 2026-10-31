package webapp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	agentv1 "dockium/gen/agent/v1"
	"dockium/internal/control"
	"dockium/internal/db"
	"dockium/internal/hub"
	"dockium/internal/panelapi"
	"dockium/internal/realtime"
	"dockium/internal/scheduler"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

type Config struct {
	Addr          string
	AgentAddr     string
	DataDir       string
	WebDir        string
	AgentEndpoint string
	Version       string
	TrustProxy    bool
	SecureCookies bool
	TLSCert       string
	TLSKey        string
	AgentTLSCert  string
	AgentTLSKey   string
}

type App struct {
	cfg        Config
	log        *slog.Logger
	store      *db.DB
	publisher  *realtime.Publisher
	agentHub   *hub.Hub
	controller *control.Controller
	scheduler  *scheduler.Scheduler
	httpServer *http.Server
	grpcServer *grpc.Server
	listeners  []net.Listener
}

func New(ctx context.Context, cfg Config, log *slog.Logger) (*App, error) {
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	store, err := db.Open(ctx, filepath.Join(cfg.DataDir, "panel.db"))
	if err != nil {
		return nil, err
	}
	if err := store.Migrate(ctx); err != nil {
		store.Close()
		return nil, err
	}
	if err := store.MarkAllAgentsOffline(ctx); err != nil {
		store.Close()
		return nil, err
	}
	publisher := realtime.NewPublisher()
	agentHub := hub.New(store, publisher, log)
	controller := control.New(store, agentHub, publisher, log)
	taskScheduler := scheduler.New(store, controller, agentHub, log)
	api := panelapi.New(panelapi.Options{
		Store:         store,
		Hub:           agentHub,
		Publisher:     publisher,
		Control:       controller,
		Scheduler:     taskScheduler,
		Log:           log,
		Version:       cfg.Version,
		SecureCookies: cfg.SecureCookies,
		TrustProxy:    cfg.TrustProxy,
		AgentEndpoint: cfg.AgentEndpoint,
	})
	apiHandler, err := api.Handler()
	if err != nil {
		store.Close()
		return nil, err
	}
	app := &App{
		cfg:        cfg,
		log:        log,
		store:      store,
		publisher:  publisher,
		agentHub:   agentHub,
		controller: controller,
		scheduler:  taskScheduler,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(writer).Encode(map[string]string{"status": "ok", "version": cfg.Version})
	})
	mux.Handle("/", app.staticOrAPI(apiHandler))
	handler := app.securityHeaders(mux)
	app.httpServer = &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
	}
	return app, nil
}

func (a *App) Run(ctx context.Context) error {
	httpListener, err := net.Listen("tcp", a.cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", a.cfg.Addr, err)
	}
	agentListener, err := net.Listen("tcp", a.cfg.AgentAddr)
	if err != nil {
		httpListener.Close()
		return fmt.Errorf("listen agents %s: %w", a.cfg.AgentAddr, err)
	}
	a.listeners = []net.Listener{httpListener, agentListener}

	var serverOptions []grpc.ServerOption
	serverOptions = append(serverOptions,
		grpc.MaxRecvMsgSize(64<<20),
		grpc.MaxSendMsgSize(64<<20),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{MinTime: 10 * time.Second, PermitWithoutStream: true}),
		grpc.KeepaliveParams(keepalive.ServerParameters{Time: 30 * time.Second, Timeout: 10 * time.Second}),
	)
	if a.cfg.AgentTLSCert != "" && a.cfg.AgentTLSKey != "" {
		creds, err := credentials.NewServerTLSFromFile(a.cfg.AgentTLSCert, a.cfg.AgentTLSKey)
		if err != nil {
			return fmt.Errorf("agent TLS: %w", err)
		}
		serverOptions = append(serverOptions, grpc.Creds(creds))
	}
	a.grpcServer = grpc.NewServer(serverOptions...)
	agentv1.RegisterAgentServiceServer(a.grpcServer, &agentServer{hub: a.agentHub})

	go a.agentHub.Run(ctx)
	go a.scheduler.Run(ctx)
	go a.cleanupLoop(ctx)
	go func() {
		if err := a.grpcServer.Serve(agentListener); err != nil && ctx.Err() == nil {
			a.log.Error("agent grpc server stopped", "error", err)
		}
	}()
	scheme := "http"
	if a.cfg.TLSCert != "" {
		scheme = "https"
	}
	a.log.Info("dockium panel started", "version", a.cfg.Version, "url", scheme+"://"+displayAddress(a.cfg.Addr), "agents", a.cfg.AgentAddr, "data", a.cfg.DataDir)
	go func() {
		var serveErr error
		if a.cfg.TLSCert != "" && a.cfg.TLSKey != "" {
			serveErr = a.httpServer.ServeTLS(httpListener, a.cfg.TLSCert, a.cfg.TLSKey)
		} else {
			serveErr = a.httpServer.Serve(httpListener)
		}
		if serveErr != nil && serveErr != http.ErrServerClosed {
			a.log.Error("http server stopped", "error", serveErr)
		}
	}()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = a.httpServer.Shutdown(shutdownCtx)
	stopped := make(chan struct{})
	go func() {
		a.grpcServer.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		a.grpcServer.Stop()
	}
	if err := a.store.Close(); err != nil {
		a.log.Warn("close database", "error", err)
	}
	a.log.Info("dockium panel stopped")
	return nil
}

func (a *App) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			_ = a.store.PurgeExpiredSessions(cleanupCtx, db.Now())
			_ = a.store.PurgeSecurityLogs(cleanupCtx, db.Now()-30*24*60*60*1000, db.Now()-7*24*60*60*1000)
			cancel()
		}
	}
}

func (a *App) staticOrAPI(apiHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.HasPrefix(request.URL.Path, "/dockium.panel.v1.") {
			apiHandler.ServeHTTP(writer, request)
			return
		}
		webDirectory := a.cfg.WebDir
		if webDirectory == "" {
			http.Error(writer, "интерфейс не собран: укажите -web-dir", http.StatusServiceUnavailable)
			return
		}
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		cleanPath := path.Clean("/" + request.URL.Path)
		if cleanPath == "/" {
			a.serveIndex(writer, request)
			return
		}
		filePath := filepath.Join(webDirectory, filepath.FromSlash(cleanPath))
		if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
			if strings.HasPrefix(cleanPath, "/assets/") {
				writer.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				writer.Header().Set("Cache-Control", "public, max-age=3600")
			}
			http.ServeFile(writer, request, filePath)
			return
		}
		a.serveIndex(writer, request)
	})
}

func (a *App) serveIndex(writer http.ResponseWriter, request *http.Request) {
	indexPath := filepath.Join(a.cfg.WebDir, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		http.Error(writer, "интерфейс не собран: "+indexPath, http.StatusServiceUnavailable)
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	http.ServeFile(writer, request, indexPath)
}

func (a *App) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		header := writer.Header()
		header.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("X-Frame-Options", "DENY")
		header.Set("Referrer-Policy", "no-referrer")
		header.Set("Cross-Origin-Opener-Policy", "same-origin")
		header.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if a.cfg.SecureCookies || a.cfg.TLSCert != "" {
			header.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(writer, request)
	})
}

type agentServer struct {
	agentv1.UnimplementedAgentServiceServer
	hub *hub.Hub
}

func (s *agentServer) Connect(stream agentv1.AgentService_ConnectServer) error {
	return s.hub.Connect(stream)
}

func displayAddress(address string) string {
	if strings.HasPrefix(address, ":") {
		return "127.0.0.1" + address
	}
	return address
}
