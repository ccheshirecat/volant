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
	"syscall"
	"time"

	pty "github.com/creack/pty"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/mdlayher/vsock"

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
	shellEnabledEnvKey    = "volant_AGENT_ENABLE_SHELL"
	shellCommandEnvKey    = "volant_AGENT_SHELL"
	shellArgsEnvKey       = "volant_AGENT_SHELL_ARGS"
	shellTTYEnvKey        = "volant_AGENT_SHELL_TTY"
	defaultShellTTY       = "/dev/ttyS0"
)

type Config struct {
	ListenAddr          string
	RemoteDebuggingAddr string
	RemoteDebuggingPort int
	UserDataDir         string
	ExecPath            string
	DefaultTimeout      time.Duration
	EnableShell         bool
	ShellCommand        []string
	ShellTTY            string
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
	shellMu        sync.Mutex
	shellCancel    context.CancelFunc
	shellDone      chan struct{}
}

var errManifestFetch = errors.New("manifest fetch failed")

func Run(ctx context.Context) error {
	var bootLog *log.Logger
	consoleFile, consoleErr := os.OpenFile("/dev/console", os.O_WRONLY, 0)
	if consoleErr != nil {
		bootLog = log.New(os.Stdout, "volary-boot: ", log.LstdFlags|log.LUTC)
		bootLog.Printf("warning: could not open /dev/console: %v", consoleErr)
	} else {
		bootLog = log.New(consoleFile, "volary-boot: ", log.LstdFlags|log.LUTC)
	}

	cfg := loadConfig()
	logger := log.New(os.Stdout, "volary: ", log.LstdFlags|log.LUTC)

	app := &App{
		cfg:     cfg,
		timeout: cfg.DefaultTimeout,
		log:     bootLog,
		started: time.Now().UTC(),
		client:  &http.Client{Timeout: cfg.DefaultTimeout + 30*time.Second},
		ctx:     ctx,
	}

	defer app.stopShell()

	if err := app.bootstrapPID1(); err != nil {
		logger.Printf("fatal: pid1 bootstrap stage1 failed: %v", err)
		return err
	}

	if err := app.startShell(); err != nil {
		app.log.Printf("debug shell start failed: %v", err)
	}

	manifest, err := resolveManifest()
	if err != nil {
		if errors.Is(err, errManifestFetch) {
			logger.Printf("manifest fetch failed: %v", err)
		} else {
			if app.handleFatal(err, "resolve manifest") {
				return err
			}
		}
	}
	if manifest != nil {
		app.manifest = manifest
	} else {
		logger.Printf("no manifest received at startup; waiting for configuration")
	}

	if app.manifest != nil {
		if err := app.startWorkload(); app.handleFatal(err, "start workload") {
			return err
		}
	} else {
		app.log.Printf("manifest absent; workload deferred")
	}

	if err := app.run(ctx); app.handleFatal(err, "api server") {
		return err
	}

	if app.pid1() {
		app.log.Printf("fatal: run loop exited in pid1; halting")
		select {}
	}

	return nil
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

	handler := router

	// Start TCP listener (for bridged/dhcp modes)
	tcpServer := &http.Server{
		Addr:         a.cfg.ListenAddr,
		Handler:      handler,
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	errCh := make(chan error, 2)
	
	// TCP listener
	go func() {
		a.log.Printf("TCP listener starting on %s", a.cfg.ListenAddr)
		if err := tcpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("tcp listener: %w", err)
		}
	}()

	// Vsock listener (for vsock mode) - port 8080
	// This enables host->guest communication over vsock
	go func() {
		const vsockPort = 8080
		listener, err := vsock.Listen(vsockPort, nil)
		if err != nil {
			a.log.Printf("vsock listener failed to start on port %d: %v", vsockPort, err)
			// Don't fail if vsock isn't available (e.g., non-VM environment)
			return
		}
		defer listener.Close()
		
		a.log.Printf("vsock listener starting on port %d", vsockPort)
		
		vsockServer := &http.Server{
			Handler:      handler,
			ReadTimeout:  120 * time.Second,
			WriteTimeout: 120 * time.Second,
			IdleTimeout:  120 * time.Second,
		}
		
		if err := vsockServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("vsock listener: %w", err)
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := tcpServer.Shutdown(shutdownCtx); a.handleFatal(err, "shutdown tcp server") {
			return err
		}
		return ctx.Err()
	case err := <-errCh:
		if a.handleFatal(err, "serve http") {
			return err
		}
	}
	return nil
}

