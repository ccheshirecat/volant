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
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ccheshirecat/volant/internal/pluginspec"
)

const (
	defaultListenAddr     = ":8080"
	defaultTimeoutEnvKey  = "volant_AGENT_DEFAULT_TIMEOUT"
	defaultListenEnvKey   = "volant_AGENT_LISTEN_ADDR"
	defaultRemoteAddrKey  = "volant_AGENT_REMOTE_DEBUGGING_ADDR"
	defaultRemotePortKey  = "volant_AGENT_REMOTE_DEBUGGING_PORT"
	defaultUserDataDirKey = "volant_AGENT_USER_DATA_DIR"
	defaultExecPathKey    = "volant_AGENT_EXEC_PATH"
)

type Config struct {
	ListenAddr          string
	RemoteDebuggingAddr string
	RemoteDebuggingPort int
	UserDataDir         string
	ExecPath            string
	DefaultTimeout      time.Duration
}

type App struct {
	cfg            Config
	server         *http.Server
	timeout        time.Duration
	log            *log.Logger
	started        time.Time
	manifest       *pluginspec.Manifest
	client         *http.Client
	ctx            context.Context
	mu             sync.Mutex
	workloadCmd    *exec.Cmd
	workloadDone   chan error
	workloadCancel context.CancelFunc
	workloadSpec   string
}

var errManifestFetch = errors.New("manifest fetch failed")

func Run(ctx context.Context) error {
	cfg := loadConfig()
	logger := log.New(os.Stdout, "volary: ", log.LstdFlags|log.LUTC)

	manifest, err := resolveManifest()
	if err != nil {
		if errors.Is(err, errManifestFetch) {
			logger.Printf("manifest fetch failed: %v", err)
		} else {
			return err
		}
	}
	if manifest == nil {
		logger.Printf("no manifest received at startup; waiting for configuration")
	}

	app := &App{
		cfg:      cfg,
		timeout:  cfg.DefaultTimeout,
		log:      logger,
		started:  time.Now().UTC(),
		manifest: manifest,
		client:   &http.Client{Timeout: cfg.DefaultTimeout + 30*time.Second},
		ctx:      ctx,
	}

	if err := app.bootstrapPID1(); err != nil {
		logger.Printf("pid1 bootstrap failed: %v", err)
	}

	if err := app.startWorkload(); err != nil {
		logger.Printf("workload start failed: %v", err)
	}

	return app.run(ctx)
}

func (a *App) run(ctx context.Context) error {
	defer a.stopWorkload()
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)
	router.Use(middleware.Timeout(a.timeout + 30*time.Second))

	router.Get("/healthz", a.handleHealth)

	router.Route("/v1", func(r chi.Router) {
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
	remoteAddr := os.Getenv(defaultRemoteAddrKey)
	remotePort := envIntOrDefault(defaultRemotePortKey, 0)
	userDataDir := os.Getenv(defaultUserDataDirKey)
	execPath := os.Getenv(defaultExecPathKey)

	defaultTimeout := parseDurationEnv(defaultTimeoutEnvKey, 0)

	return Config{
		ListenAddr:          listen,
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
	var (
		pluginName       string
		apiHost, apiPort string
		manifestEncoded  string
	)
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
		case pluginspec.CmdlineKey:
			manifestEncoded = parts[1]
		}
	}
	if strings.TrimSpace(manifestEncoded) != "" {
		manifest, err := pluginspec.Decode(manifestEncoded)
		if err != nil {
			return nil, fmt.Errorf("decode manifest: %w", err)
		}
		manifest.Normalize()
		return &manifest, nil
	}
	if apiHost == "" || apiPort == "" || pluginName == "" {
		return nil, nil
	}

	manifestURL := fmt.Sprintf("http://%s:%s/api/v1/plugins/%s/manifest", apiHost, apiPort, pluginName)
	resp, err := http.Get(manifestURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errManifestFetch, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: unexpected status %d", errManifestFetch, resp.StatusCode)
	}

	var manifest pluginspec.Manifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("%w: %v", errManifestFetch, err)
	}
	manifest.Normalize()
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

	a.mountWorkloadPassthrough(router, parsedBase)
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

func ensurePath(env []string, additions []string) []string {
	const pathKey = "PATH"
	pathValue := ""
	pathIndex := -1
	for i, kv := range env {
		if strings.HasPrefix(kv, pathKey+"=") {
			pathValue = kv[len(pathKey)+1:]
			pathIndex = i
			break
		}
	}

	existing := make(map[string]struct{})
	parts := []string{}
	if pathValue != "" {
		for _, segment := range strings.Split(pathValue, ":") {
			segment = strings.TrimSpace(segment)
			if segment == "" {
				continue
			}
			if _, ok := existing[segment]; ok {
				continue
			}
			existing[segment] = struct{}{}
			parts = append(parts, segment)
		}
	}
	for _, add := range additions {
		add = strings.TrimSpace(add)
		if add == "" {
			continue
		}
		if _, ok := existing[add]; ok {
			continue
		}
		existing[add] = struct{}{}
		parts = append(parts, add)
	}
	if len(parts) == 0 {
		return env
	}
	joined := strings.Join(parts, ":")
	if pathIndex >= 0 {
		env[pathIndex] = pathKey + "=" + joined
	} else {
		env = append(env, pathKey+"="+joined)
	}
	return env
}

func (a *App) mountWorkloadPassthrough(router chi.Router, base *url.URL) {
	handler := a.forwardWorkload(base, a.timeout, "/v1")
	methods := []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodHead,
		http.MethodOptions,
	}
	for _, method := range methods {
		router.MethodFunc(method, "/*", handler)
		router.MethodFunc(method, "/", handler)
	}
}

