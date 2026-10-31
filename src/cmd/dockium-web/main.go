package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"dockium/internal/version"
	"dockium/internal/webapp"
)

func main() {
	var (
		addr          = flag.String("addr", env("DOCKIUM_ADDR", ":3434"), "адрес веб-интерфейса и API")
		agentAddr     = flag.String("agent-addr", env("DOCKIUM_AGENT_ADDR", ":3443"), "адрес gRPC-сервера для агентов")
		dataDir       = flag.String("data-dir", env("DOCKIUM_DATA_DIR", "data"), "каталог данных SQLite")
		webDir        = flag.String("web-dir", env("DOCKIUM_WEB_DIR", "web/dist"), "каталог собранного интерфейса")
		agentEndpoint = flag.String("agent-endpoint", env("DOCKIUM_AGENT_ENDPOINT", "localhost:3443"), "адрес панели для подключения агентов")
		trustProxy    = flag.Bool("trust-proxy", env("DOCKIUM_TRUST_PROXY", "") == "1", "доверять X-Forwarded-For от обратного прокси")
		secureCookies = flag.Bool("secure-cookies", env("DOCKIUM_SECURE_COOKIES", "") == "1", "только secure-cookie (за TLS)")
		tlsCert       = flag.String("tls-cert", env("DOCKIUM_TLS_CERT", ""), "сертификат TLS для web")
		tlsKey        = flag.String("tls-key", env("DOCKIUM_TLS_KEY", ""), "ключ TLS для web")
		agentTLSCert  = flag.String("agent-tls-cert", env("DOCKIUM_AGENT_TLS_CERT", ""), "сертификат TLS для gRPC агентов")
		agentTLSKey   = flag.String("agent-tls-key", env("DOCKIUM_AGENT_TLS_KEY", ""), "ключ TLS для gRPC агентов")
		logLevel      = flag.String("log-level", env("DOCKIUM_LOG_LEVEL", "info"), "уровень логирования: debug, info, warn, error")
	)
	flag.Parse()

	logger := newLogger(*logLevel)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app, err := webapp.New(ctx, webapp.Config{
		Addr:          *addr,
		AgentAddr:     *agentAddr,
		DataDir:       *dataDir,
		WebDir:        *webDir,
		AgentEndpoint: *agentEndpoint,
		Version:       version.Version,
		TrustProxy:    *trustProxy,
		SecureCookies: *secureCookies,
		TLSCert:       *tlsCert,
		TLSKey:        *tlsKey,
		AgentTLSCert:  *agentTLSCert,
		AgentTLSKey:   *agentTLSKey,
	}, logger)
	if err != nil {
		logger.Error("failed to start panel", "error", err)
		os.Exit(1)
	}
	if err := app.Run(ctx); err != nil {
		logger.Error("panel stopped with error", "error", err)
		os.Exit(1)
	}
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func newLogger(level string) *slog.Logger {
	var parsed slog.Level
	switch level {
	case "debug":
		parsed = slog.LevelDebug
	case "warn":
		parsed = slog.LevelWarn
	case "error":
		parsed = slog.LevelError
	default:
		parsed = slog.LevelInfo
	}
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: parsed})
	return slog.New(handler).With("service", "dockium-web")
}