func (a *App) startShell() error {
	if !a.cfg.EnableShell {
		return nil
	}
	if len(a.cfg.ShellCommand) == 0 {
		return fmt.Errorf("shell command not configured")
	}

	a.shellMu.Lock()
	if a.shellCancel != nil {
		a.shellMu.Unlock()
		return nil
	}
	baseCtx := a.ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	shellCtx, cancel := context.WithCancel(baseCtx)
	done := make(chan struct{})
	a.shellCancel = cancel
	a.shellDone = done
	a.shellMu.Unlock()

	go a.shellLoop(shellCtx, done)
	return nil
}

func (a *App) stopShell() {
	a.shellMu.Lock()
	cancel := a.shellCancel
	done := a.shellDone
	a.shellCancel = nil
	a.shellDone = nil
	a.shellMu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			a.log.Printf("shell shutdown timed out")
		}
	}
}

func (a *App) shellLoop(ctx context.Context, done chan struct{}) {
	defer close(done)

	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := a.launchShell(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, io.EOF) {
				a.log.Printf("debug shell stopped: %v", err)
			} else {
				a.log.Printf("debug shell exited: %v", err)
			}
		} else {
			a.log.Printf("debug shell exited")
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		if err == nil {
			backoff = time.Second
		} else if backoff < 10*time.Second {
			backoff *= 2
			if backoff > 10*time.Second {
				backoff = 10 * time.Second
			}
		}
	}
}

func (a *App) launchShell(ctx context.Context) error {
	ttyPath := strings.TrimSpace(a.cfg.ShellTTY)
	if ttyPath == "" {
		ttyPath = defaultShellTTY
	}

	args := a.cfg.ShellCommand
	if len(args) == 0 {
		return errors.New("shell command not configured")
	}

	if err := a.launchShellDirect(ctx, ttyPath, args); err != nil {
		if !isSetcttyError(err) {
			return err
		}
		a.log.Printf("debug shell direct mode failed: %v; falling back to PTY bridge", err)
		return a.launchShellPTY(ctx, ttyPath, args)
	}

	return nil
}

func (a *App) launchShellDirect(ctx context.Context, ttyPath string, args []string) error {
	tty, err := os.OpenFile(ttyPath, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open shell tty %s: %w", ttyPath, err)
	}
	defer tty.Close()

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = "/"
	env := ensurePath(os.Environ(), []string{"/usr/local/sbin", "/usr/local/bin", "/usr/sbin", "/usr/bin", "/sbin", "/bin"})
	env = append(env, "TERM=linux")
	cmd.Env = env
	cmd.Stdout = tty
	cmd.Stderr = tty
	cmd.Stdin = tty
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
		Ctty:    int(tty.Fd()),
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start shell: %w", err)
	}
	a.log.Printf("debug shell started on %s (pid=%d)", ttyPath, cmd.Process.Pid)

	errCh := make(chan error, 1)
	go func() {
		errCh <- cmd.Wait()
	}()

	return a.waitShellProcess(ctx, cmd, errCh)
}

func (a *App) launchShellPTY(ctx context.Context, ttyPath string, args []string) error {
	serial, err := os.OpenFile(ttyPath, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open shell tty %s: %w", ttyPath, err)
	}
	defer serial.Close()

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = "/"
	env := ensurePath(os.Environ(), []string{"/usr/local/sbin", "/usr/local/bin", "/usr/sbin", "/usr/bin", "/sbin", "/bin"})
	env = append(env, "TERM=linux")
	cmd.Env = env

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("start shell pty: %w", err)
	}
	defer ptmx.Close()

	a.log.Printf("debug shell started on %s via PTY bridge (pid=%d)", ttyPath, cmd.Process.Pid)

	bridgeDone := make(chan shellPipeResult, 2)
	go pipeShell(serial, ptmx, "pty->tty", bridgeDone)
	go pipeShell(ptmx, serial, "tty->pty", bridgeDone)

	go func() {
		for i := 0; i < 2; i++ {
			res := <-bridgeDone
			if res.err != nil && !errors.Is(res.err, io.EOF) {
				a.log.Printf("debug shell bridge %s error: %v", res.dir, res.err)
			}
		}
	}()

	errCh := make(chan error, 1)
	go func() {
		errCh <- cmd.Wait()
	}()

	return a.waitShellProcess(ctx, cmd, errCh)
}

func (a *App) waitShellProcess(ctx context.Context, cmd *exec.Cmd, errCh <-chan error) error {
	select {
	case <-ctx.Done():
		pgid, pgErr := syscall.Getpgid(cmd.Process.Pid)
		if pgErr == nil {
			_ = syscall.Kill(-pgid, syscall.SIGTERM)
		} else {
			_ = cmd.Process.Signal(syscall.SIGTERM)
		}
		select {
		case err := <-errCh:
			return err
		case <-time.After(5 * time.Second):
			if pgErr == nil {
				_ = syscall.Kill(-pgid, syscall.SIGKILL)
			} else {
				_ = cmd.Process.Kill()
			}
			return <-errCh
		}
	case err := <-errCh:
		return err
	}
}

