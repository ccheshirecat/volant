package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	agentruntime "github.com/ccheshirecat/volant/internal/agent/runtime"
	"github.com/ccheshirecat/volant/internal/pluginspec"
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
	cfg      Config
	runtime  agentruntime.Runtime
	server   *http.Server
	timeout  time.Duration
	log      *log.Logger
	started  time.Time
	manifest *pluginspec.Manifest
	client   *http.Client
}

func Run(ctx context.Context) error {
	cfg := loadConfig()
	logger := log.New(os.Stdout, "volary: ", log.LstdFlags|log.LUTC)

	manifest, err := resolveManifest()
	if err != nil {
		return err
	}
	if manifest == nil {
		logger.Printf("no manifest received at startup; waiting for configuration")
	}

	runtimeName := strings.TrimSpace(cfg.Runtime)
	if runtimeName == "" && manifest != nil {
		runtimeName = strings.TrimSpace(manifest.Runtime)
	}
	cfg.Runtime = runtimeName

	app := &App{
		cfg:      cfg,
		timeout:  cfg.DefaultTimeout,
		log:      logger,
		started:  time.Now().UTC(),
		manifest: manifest,
		client:   &http.Client{Timeout: cfg.DefaultTimeout + 30*time.Second},
	}

	if err := app.ensureRuntime(ctx); err != nil {
		if runtimeName != "" {
			logger.Printf("failed to start runtime %q: %v", runtimeName, err)
		}
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
		if err := a.mountRuntimeRoutes(r); err != nil {
			a.log.Printf("runtime route mount error: %v", err)
		}
		if err := a.mountManifestRoutes(r); err != nil {
			a.log.Printf("manifest route mount error: %v", err)
		}
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

func resolveManifest() (*pluginspec.Manifest, error) {
	if encoded := strings.TrimSpace(os.Getenv("VOLANT_MANIFEST")); encoded != "" {
		manifest, err := pluginspec.Decode(encoded)
		if err != nil {
			return nil, err
		}
		return &manifest, nil
	}

	data, err := os.ReadFile("/proc/cmdline")
	if err != nil {
		return nil, err
	}
	fields := strings.Fields(string(data))
	var pluginName string
	var apiHost, apiPort string
	for _, field := range fields {
		parts := strings.SplitN(field, "=", 2)
		if len(parts) != 2 {
			continue
		}
		switch parts[0] {
		case pluginspec.APIHostKey:
			apiHost = parts[1]
		case pluginspec.APIPortKey:
			apiPort = parts[1]
		case pluginspec.PluginKey:
			pluginName = parts[1]
		}
	}
	if apiHost == "" || apiPort == "" || pluginName == "" {
		return nil, nil
	}

	manifestURL := fmt.Sprintf("http://%s:%s/api/v1/plugins/%s/manifest", apiHost, apiPort, pluginName)
	resp, err := http.Get(manifestURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch manifest: unexpected status %d", resp.StatusCode)
	}

	var manifest pluginspec.Manifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
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

func envValue(key string) string {
	return strings.TrimSpace(os.Getenv(key))
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

func (a *App) mountRuntimeRoutes(r chi.Router) error {
	if a.runtime == nil {
		a.log.Printf("runtime not yet configured; skipping runtime routes")
		return nil
	}
	adapter := runtimeRouter{Router: r}
	if aware, ok := a.runtime.(agentruntime.ManifestAware); ok && a.manifest != nil {
		return aware.MountRoutesWithManifest(adapter, *a.manifest)
	}
	return a.runtime.MountRoutes(adapter)
}

func (a *App) mountManifestRoutes(router chi.Router) error {
	if a.manifest == nil {
		return nil
	}
	if strings.TrimSpace(strings.ToLower(a.manifest.Workload.Type)) != "http" {
		return nil
	}
	baseURL := strings.TrimSpace(a.manifest.Workload.BaseURL)
	if baseURL == "" {
		return nil
	}
	parsedBase, err := url.Parse(baseURL)
	if err != nil {
		return err
	}

	for actionName, action := range a.manifest.Actions {
		method := strings.ToUpper(strings.TrimSpace(action.Method))
		if method == "" {
			method = http.MethodPost
		}
		path := strings.TrimSpace(action.Path)
		if path == "" {
			continue
		}
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}

		var routeTimeout time.Duration
		if action.TimeoutMs > 0 {
			routeTimeout = time.Duration(action.TimeoutMs) * time.Millisecond
		} else {
			routeTimeout = a.timeout
		}

		handler := a.forwardManifestAction(parsedBase, path, routeTimeout, actionName)
		router.MethodFunc(method, path, handler)
	}
	return nil
}

func (a *App) forwardManifestAction(base *url.URL, actionPath string, timeout time.Duration, actionName string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		if timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}

		body, err := io.ReadAll(req.Body)
		if err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		_ = req.Body.Close()

		rel := &url.URL{Path: actionPath, RawQuery: req.URL.RawQuery}
		target := base.ResolveReference(rel)

		proxyReq, err := http.NewRequestWithContext(ctx, req.Method, target.String(), bytes.NewReader(body))
		if err != nil {
			errorJSON(w, http.StatusBadGateway, err)
			return
		}
		copyHeaders(proxyReq.Header, req.Header)

		resp, err := a.client.Do(proxyReq)
		if err != nil {
			errorJSON(w, http.StatusBadGateway, err)
			return
		}
		defer resp.Body.Close()

		copyHeaders(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		if _, err := io.Copy(w, resp.Body); err != nil {
			a.log.Printf("manifest action %s proxy error: %v", actionName, err)
		}
	}
}

func copyHeaders(dst, src http.Header) {
	dst.Del("Host")
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

type runtimeRouter struct {
	chi.Router
}

func (rr runtimeRouter) Route(prefix string, fn func(agentruntime.Router)) {
	rr.Router.Route(prefix, func(r chi.Router) {
		fn(runtimeRouter{Router: r})
	})
}

func (rr runtimeRouter) Handle(method, path string, handler http.Handler) {
	rr.Router.Method(method, path, handler)
}

func respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func errorJSON(w http.ResponseWriter, status int, err error) {
	respondJSON(w, status, map[string]any{"error": err.Error()})
}

func (a *App) refreshManifest() error {
	if a.manifest != nil {
		return nil
	}

	host := envValue("volant.api_host")
	port := envValue("volant.api_port")
	plugin := envValue("volant.plugin")
	if host == "" || port == "" || plugin == "" {
		return nil
	}

	url := fmt.Sprintf("http://%s:%s/api/v1/plugins/%s/manifest", host, port, plugin)
	resp, err := a.client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("manifest fetch status %d", resp.StatusCode)
	}

	var manifest pluginspec.Manifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return err
	}
	a.manifest = &manifest
	a.log.Printf("manifest fetched for plugin %s", plugin)
	if err := a.ensureRuntime(context.Background()); err != nil {
		a.log.Printf("runtime start failed after manifest fetch: %v", err)
	}
	return nil
}

func (a *App) ensureRuntime(ctx context.Context) error {
	if a.manifest == nil {
		return fmt.Errorf("runtime name required")
	}

	runtimeName := strings.TrimSpace(a.cfg.Runtime)
	if runtimeName == "" {
		runtimeName = strings.TrimSpace(a.manifest.Runtime)
		if runtimeName != "" {
			a.cfg.Runtime = runtimeName
		}
	}
	if runtimeName == "" {
		return fmt.Errorf("runtime name required")
	}

	opts := agentruntime.Options{
		DefaultTimeout: a.cfg.DefaultTimeout,
		Config: map[string]string{
			"remote_debugging_addr": a.cfg.RemoteDebuggingAddr,
			"remote_debugging_port": strconv.Itoa(a.cfg.RemoteDebuggingPort),
			"user_data_dir":         a.cfg.UserDataDir,
			"exec_path":             a.cfg.ExecPath,
		},
		Manifest: a.manifest,
	}

	runtimeInstance, err := agentruntime.New(ctx, runtimeName, opts)
	if err != nil {
		return err
	}
	if a.runtime != nil {
		_ = a.runtime.Shutdown(context.Background())
	}
	a.runtime = runtimeInstance
	a.log.Printf("runtime %s started", runtimeName)
	return nil
}
