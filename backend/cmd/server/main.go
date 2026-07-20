package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"live-orchestrator/backend/internal/client"
	"live-orchestrator/backend/internal/config"
	"live-orchestrator/backend/internal/dbconn"
	"live-orchestrator/backend/internal/events"
	"live-orchestrator/backend/internal/httpapi"
	"live-orchestrator/backend/internal/ingest"
	"live-orchestrator/backend/internal/liveid"
	"live-orchestrator/backend/internal/mediaserver"
	"live-orchestrator/backend/internal/obs"
	"live-orchestrator/backend/internal/orchestrator"
	"live-orchestrator/backend/internal/streamplatform"
)

const shutdownTimeout = 3 * time.Second

func main() {
	cfg := config.Load()
	setupLogging(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	msClient := mediaserver.NewClient(cfg.MediaMTXAPIURL)
	obsCtl := obs.New(cfg.OBSAddr, cfg.OBSPassword)
	hub := events.NewHub()
	orch := orchestrator.New(msClient, obsCtl, hub, cfg.ProgramScene, cfg.SyncInterval, cfg.MediaSourceBaseURL)

	orchCtx, orchCancel := context.WithCancel(context.Background())
	go orch.Run(orchCtx)

	db, err := dbconn.Open(cfg.DatabaseURL)
	if err != nil {
		slog.Error("could not connect to database", "error", err)
		os.Exit(1)
	}

	clientSvc := client.NewService(db)
	platformSvc := streamplatform.NewService(db)
	ingestSvc := ingest.NewService(db, clientSvc)
	liveIDSvc := liveid.NewService(db, clientSvc, platformSvc)

	apiServer := httpapi.NewServer(orch, hub, cfg.APIToken, clientSvc, ingestSvc, platformSvc, liveIDSvc)
	httpServer := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: apiServer.Handler(),
	}

	go func() {
		slog.Info("http server listening", "addr", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server error", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	slog.Info("shutdown signal received, stopping gracefully")

	orchCancel()
	apiServer.CloseAllWS()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("http server shutdown error", "error", err)
	}

	obsCtl.Close()
	slog.Info("shutdown complete")
}

func setupLogging(level string) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})))
}