type shellPipeResult struct {
	dir string
	err error
}

func pipeShell(dst io.Writer, src io.Reader, dir string, ch chan<- shellPipeResult) {
	_, err := io.Copy(dst, src)
	ch <- shellPipeResult{dir: dir, err: err}
}

func isSetcttyError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.EINVAL) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "Setctty") || strings.Contains(msg, "Ctty not valid")
}
func loadConfig() Config {
	listen := envOrDefault(defaultListenEnvKey, defaultListenAddr)
	remoteAddr := os.Getenv(defaultRemoteAddrKey)
	remotePort := envIntOrDefault(defaultRemotePortKey, 0)
	userDataDir := os.Getenv(defaultUserDataDirKey)
	execPath := os.Getenv(defaultExecPathKey)

	defaultTimeout := parseDurationEnv(defaultTimeoutEnvKey, 0)

	enableShell := envBoolOrDefault(shellEnabledEnvKey, true)
	shellTTY := envOrDefault(shellTTYEnvKey, defaultShellTTY)
	shellPath := strings.TrimSpace(os.Getenv(shellCommandEnvKey))
	if shellPath == "" {
		shellPath = "/bin/sh"
	}
	shellArgsRaw := strings.TrimSpace(os.Getenv(shellArgsEnvKey))
	shellCommand := []string{shellPath}
	if shellArgsRaw != "" {
		shellCommand = append(shellCommand, strings.Fields(shellArgsRaw)...)
	}

	return Config{
		ListenAddr:          listen,
		RemoteDebuggingAddr: remoteAddr,
		RemoteDebuggingPort: remotePort,
		UserDataDir:         userDataDir,
		ExecPath:            execPath,
		DefaultTimeout:      defaultTimeout,
		EnableShell:         enableShell,
		ShellCommand:        shellCommand,
		ShellTTY:            shellTTY,
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

func (a *App) pid1() bool {
	return os.Getpid() == 1
}

func (a *App) handleFatal(err error, context string) bool {
	if err == nil {
		return false
	}
	if a.pid1() {
		a.log.Printf("fatal error in pid1 (%s): %v", context, err)
		a.log.Printf("pid1 cannot exit; halting")
		select {}
	}
	return true
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

func envBoolOrDefault(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
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
		path = strings.TrimSpace(path)

		target := resolveWorkloadURL(base, path, req.URL.RawQuery)

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

func resolveWorkloadURL(base *url.URL, path, rawQuery string) *url.URL {
	target := *base
	target.Path = joinURLPath(base.Path, path)
	target.RawPath = target.Path
	target.RawQuery = rawQuery
	return &target
}

func joinURLPath(basePath, subPath string) string {
	basePath = strings.TrimSpace(basePath)
	subPath = strings.TrimSpace(subPath)

	if basePath != "" && !strings.HasPrefix(basePath, "/") {
		basePath = "/" + basePath
	}
	basePath = strings.TrimRight(basePath, "/")
	subPath = strings.TrimLeft(subPath, "/")

	switch {
	case basePath == "" && subPath == "":
		return "/"
	case basePath == "":
		if subPath == "" {
			return "/"
		}
		return "/" + subPath
	case subPath == "":
		return basePath
	default:
		return basePath + "/" + subPath
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
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

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

	var (
		pgid    int
		pgidErr error
	)
	if cmd != nil && cmd.Process != nil {
		pgid, pgidErr = syscall.Getpgid(cmd.Process.Pid)
		if pgidErr == nil {
			if killErr := syscall.Kill(-pgid, syscall.SIGTERM); killErr != nil && !errors.Is(killErr, syscall.ESRCH) {
				a.log.Printf("workload process group kill error: %v", killErr)
			}
		} else if !errors.Is(pgidErr, syscall.ESRCH) {
			a.log.Printf("workload getpgid error: %v", pgidErr)
		}
	}

	if done != nil {
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			a.log.Printf("workload process shutdown timed out")
			if pgidErr == nil && pgid != 0 {
				if killErr := syscall.Kill(-pgid, syscall.SIGKILL); killErr != nil && !errors.Is(killErr, syscall.ESRCH) {
					a.log.Printf("workload process group kill error: %v", killErr)
				}
			}
			if cmd != nil && cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
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

	const minHealthTimeout = 2 * time.Minute
	timeout := time.Duration(hc.Timeout) * time.Millisecond
	if timeout <= 0 {
		timeout = minHealthTimeout
	} else if timeout < minHealthTimeout {
		timeout = minHealthTimeout
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
