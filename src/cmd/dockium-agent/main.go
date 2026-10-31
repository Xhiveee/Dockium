package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"dockium/internal/agent"
	"dockium/internal/version"
)

func main() {
	var (
		panel      = flag.String("panel", env("DOCKIUM_PANEL_URL", "http://127.0.0.1:3443"), "адрес gRPC панели (https:// для TLS)")
		token      = flag.String("token", env("DOCKIUM_AGENT_TOKEN", ""), "токен агента из панели")
		dataDir    = flag.String("data-dir", env("DOCKIUM_AGENT_DATA", "data"), "каталог данных экземпляров")
		caFile     = flag.String("ca", env("DOCKIUM_AGENT_CA", ""), "CA сертификат панели")
		skipVerify = flag.Bool("skip-verify", env("DOCKIUM_AGENT_SKIP_VERIFY", "") == "1", "не проверять TLS-сертификат панели")
		serverName = flag.String("server-name", env("DOCKIUM_AGENT_TLS_SERVER_NAME", ""), "TLS server name панели")
		logLevel   = flag.String("log-level", env("DOCKIUM_LOG_LEVEL", "info"), "уровень логирования")
	)
	flag.Parse()
	logger := newLogger(*logLevel)
	if *token == "" {
		logger.Error("agent token is required", "hint", "укажите -token или DOCKIUM_AGENT_TOKEN")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := agent.Run(ctx, agent.Config{
		PanelURL:      *panel,
		Token:         *token,
		DataDir:       *dataDir,
		TLSCA:         *caFile,
		TLSSkipVerify: *skipVerify,
		TLSServerName: *serverName,
		Version:       version.Version,
	}, logger)
	if err != nil {
		logger.Error("agent stopped with error", "error", err)
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
	return slog.New(handler).With("service", "dockium-agent")
}
