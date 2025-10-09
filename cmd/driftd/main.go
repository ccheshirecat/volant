package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/volantvm/volant/internal/drift/app"
	"github.com/volantvm/volant/internal/drift/config"
	"github.com/volantvm/volant/internal/drift/controller"
	"github.com/volantvm/volant/internal/drift/dataplane"
	"github.com/volantvm/volant/internal/drift/httpapi"
	"github.com/volantvm/volant/internal/drift/routes"
	"github.com/volantvm/volant/internal/drift/vsockproxy"
	"github.com/volantvm/volant/internal/shared/logging"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger := logging.New("driftd")

	cfg, err := config.FromEnv()
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(cfg.StateDir, 0o755); err != nil {
		logger.Error("ensure state dir", "path", cfg.StateDir, "error", err)
		os.Exit(1)
	}

	store, err := routes.NewFileStore(cfg.RoutesPath)
	if err != nil {
		logger.Error("init route store", "error", err)
		os.Exit(1)
	}

	var dp dataplane.Interface
	if manager, err := dataplane.New(dataplane.Options{
		ObjectPath: cfg.BPFObjectPath,
		Interface:  cfg.BridgeName,
		Logger:     logger,
	}); err != nil {
		if errors.Is(err, dataplane.ErrUnsupported) {
			logger.Warn("dataplane unavailable on this platform", "error", err)
		} else {
			logger.Error("initialize dataplane", "error", err)
			os.Exit(1)
		}
	} else {
		dp = manager
		defer dp.Close()
	}

	var vsockMgr vsockproxy.Manager
	if mgr, err := vsockproxy.New(vsockproxy.Options{Logger: logger}); err != nil {
		if errors.Is(err, vsockproxy.ErrUnsupported) {
			logger.Warn("vsock proxy unavailable on this platform", "error", err)
		} else {
			logger.Error("initialize vsock proxy", "error", err)
			os.Exit(1)
		}
	} else {
		vsockMgr = mgr
		defer vsockMgr.Close()
	}

	ctrl := controller.New(store, dp, vsockMgr)
	if err := ctrl.Restore(context.Background()); err != nil {
		logger.Error("restore routes", "error", err)
		os.Exit(1)
	}

	handler := httpapi.New(ctrl)
	daemon := app.New(cfg, logger, handler)

	if err := daemon.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("daemon exit", "error", err)
		os.Exit(1)
	}
	logger.Info("shutdown complete", "addr", cfg.HTTPListen)
}
