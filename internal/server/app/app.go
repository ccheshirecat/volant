package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/ccheshirecat/overhyped/internal/server/config"
	"github.com/ccheshirecat/overhyped/internal/server/db"
	"github.com/ccheshirecat/overhyped/internal/server/eventbus"
	"github.com/ccheshirecat/overhyped/internal/server/orchestrator"
)

// App wires the config, persistence, orchestrator, and HTTP transport.
type App struct {
	cfg          config.ServerConfig
	logger       *slog.Logger
	store        db.Store
	engine       orchestrator.Engine
	events       eventbus.Bus
	httpServer   *http.Server
	shutdownWait time.Duration
}

// New constructs the daemon application. Dependencies that are not yet
// implemented should be passed as nil until their concrete types land.
func New(cfg config.ServerConfig, logger *slog.Logger, store db.Store, engine orchestrator.Engine, events eventbus.Bus, mux http.Handler) (*App, error) {
	if logger == nil {
		return nil, fmt.Errorf("logger must not be nil")
	}
	if mux == nil {
		mux = http.NewServeMux()
	}

	httpServer := &http.Server{
		Addr:         cfg.APIListenAddr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return &App{
		cfg:          cfg,
		logger:       logger,
		store:        store,
		engine:       engine,
		events:       events,
		httpServer:   httpServer,
		shutdownWait: 15 * time.Second,
	}, nil
}

// Run starts the orchestrator engine and HTTP server, blocking until context cancellation.
func (a *App) Run(ctx context.Context) error {
	if a.engine == nil {
		return fmt.Errorf("orchestrator engine not provided")
	}
	if err := a.engine.Start(ctx); err != nil {
		return fmt.Errorf("start orchestrator: %w", err)
	}

	errCh := make(chan error, 1)
	go func() {
		a.logger.Info("api server listening", "addr", a.httpServer.Addr)
		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), a.shutdownWait)
		defer cancel()
		if err := a.httpServer.Shutdown(shutdownCtx); err != nil {
			a.logger.Error("http shutdown", "error", err)
		}
		if err := a.engine.Stop(shutdownCtx); err != nil {
			a.logger.Error("engine stop", "error", err)
		}
		if a.store != nil {
			if err := a.store.Close(shutdownCtx); err != nil {
				a.logger.Error("store close", "error", err)
			}
		}
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}
