package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/ccheshirecat/volant/internal/server/app"
	"github.com/ccheshirecat/volant/internal/server/config"
	"github.com/ccheshirecat/volant/internal/server/db/sqlite"
	"github.com/ccheshirecat/volant/internal/server/eventbus/memory"
	"github.com/ccheshirecat/volant/internal/server/httpapi"
	"github.com/ccheshirecat/volant/internal/server/orchestrator"
	"github.com/ccheshirecat/volant/internal/server/orchestrator/cloudhypervisor"
	"github.com/ccheshirecat/volant/internal/server/orchestrator/network"
	"github.com/ccheshirecat/volant/internal/server/plugins"
	"github.com/ccheshirecat/volant/internal/shared/logging"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger := logging.New("volantd")

	cfg, err := config.FromEnv()
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}

	store, err := sqlite.Open(ctx, cfg.DatabasePath)
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}

	subnet := parseSubnetOrExit(cfg.SubnetCIDR, logger)
	hostIP := net.ParseIP(cfg.HostIP)
	if hostIP == nil {
		logger.Error("parse host ip", "ip", cfg.HostIP)
		os.Exit(1)
	}

	runtimeDir := expandPath(cfg.RuntimeDir, logger)
	logDir := expandPath(cfg.LogDir, logger)

	launcher := cloudhypervisor.New(
		cfg.HypervisorBinary,
		expandPath(cfg.KernelImagePath, logger),
		expandPath(cfg.InitramfsPath, logger),
		runtimeDir,
		logDir,
	)

	runtimeRegistry := plugins.NewRegistry(store.Queries().Plugins())

	var netManager network.Manager
	if runtime.GOOS == "linux" {
		netManager = network.NewBridgeManager(cfg.BridgeName)
	} else {
		logger.Warn("using noop network manager (non-linux host)")
		netManager = network.NewNoop()
	}

	events := memory.New()

	engine, err := orchestrator.New(orchestrator.Params{
		Store:            store,
		Logger:           logger,
		Subnet:           subnet,
		HostIP:           hostIP,
		APIListenAddr:    cfg.APIListenAddr,
		APIAdvertiseAddr: cfg.APIAdvertiseAddr,
		Launcher:         launcher,
		Network:          netManager,
		Bus:              events,
		RuntimeDir:       runtimeDir,
	})
	if err != nil {
		logger.Error("init orchestrator", "error", err)
		os.Exit(1)
	}

    artifactRoot := filepath.Join(runtimeDir, "plugins")
    handler := httpapi.New(logger, engine, events, runtimeRegistry, artifactRoot)

	daemon, err := app.New(cfg, logger, store, engine, events, runtimeRegistry, handler)
	if err != nil {
		logger.Error("init app", "error", err)
		os.Exit(1)
	}

	if err := daemon.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("daemon exit", "error", err)
		os.Exit(1)
	}
}

func parseSubnetOrExit(cidr string, logger *slog.Logger) *net.IPNet {
	_, subnet, err := net.ParseCIDR(cidr)
	if err != nil {
		logger.Error("parse subnet", "cidr", cidr, "error", err)
		os.Exit(1)
	}
	return subnet
}

func expandPath(path string, logger *slog.Logger) string {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			logger.Error("resolve home", "error", err)
			os.Exit(1)
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	return path
}
