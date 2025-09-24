package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	agentruntime "github.com/viperhq/viper/internal/agent/runtime"
)

const (
	defaultListenAddr     = ":8080"
	defaultTimeoutEnvKey  = "VIPER_AGENT_DEFAULT_TIMEOUT"
	defaultListenEnvKey   = "VIPER_AGENT_LISTEN_ADDR"
	defaultRemoteAddrKey  = "VIPER_AGENT_REMOTE_DEBUGGING_ADDR"
	defaultRemotePortKey  = "VIPER_AGENT_REMOTE_DEBUGGING_PORT"
	defaultUserDataDirKey = "VIPER_AGENT_USER_DATA_DIR"
	defaultExecPathKey    = "VIPER_AGENT_EXEC_PATH"
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
	cfg     Config
	browser *agentruntime.Browser
	server  *http.Server
	timeout time.Duration
	log     *log.Logger
	started time.Time
}

func Run(ctx context.Context) error {
	cfg := loadConfig()
	logger := log.New(os.Stdout, "viper-agent: ", log.LstdFlags|log.LUTC)

	browser, err := agentruntime.NewBrowser(ctx, agentruntime.BrowserConfig{
		RemoteDebuggingAddr: cfg.RemoteDebuggingAddr,
		RemoteDebuggingPort: cfg.RemoteDebuggingPort,
		UserDataDir:         cfg.UserDataDir,
		ExecPath:            cfg.ExecPath,
		DefaultTimeout:      cfg.DefaultTimeout,
	})
	if err != nil {
		return err
	}
	defer browser.Close()

	app := &App{
		cfg:     cfg,
		browser: browser,
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

		r.Post("/browser/navigate", a.handleNavigate)
		r.Post("/browser/reload", a.handleReload)
		r.Post("/browser/back", a.handleBack)
		r.Post("/browser/forward", a.handleForward)
		r.Post("/browser/viewport", a.handleViewport)
		r.Post("/browser/user-agent", a.handleUserAgent)
		r.Post("/browser/wait-navigation", a.handleWaitNavigation)
		r.Post("/browser/screenshot", a.handleScreenshot)

		r.Post("/dom/click", a.handleClick)
		r.Post("/dom/type", a.handleType)
		r.Post("/dom/get-text", a.handleGetText)
		r.Post("/dom/get-html", a.handleGetHTML)
		r.Post("/dom/get-attribute", a.handleGetAttribute)
		r.Post("/dom/wait-selector", a.handleWaitSelector)

		r.Post("/script/evaluate", a.handleEvaluate)

		r.Post("/profile/attach", a.handleProfileAttach)
		r.Get("/profile/extract", a.handleProfileExtract)
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
		a.log.Printf("listening on %s (devtools ws: %s)", a.cfg.ListenAddr, a.browser.DevToolsInfo().WebSocketURL)
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
	remoteAddr := envOrDefault(defaultRemoteAddrKey, agentruntime.DefaultRemoteAddr)
	remotePort := envIntOrDefault(defaultRemotePortKey, agentruntime.DefaultRemotePort)
	userDataDir := os.Getenv(defaultUserDataDirKey)
	execPath := os.Getenv(defaultExecPathKey)

	defaultTimeout := parseDurationEnv(defaultTimeoutEnvKey, agentruntime.DefaultActionTimeout)

	return Config{
		ListenAddr:          listen,
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

type okResponse struct {
	Status string `json:"status"`
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"uptime":  time.Since(a.started).Round(time.Second).String(),
		"version": "v2.0",
	})
}

func (a *App) handleDevTools(w http.ResponseWriter, r *http.Request) {
	info := a.browser.DevToolsInfo()
	respondJSON(w, http.StatusOK, info)
}

func (a *App) handleLogs(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		respondError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	ch, unsubscribe := a.browser.SubscribeLogs(128)
	defer unsubscribe()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(event)
			if err != nil {
				a.log.Printf("log stream marshal error: %v", err)
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

type navigateRequest struct {
	URL       string `json:"url"`
	TimeoutMs int64  `json:"timeout_ms"`
}

func (a *App) handleNavigate(w http.ResponseWriter, r *http.Request) {
	var req navigateRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.URL) == "" {
		respondError(w, http.StatusBadRequest, "url is required")
		return
	}
	if err := a.browser.Navigate(a.duration(req.TimeoutMs), req.URL); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, okResponse{Status: "ok"})
}

type reloadRequest struct {
	IgnoreCache bool  `json:"ignore_cache"`
	TimeoutMs   int64 `json:"timeout_ms"`
}

func (a *App) handleReload(w http.ResponseWriter, r *http.Request) {
	var req reloadRequest
	_ = decodeJSON(r, &req)
	if err := a.browser.Reload(a.duration(req.TimeoutMs), req.IgnoreCache); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, okResponse{Status: "ok"})
}