func (a *App) forwardWorkload(base *url.URL, timeout time.Duration, stripPrefix string) http.HandlerFunc {
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

		path := req.URL.Path
		if stripPrefix != "" {
			path = strings.TrimPrefix(path, stripPrefix)
		}
		if path == "" {
			path = "/"
		}
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}

		rel := &url.URL{Path: path, RawQuery: req.URL.RawQuery}
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
			a.log.Printf("workload proxy error: %v", err)
		}
	}
}

func (a *App) startWorkload() error {
	a.mu.Lock()
	if a.manifest == nil {
		a.mu.Unlock()
		return nil
	}
	manifest := *a.manifest
	ctx := a.ctx
	existingCmd := a.workloadCmd
	existingSpec := a.workloadSpec
	existingCancel := a.workloadCancel
	existingDone := a.workloadDone
	a.mu.Unlock()

	if strings.TrimSpace(strings.ToLower(manifest.Workload.Type)) != "http" {
		return nil
	}
	baseURL := strings.TrimSpace(manifest.Workload.BaseURL)
	if baseURL == "" {
		return fmt.Errorf("manifest workload base_url required for http workload")
	}
	if len(manifest.Workload.Entrypoint) == 0 || strings.TrimSpace(manifest.Workload.Entrypoint[0]) == "" {
		return fmt.Errorf("manifest workload entrypoint required for http workload")
	}

	parsedBase, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("manifest workload base_url invalid: %w", err)
	}

	spec := workloadSignature(manifest.Workload)
	if existingCmd != nil && spec == existingSpec {
		return nil
	}

	if existingCancel != nil {
		existingCancel()
	}
	if existingDone != nil {
		select {
		case <-existingDone:
		case <-time.After(10 * time.Second):
			a.log.Printf("workload process shutdown timed out")
		}
	}

	if ctx == nil {
		ctx = context.Background()
	}
	procCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(procCtx, manifest.Workload.Entrypoint[0], manifest.Workload.Entrypoint[1:]...)

	env := os.Environ()
	env = ensurePath(env, []string{"/usr/local/bin", "/usr/bin", "/bin"})
	for key, value := range manifest.Workload.Env {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}
	cmd.Env = env
	if dir := strings.TrimSpace(manifest.Workload.WorkDir); dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		cancel()
		return err
	}

	done := make(chan error, 1)

	a.mu.Lock()
	a.workloadCmd = cmd
	a.workloadDone = done
	a.workloadCancel = cancel
	a.workloadSpec = spec
	a.mu.Unlock()

	go func() {
		err := cmd.Wait()
		done <- err
		close(done)
		if err != nil {
			a.log.Printf("workload process exited with error: %v", err)
		} else {
			a.log.Printf("workload process exited")
		}
		a.mu.Lock()
		if a.workloadCmd == cmd {
			a.workloadCmd = nil
			a.workloadCancel = nil
			a.workloadDone = nil
			a.workloadSpec = ""
		}
		a.mu.Unlock()
	}()

	if err := a.waitForHealth(procCtx, parsedBase, manifest.HealthCheck); err != nil {
		a.log.Printf("workload health check failed: %v", err)
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			a.log.Printf("workload process shutdown timed out after health failure")
		}
		a.mu.Lock()
		if a.workloadCmd == cmd {
			a.workloadCmd = nil
			a.workloadCancel = nil
			a.workloadDone = nil
			a.workloadSpec = ""
		}
		a.mu.Unlock()
		return fmt.Errorf("workload health check failed: %w", err)
	}

	a.log.Printf("workload process started (pid=%d)", cmd.Process.Pid)
	return nil
}

func (a *App) stopWorkload() {
	a.mu.Lock()
	cancel := a.workloadCancel
	done := a.workloadDone
	cmd := a.workloadCmd
	a.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	if done != nil {
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			a.log.Printf("workload process shutdown timed out")
		}
	}

	a.mu.Lock()
	if a.workloadCmd == cmd {
		a.workloadCmd = nil
		a.workloadCancel = nil
		a.workloadDone = nil
		a.workloadSpec = ""
	}
	a.mu.Unlock()
}

func (a *App) waitForHealth(parent context.Context, base *url.URL, hc pluginspec.HealthCheck) error {
	endpoint := strings.TrimSpace(hc.Endpoint)
	if endpoint == "" {
		return nil
	}

	timeout := time.Duration(hc.Timeout) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	var ticker = time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	client := &http.Client{Timeout: 5 * time.Second}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}

		rel := &url.URL{Path: endpoint}
		target := base.ResolveReference(rel)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			a.log.Printf("workload health probe error: %v", err)
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		a.log.Printf("workload health probe status %d", resp.StatusCode)
	}
}

func workloadSignature(w pluginspec.Workload) string {
	parts := make([]string, 0, len(w.Entrypoint)+len(w.Env)+1)
	parts = append(parts, strings.Join(w.Entrypoint, "||"))
	if len(w.Env) > 0 {
		keys := make([]string, 0, len(w.Env))
		for key := range w.Env {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			parts = append(parts, fmt.Sprintf("%s=%s", key, w.Env[key]))
		}
	}
	return strings.Join(parts, "##")
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
	manifest.Normalize()
	a.manifest = &manifest
	a.log.Printf("manifest fetched for plugin %s", plugin)
	if err := a.startWorkload(); err != nil {
		a.log.Printf("workload start failed after manifest fetch: %v", err)
	}
	return nil
}
