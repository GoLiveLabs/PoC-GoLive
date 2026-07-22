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

	"github.com/google/uuid"

	"live-orchestrator/backend/internal/broadcast"
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
	"live-orchestrator/backend/internal/positions"
	"live-orchestrator/backend/internal/scenes"
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
	positionsStore := positions.NewFileStore(cfg.PositionsStorePath)
	scenesStore := scenes.NewFileStore(cfg.ScenesStorePath)
	orch := orchestrator.New(msClient, obsCtl, hub, cfg.ProgramScene, cfg.SyncInterval, cfg.MediaSourceBaseURL, positionsStore, scenesStore)

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

	programStreamURL := strings.TrimRight(cfg.MediaSourceBaseURL, "/") + "/" + strings.TrimLeft(cfg.ProgramStreamPath, "/")
	bcast := broadcast.NewManager(
		obsCtl,
		&liveIDProviderAdapter{svc: liveIDSvc},
		programStreamURL,
		&broadcast.ExecRunner{Bin: cfg.FFmpegBin},
		hub,
		&liveChecker{orch: orch},
	)

	apiServer := httpapi.NewServer(orch, hub, cfg.APIToken, clientSvc, ingestSvc, platformSvc, liveIDSvc, bcast)
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
	_ = bcast.Stop(context.Background())
	apiServer.CloseAllWS()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("http server shutdown error", "error", err)
	}

	obsCtl.Close()
	slog.Info("shutdown complete")
}

type liveIDProviderAdapter struct {
	svc *liveid.Service
}

func (a *liveIDProviderAdapter) ListActiveForClient(ctx context.Context, clientID uuid.UUID) ([]broadcast.Destination, error) {
	rows, err := a.svc.ListActiveForClient(ctx, clientID)
	if err != nil {
		return nil, err
	}
	out := make([]broadcast.Destination, len(rows))
	for i, r := range rows {
		out[i] = broadcast.Destination{
			LiveID:       r.LiveID,
			PlatformName: r.PlatformName,
			PushURL:      r.PushURL,
		}
	}
	return out, nil
}

type liveChecker struct {
	orch *orchestrator.Orchestrator
}

func (l *liveChecker) SomethingLive() bool {
	return l.orch.LiveState().LiveKind != orchestrator.LiveKindNone
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
