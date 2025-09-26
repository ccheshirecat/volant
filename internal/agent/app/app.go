package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	agentruntime "github.com/ccheshirecat/volant/internal/agent/runtime"
)

const (
	defaultListenAddr     = ":8080"
	defaultTimeoutEnvKey  = "volant_AGENT_DEFAULT_TIMEOUT"
	defaultListenEnvKey   = "volant_AGENT_LISTEN_ADDR"
	defaultRuntimeEnvKey  = "volant_AGENT_RUNTIME"
	defaultRemoteAddrKey  = "volant_AGENT_REMOTE_DEBUGGING_ADDR"
	defaultRemotePortKey  = "volant_AGENT_REMOTE_DEBUGGING_PORT"
	defaultUserDataDirKey = "volant_AGENT_USER_DATA_DIR"
	defaultExecPathKey    = "volant_AGENT_EXEC_PATH"
)

type Config struct {
	ListenAddr          string
	Runtime             string
	RemoteDebuggingAddr string
	RemoteDebuggingPort int
	UserDataDir         string
	ExecPath            string
	DefaultTimeout      time.Duration
}

type App struct {
	cfg     Config
	runtime agentruntime.Runtime
	server  *http.Server
	timeout time.Duration
	log     *log.Logger
	started time.Time
}

func Run(ctx context.Context) error {
	cfg := loadConfig()
	logger := log.New(os.Stdout, "volary: ", log.LstdFlags|log.LUTC)

	runtimeName := strings.TrimSpace(cfg.Runtime)
	if runtimeName == "" {
		return errors.New("runtime not specified")
	}

	opts := agentruntime.Options{
		DefaultTimeout: cfg.DefaultTimeout,
		Config: map[string]string{
			"remote_debugging_addr": cfg.RemoteDebuggingAddr,
			"remote_debugging_port": strconv.Itoa(cfg.RemoteDebuggingPort),
			"user_data_dir":         cfg.UserDataDir,
			"exec_path":             cfg.ExecPath,
		},
	}

	runtimeInstance, err := agentruntime.New(ctx, runtimeName, opts)
	if err != nil {
		return err
	}
	defer runtimeInstance.Shutdown(context.Background())

	app := &App{
		cfg:     cfg,
		runtime: runtimeInstance,
		timeout: cfg.DefaultTimeout,
		log:     logger,
		started: time.Now().UTC(),
	}
	return app.run(ctx)
}

func (a *App) run(ctx context.Context) error {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)
	router.Use(middleware.Timeout(a.timeout + 30*time.Second))

	router.Get("/healthz", a.handleHealth)

	router.Route("/v1", func(r chi.Router) {
		r.Get("/devtools", a.handleDevTools)
		r.Get("/logs/stream", a.handleLogs)
	})

	server := &http.Server{
		Addr:         a.cfg.ListenAddr,
		Handler:      router,
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		a.log.Printf("listening on %s", a.cfg.ListenAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			a.log.Printf("shutdown error: %v", err)
		}
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

func loadConfig() Config {
	listen := envOrDefault(defaultListenEnvKey, defaultListenAddr)
	runtimeName := envOrDefault(defaultRuntimeEnvKey, "")
	remoteAddr := os.Getenv(defaultRemoteAddrKey)
	remotePort := envIntOrDefault(defaultRemotePortKey, 0)
	userDataDir := os.Getenv(defaultUserDataDirKey)
	execPath := os.Getenv(defaultExecPathKey)

	defaultTimeout := parseDurationEnv(defaultTimeoutEnvKey, 0)

	return Config{
		ListenAddr:          listen,
		Runtime:             runtimeName,
		RemoteDebuggingAddr: remoteAddr,
		RemoteDebuggingPort: remotePort,
		UserDataDir:         userDataDir,
		ExecPath:            execPath,
		DefaultTimeout:      defaultTimeout,
	}
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envIntOrDefault(key string, fallback int) int {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}

func parseDurationEnv(key string, fallback time.Duration) time.Duration {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
		if ms, err := strconv.ParseInt(value, 10, 64); err == nil {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return fallback
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"uptime":  time.Since(a.started).Round(time.Second).String(),
		"version": "v2.0",
	})
}

func (a *App) handleDevTools(w http.ResponseWriter, r *http.Request) {
	errorJSON(w, http.StatusNotFound, errors.New("runtime does not expose devtools"))
}

func (a *App) handleLogs(w http.ResponseWriter, r *http.Request) {
	errorJSON(w, http.StatusNotFound, errors.New("runtime does not expose log streaming"))
}

func respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func errorJSON(w http.ResponseWriter, status int, err error) {
	respondJSON(w, status, map[string]any{"error": err.Error()})
}