func (a *App) handleBack(w http.ResponseWriter, r *http.Request) {
	timeout := parseTimeout(r)
	if err := a.browser.Back(timeout); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, okResponse{Status: "ok"})
}

func (a *App) handleForward(w http.ResponseWriter, r *http.Request) {
	timeout := parseTimeout(r)
	if err := a.browser.Forward(timeout); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, okResponse{Status: "ok"})
}

type viewportRequest struct {
	Width     int     `json:"width"`
	Height    int     `json:"height"`
	Scale     float64 `json:"scale"`
	Mobile    bool    `json:"mobile"`
	TimeoutMs int64   `json:"timeout_ms"`
}

func (a *App) handleViewport(w http.ResponseWriter, r *http.Request) {
	var req viewportRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.browser.SetViewport(a.duration(req.TimeoutMs), req.Width, req.Height, req.Scale, req.Mobile); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, okResponse{Status: "ok"})
}

type userAgentRequest struct {
	UserAgent      string `json:"user_agent"`
	AcceptLanguage string `json:"accept_language"`
	Platform       string `json:"platform"`
	TimeoutMs      int64  `json:"timeout_ms"`
}

func (a *App) handleUserAgent(w http.ResponseWriter, r *http.Request) {
	var req userAgentRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.browser.SetUserAgent(a.duration(req.TimeoutMs), req.UserAgent, req.AcceptLanguage, req.Platform); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, okResponse{Status: "ok"})
}

type clickRequest struct {
	Selector  string `json:"selector"`
	Button    string `json:"button"`
	TimeoutMs int64  `json:"timeout_ms"`
}

func (a *App) handleClick(w http.ResponseWriter, r *http.Request) {
	var req clickRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.browser.Click(a.duration(req.TimeoutMs), req.Selector, req.Button); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, okResponse{Status: "ok"})
}

type typeRequest struct {
	Selector  string `json:"selector"`
	Value     string `json:"value"`
	Clear     bool   `json:"clear"`
	TimeoutMs int64  `json:"timeout_ms"`
}

func (a *App) handleType(w http.ResponseWriter, r *http.Request) {
	var req typeRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.browser.Type(a.duration(req.TimeoutMs), req.Selector, req.Value, req.Clear); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, okResponse{Status: "ok"})
}

type textRequest struct {
	Selector  string `json:"selector"`
	Visible   bool   `json:"visible"`
	TimeoutMs int64  `json:"timeout_ms"`
}

func (a *App) handleGetText(w http.ResponseWriter, r *http.Request) {
	var req textRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	text, err := a.browser.GetText(a.duration(req.TimeoutMs), req.Selector, req.Visible)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"text": text})
}

type htmlRequest struct {
	Selector  string `json:"selector"`
	TimeoutMs int64  `json:"timeout_ms"`
}

func (a *App) handleGetHTML(w http.ResponseWriter, r *http.Request) {
	var req htmlRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	html, err := a.browser.GetHTML(a.duration(req.TimeoutMs), req.Selector)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"html": html})
}

type attributeRequest struct {
	Selector  string `json:"selector"`
	Name      string `json:"name"`
	TimeoutMs int64  `json:"timeout_ms"`
}

func (a *App) handleGetAttribute(w http.ResponseWriter, r *http.Request) {
	var req attributeRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	value, ok, err := a.browser.GetAttribute(a.duration(req.TimeoutMs), req.Selector, req.Name)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"value":  value,
		"exists": ok,
	})
}

type waitSelectorRequest struct {
	Selector  string `json:"selector"`
	Visible   bool   `json:"visible"`
	TimeoutMs int64  `json:"timeout_ms"`
}

func (a *App) handleWaitSelector(w http.ResponseWriter, r *http.Request) {
	var req waitSelectorRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.browser.WaitForSelector(a.duration(req.TimeoutMs), req.Selector, req.Visible); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, okResponse{Status: "ok"})
}

type waitNavigationRequest struct {
	TimeoutMs int64 `json:"timeout_ms"`
}

func (a *App) handleWaitNavigation(w http.ResponseWriter, r *http.Request) {
	var req waitNavigationRequest
	_ = decodeJSON(r, &req)
	if err := a.browser.WaitForNavigation(a.duration(req.TimeoutMs)); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, okResponse{Status: "ok"})
}

type evaluateRequest struct {
	Expression   string `json:"expression"`
	AwaitPromise bool   `json:"await_promise"`
	TimeoutMs    int64  `json:"timeout_ms"`
}

func (a *App) handleEvaluate(w http.ResponseWriter, r *http.Request) {
	var req evaluateRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := a.browser.Evaluate(a.duration(req.TimeoutMs), req.Expression, req.AwaitPromise)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"result": result})
}

type screenshotRequest struct {
	FullPage  bool   `json:"full_page"`
	Format    string `json:"format"`
	Quality   int    `json:"quality"`
	TimeoutMs int64  `json:"timeout_ms"`
}

