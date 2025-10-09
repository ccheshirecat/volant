package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/volantvm/volant/internal/drift/config"
)

// Daemon coordinates HTTP serving and graceful shutdown for Drift.
type Daemon struct {
	cfg    config.Config
	logger *slog.Logger
	http   *http.Server
}

// New constructs a Daemon with the provided configuration and handler.
func New(cfg config.Config, logger *slog.Logger, handler http.Handler) *Daemon {
	return &Daemon{
		cfg:    cfg,
		logger: logger,
		http: &http.Server{
			Addr:    cfg.HTTPListen,
			Handler: handler,
		},
	}
}

// Run starts the HTTP server and blocks until the context is canceled.
func (d *Daemon) Run(ctx context.Context) error {
	serverErr := make(chan error, 1)
	go func() {
		d.logger.Info("http server starting", "addr", d.cfg.HTTPListen)
		if err := d.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := d.http.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return nil
	case err := <-serverErr:
		return err
	}
}