func (a *App) handleScreenshot(w http.ResponseWriter, r *http.Request) {
	var req screenshotRequest
	_ = decodeJSON(r, &req)

	data, err := a.browser.Screenshot(a.duration(req.TimeoutMs), req.FullPage, req.Format, req.Quality)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := map[string]any{
		"data":        base64.StdEncoding.EncodeToString(data),
		"format":      strings.ToLower(req.Format),
		"full_page":   req.FullPage,
		"byte_length": len(data),
		"captured_at": time.Now().UTC().Format(time.RFC3339Nano),
	}
	respondJSON(w, http.StatusOK, response)
}

type cookieParamRequest struct {
	Name     string   `json:"name"`
	Value    string   `json:"value"`
	Domain   string   `json:"domain"`
	Path     string   `json:"path"`
	Expires  *float64 `json:"expires"`
	HTTPOnly bool     `json:"http_only"`
	Secure   bool     `json:"secure"`
	SameSite string   `json:"same_site"`
}

type profileAttachRequest struct {
	Cookies []cookieParamRequest `json:"cookies"`
	Local   map[string]string    `json:"local_storage"`
	Session map[string]string    `json:"session_storage"`
	Timeout int64                `json:"timeout_ms"`
}

func (a *App) handleProfileAttach(w http.ResponseWriter, r *http.Request) {
	var req profileAttachRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var cookies []*network.CookieParam
	for _, c := range req.Cookies {
		cookie, err := convertCookieParam(c)
		if err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		cookies = append(cookies, cookie)
	}

	timeout := a.duration(req.Timeout)
	if len(cookies) > 0 {
		if err := a.browser.SetCookies(timeout, cookies); err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	payload := agentruntime.StoragePayload{
		Local:   req.Local,
		Session: req.Session,
	}
	if err := a.browser.SetStorage(timeout, payload); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, okResponse{Status: "ok"})
}

func (a *App) handleProfileExtract(w http.ResponseWriter, r *http.Request) {
	timeout := parseTimeout(r)

	cookies, err := a.browser.GetCookies(timeout)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	storage, err := a.browser.GetStorage(timeout)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := map[string]any{
		"cookies":         convertCookiesResponse(cookies),
		"local_storage":   storage.Local,
		"session_storage": storage.Session,
	}
	respondJSON(w, http.StatusOK, response)
}

func convertCookieParam(req cookieParamRequest) (*network.CookieParam, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, errors.New("cookie name required")
	}
	cookie := &network.CookieParam{
		Name:  req.Name,
		Value: req.Value,
	}
	if req.Domain != "" {
		cookie.Domain = req.Domain
	}
	if req.Path != "" {
		cookie.Path = req.Path
	}
	cookie.HTTPOnly = req.HTTPOnly
	cookie.Secure = req.Secure

	switch strings.ToLower(req.SameSite) {
	case "", "lax":
		cookie.SameSite = network.CookieSameSiteLax
	case "none":
		cookie.SameSite = network.CookieSameSiteNone
	case "strict":
		cookie.SameSite = network.CookieSameSiteStrict
	default:
		return nil, fmt.Errorf("invalid same_site value %q", req.SameSite)
	}

	if req.Expires != nil {
		expires := time.Unix(int64(*req.Expires), 0).UTC()
		cdpEpoch := cdp.TimeSinceEpoch(expires)
		cookie.Expires = &cdpEpoch
	}

	return cookie, nil
}

func convertCookiesResponse(cookies []*network.Cookie) []map[string]any {
	result := make([]map[string]any, 0, len(cookies))
	for _, c := range cookies {
		item := map[string]any{
			"name":      c.Name,
			"value":     c.Value,
			"domain":    c.Domain,
			"path":      c.Path,
			"http_only": c.HTTPOnly,
			"secure":    c.Secure,
			"same_site": strings.ToLower(string(c.SameSite)),
			"expires":   c.Expires,
		}
		result = append(result, item)
	}
	return result
}

func parseTimeout(r *http.Request) time.Duration {
	if value := r.URL.Query().Get("timeout_ms"); value != "" {
		if ms, err := strconv.ParseInt(value, 10, 64); err == nil && ms > 0 {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return 0
}

func (a *App) duration(ms int64) time.Duration {
	if ms <= 0 {
		return a.timeout
	}
	return time.Duration(ms) * time.Millisecond
}

func decodeJSON(r *http.Request, dest any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dest); err != nil {
		return err
	}
	return nil
}

func respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload != nil {
		_ = json.NewEncoder(w).Encode(payload)
	}
}

func respondError(w http.ResponseWriter, status int, message string) {
	if status < 400 {
		status = http.StatusInternalServerError
	}
	respondJSON(w, status, map[string]any{
		"error": message,
	})
}
