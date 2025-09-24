package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	cpruntime "github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/cdproto/storage"
	"github.com/chromedp/chromedp"
)

const (
	DefaultRemoteAddr         = "0.0.0.0"
	DefaultRemotePort         = 9222
	defaultUserDataDirName    = "viper-agent-data"
	DefaultActionTimeout      = 60 * time.Second
	devtoolsProbeRetryBackoff = 250 * time.Millisecond
	devtoolsProbeAttempts     = 20
)

var defaultExecCandidates = []string{
	"/headless-chrome/headless-chrome",
	"/headless-shell/headless-shell",
	"/usr/bin/headless-shell",
	"/usr/bin/chromium",
	"/usr/bin/google-chrome",
}

func resolveExecPath(requested string) (string, error) {
	candidates := make([]string, 0, len(defaultExecCandidates)+1)
	if path := strings.TrimSpace(requested); path != "" {
		candidates = append(candidates, path)
	}
	candidates = append(candidates, defaultExecCandidates...)

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("browser: could not find a headless Chrome binary; tried %s", strings.Join(candidates, ", "))
}

// BrowserConfig controls how the chromedp managed browser instance is launched.
type BrowserConfig struct {
	RemoteDebuggingAddr string
	RemoteDebuggingPort int
	UserDataDir         string
	ExecPath            string
	DefaultTimeout      time.Duration
}

// StoragePayload captures localStorage/sessionStorage key/value pairs.
type StoragePayload struct {
	Local   map[string]string `json:"local"`
	Session map[string]string `json:"session"`
}

// DevToolsInfo exposes debugging metadata for CDP proxying.
type DevToolsInfo struct {
	WebSocketURL   string `json:"websocket_url"`
	WebSocketPath  string `json:"websocket_path"`
	BrowserVersion string `json:"browser_version"`
	UserAgent      string `json:"user_agent"`
	Address        string `json:"address"`
	Port           int    `json:"port"`
}

type devToolsInternal struct {
	WebSocketURL   string
	WebSocketPath  string
	BrowserVersion string
	UserAgent      string
}

// Browser orchestrates a chromedp-controlled headless browser instance and
// provides higher-level automation helpers.
type Browser struct {
	cfg         BrowserConfig
	allocCtx    context.Context
	allocCancel context.CancelFunc
	ctx         context.Context
	cancel      context.CancelFunc

	mu       sync.Mutex
	log      *logEmitter
	devtools devToolsInternal
}

// NewBrowser launches a headless Chrome instance reachable through chromedp.
func NewBrowser(ctx context.Context, cfg BrowserConfig) (*Browser, error) {
	if ctx == nil {
		return nil, errors.New("browser: context is required")
	}

	cfg.RemoteDebuggingAddr = strings.TrimSpace(cfg.RemoteDebuggingAddr)
	if cfg.RemoteDebuggingAddr == "" {
		cfg.RemoteDebuggingAddr = DefaultRemoteAddr
	}
	if cfg.RemoteDebuggingPort == 0 {
		cfg.RemoteDebuggingPort = DefaultRemotePort
	}

	if cfg.UserDataDir == "" {
		cfg.UserDataDir = filepath.Join(os.TempDir(), defaultUserDataDirName)
	}
	if err := os.MkdirAll(cfg.UserDataDir, 0o755); err != nil {
		return nil, fmt.Errorf("browser: ensure user data dir: %w", err)
	}

	resolvedExecPath, err := resolveExecPath(cfg.ExecPath)
	if err != nil {
		return nil, err
	}
	cfg.ExecPath = resolvedExecPath

	if cfg.DefaultTimeout <= 0 {
		cfg.DefaultTimeout = DefaultActionTimeout
	}

	logEmitter := newLogEmitter()

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("headless", true),
		chromedp.Flag("hide-scrollbars", true),
		chromedp.Flag("mute-audio", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("remote-debugging-address", cfg.RemoteDebuggingAddr),
		chromedp.Flag("remote-debugging-port", cfg.RemoteDebuggingPort),
		chromedp.UserDataDir(cfg.UserDataDir),
	)
	if cfg.ExecPath != "" {
		opts = append(opts, chromedp.ExecPath(cfg.ExecPath))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)

	logFunc := func(format string, args ...interface{}) {
		logEmitter.Publish(LogEvent{
			Stream:    "chromedp",
			Line:      fmt.Sprintf(format, args...),
			Timestamp: time.Now().UTC(),
		})
	}

	browserCtx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(logFunc))

	// Prime the browser process.
	if err := chromedp.Run(browserCtx); err != nil {
		cancel()
		allocCancel()
		return nil, fmt.Errorf("browser: start: %w", err)
	}

	// Enable network domain for cookie/profile operations.
	if err := chromedp.Run(browserCtx, network.Enable()); err != nil {
		cancel()
		allocCancel()
		return nil, fmt.Errorf("browser: enable network: %w", err)
	}

	devtools, err := probeDevTools(cfg.RemoteDebuggingPort)
	if err != nil {
		cancel()
		allocCancel()
		return nil, err
	}

	b := &Browser{
		cfg:         cfg,
		allocCtx:    allocCtx,
		allocCancel: allocCancel,
		ctx:         browserCtx,
		cancel:      cancel,
		log:         logEmitter,
		devtools:    devtools,
	}

	b.publish("agent", fmt.Sprintf("headless browser ready (devtools port %d)", cfg.RemoteDebuggingPort))
	return b, nil
}

// Close tears down the browser process and resources.
func (b *Browser) Close() {
	b.cancel()
	b.allocCancel()
	b.log.Close()
}

// DevToolsInfo returns debugging metadata.
func (b *Browser) DevToolsInfo() DevToolsInfo {
	return DevToolsInfo{
		WebSocketURL:   b.devtools.WebSocketURL,
		WebSocketPath:  b.devtools.WebSocketPath,
		BrowserVersion: b.devtools.BrowserVersion,
		UserAgent:      b.devtools.UserAgent,
		Address:        b.cfg.RemoteDebuggingAddr,
		Port:           b.cfg.RemoteDebuggingPort,
	}
}

// SubscribeLogs registers a subscriber for agent log events.
func (b *Browser) SubscribeLogs(buffer int) (<-chan LogEvent, func()) {
	return b.log.Subscribe(buffer)
}

// Navigate opens the requested URL.
func (b *Browser) Navigate(timeout time.Duration, url string) error {
	return b.run(timeout, "navigate", fmt.Sprintf("Navigating to %s", url), func(ctx context.Context) (string, error) {
		if err := chromedp.Run(ctx, chromedp.Navigate(url)); err != nil {
			return "", err
		}
		return "navigation committed", nil
	})
}

// Reload refreshes the current page.
func (b *Browser) Reload(timeout time.Duration, ignoreCache bool) error {
	return b.run(timeout, "reload", "Reloading current page", func(ctx context.Context) (string, error) {
		action := page.Reload().WithIgnoreCache(ignoreCache)
		if err := action.Do(ctx); err != nil {
			return "", err
		}
		return "reload completed", nil
	})
}

// Back navigates back in history.
func (b *Browser) Back(timeout time.Duration) error {
	return b.run(timeout, "history_back", "Navigating back", func(ctx context.Context) (string, error) {
		if err := chromedp.Run(ctx, chromedp.NavigateBack()); err != nil {
			return "", err
		}
		return "back navigation completed", nil
	})
}

// Forward navigates forward in history.
func (b *Browser) Forward(timeout time.Duration) error {
	return b.run(timeout, "history_forward", "Navigating forward", func(ctx context.Context) (string, error) {
		if err := chromedp.Run(ctx, chromedp.NavigateForward()); err != nil {
			return "", err
		}
		return "forward navigation completed", nil
	})
}

// SetViewport updates viewport dimensions and scale.
func (b *Browser) SetViewport(timeout time.Duration, width, height int, scale float64, mobile bool) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("viewport dimensions must be greater than zero")
	}
	if scale <= 0 {
		scale = 1
	}
	return b.run(timeout, "set_viewport", fmt.Sprintf("Setting viewport to %dx%d (scale %.2f)", width, height, scale), func(ctx context.Context) (string, error) {
		params := emulation.SetDeviceMetricsOverride(int64(width), int64(height), scale, mobile)
		if err := params.Do(ctx); err != nil {
			return "", err
		}
		return "viewport updated", nil
	})
}

// SetUserAgent overrides the browser user agent metadata.
func (b *Browser) SetUserAgent(timeout time.Duration, ua, acceptLanguage, platform string) error {
	if strings.TrimSpace(ua) == "" {
		return errors.New("user agent must not be empty")
	}
	return b.run(timeout, "set_user_agent", fmt.Sprintf("Setting user agent: %s", ua), func(ctx context.Context) (string, error) {
		params := emulation.SetUserAgentOverride(ua)
		if acceptLanguage != "" {
			params = params.WithAcceptLanguage(acceptLanguage)
		}
		if platform != "" {
			params = params.WithPlatform(platform)
		}
		if err := params.Do(ctx); err != nil {
			return "", err
		}
		return "user agent updated", nil
	})
}

// WaitForNavigation blocks until the current navigation finishes loading.
func (b *Browser) WaitForNavigation(timeout time.Duration) error {
	return b.run(timeout, "wait_for_navigation", "Waiting for navigation to complete", func(ctx context.Context) (string, error) {
		listener := make(chan struct{}, 1)
		lctx, cancel := context.WithCancel(ctx)
		defer cancel()

		chromedp.ListenTarget(lctx, func(ev any) {
			switch evt := ev.(type) {
			case *page.EventLifecycleEvent:
				if evt.Name == "DOMContentLoaded" || evt.Name == "load" {
					select {
					case listener <- struct{}{}:
					default:
					}
				}
			case *page.EventLoadEventFired, *page.EventDomContentEventFired, *page.EventNavigatedWithinDocument, *page.EventFrameNavigated:
				select {
				case listener <- struct{}{}:
				default:
				}
			}
		})

		timer := time.NewTimer(b.timeout(timeout))
		defer timer.Stop()

		select {
		case <-listener:
			return "navigation finished", nil
		case <-timer.C:
			return "", errors.New("navigation timed out")
		case <-ctx.Done():
			return "", ctx.Err()
		}
	})
}

// Evaluate executes arbitrary JavaScript within the current document context.
func (b *Browser) Evaluate(timeout time.Duration, expression string, awaitPromise bool) (interface{}, error) {
	if strings.TrimSpace(expression) == "" {
		return nil, errors.New("expression required")
	}
	var result interface{}
	err := b.run(timeout, "evaluate", truncateForLog(fmt.Sprintf("Evaluating script: %s", expression), 120), func(ctx context.Context) (string, error) {
		eval := cpruntime.Evaluate(expression).WithReturnByValue(true)
		if awaitPromise {
			eval = eval.WithAwaitPromise(true)
		}
		remote, _, err := eval.Do(ctx)
		if err != nil {
			return "", err
		}
		value, decodeErr := decodeRemoteObject(remote)
		if decodeErr != nil {
			return "", decodeErr
		}
		result = value
		return "script evaluation complete", nil
	})
	return result, err
}

// Screenshot captures a screenshot and returns the raw bytes.
func (b *Browser) Screenshot(timeout time.Duration, fullPage bool, format string, quality int) ([]byte, error) {
	if quality <= 0 || quality > 100 {
		quality = 90
	}
	if format == "" {
		format = "png"
	}
	format = strings.ToLower(format)

	var data []byte
	err := b.run(timeout, "screenshot", fmt.Sprintf("Capturing screenshot (full=%t, format=%s)", fullPage, format), func(ctx context.Context) (string, error) {
		var captureErr error
		switch {
		case fullPage && format == "png":
			captureErr = chromedp.Run(ctx, chromedp.FullScreenshot(&data, quality))
		default:
			captureErr = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
				params := page.CaptureScreenshot()
				switch format {
				case "png":
					params = params.WithFormat(page.CaptureScreenshotFormatPng)
				case "jpeg", "jpg":
					params = params.WithFormat(page.CaptureScreenshotFormatJpeg).WithQuality(int64(quality))
				default:
					return fmt.Errorf("unsupported screenshot format %q", format)
				}

				if fullPage {
					_, _, _, _, _, cssContent, err := page.GetLayoutMetrics().Do(ctx)
					if err != nil {
						return err
					}
					if cssContent == nil {
						return errors.New("browser: css content metrics unavailable")
					}
					width := cssContent.Width
					height := cssContent.Height
					params = params.WithClip(&page.Viewport{
						X:      0,
						Y:      0,
						Width:  width,
						Height: height,
						Scale:  1,
					})
				}

				buf, err := params.Do(ctx)
				if err != nil {
					return err
				}
				data = buf
				return nil
			}))
		}
		if captureErr != nil {
			return "", captureErr
		}
		return fmt.Sprintf("screenshot captured (%d bytes)", len(data)), nil
	})
	return data, err
}

// SetCookies sets browser cookies.
func (b *Browser) SetCookies(timeout time.Duration, cookies []*network.CookieParam) error {
	return b.run(timeout, "set_cookies", fmt.Sprintf("Setting %d cookie(s)", len(cookies)), func(ctx context.Context) (string, error) {
		if len(cookies) == 0 {
			if err := network.ClearBrowserCookies().Do(ctx); err != nil {
				return "", err
			}
			return "cleared cookies", nil
		}
		if err := network.SetCookies(cookies).Do(ctx); err != nil {
			return "", err
		}
		return "cookies applied", nil
	})
}

// GetCookies retrieves all browser cookies.
func (b *Browser) GetCookies(timeout time.Duration) ([]*network.Cookie, error) {
	var cookies []*network.Cookie
	err := b.run(timeout, "get_cookies", "Retrieving cookies", func(ctx context.Context) (string, error) {
		response, err := storage.GetCookies().Do(ctx)
		if err != nil {
			return "", err
		}
		cookies = response
		return fmt.Sprintf("retrieved %d cookie(s)", len(cookies)), nil
	})
	return cookies, err
}

// SetStorage populates localStorage/sessionStorage.
func (b *Browser) SetStorage(timeout time.Duration, payload StoragePayload) error {
	return b.run(timeout, "set_storage", "Applying storage values", func(ctx context.Context) (string, error) {
		tasks := chromedp.Tasks{}
		for k, v := range payload.Local {
			expr := fmt.Sprintf("window.localStorage.setItem(%s, %s);", jsString(k), jsString(v))
			tasks = append(tasks, evaluateExpression(expr, false))
		}
		for k, v := range payload.Session {
			expr := fmt.Sprintf("window.sessionStorage.setItem(%s, %s);", jsString(k), jsString(v))
			tasks = append(tasks, evaluateExpression(expr, false))
		}
		if len(tasks) == 0 {
			return "storage unchanged", nil
		}
		if err := chromedp.Run(ctx, tasks); err != nil {
			return "", err
		}
		return "storage applied", nil
	})
}

// GetStorage extracts localStorage/sessionStorage content.
func (b *Browser) GetStorage(timeout time.Duration) (StoragePayload, error) {
	result := StoragePayload{
		Local:   map[string]string{},
		Session: map[string]string{},
	}
	err := b.run(timeout, "get_storage", "Extracting storage", func(ctx context.Context) (string, error) {
		var localJSON, sessionJSON string
		tasks := chromedp.Tasks{
			chromedp.Evaluate(`JSON.stringify(Object.fromEntries(Object.keys(localStorage).map(k => [k, localStorage.getItem(k)])))`, &localJSON),
			chromedp.Evaluate(`JSON.stringify(Object.fromEntries(Object.keys(sessionStorage).map(k => [k, sessionStorage.getItem(k)])))`, &sessionJSON),
		}
		if err := chromedp.Run(ctx, tasks); err != nil {
			return "", err
		}
		if localJSON != "" {
			if err := json.Unmarshal([]byte(localJSON), &result.Local); err != nil {
				return "", err
			}
		}
		if sessionJSON != "" {
			if err := json.Unmarshal([]byte(sessionJSON), &result.Session); err != nil {
				return "", err
			}
		}
		return fmt.Sprintf("storage extracted (local=%d, session=%d)", len(result.Local), len(result.Session)), nil
	})
	return result, err
}

// Click dispatches a mouse click event to a specific element.
func (b *Browser) Click(timeout time.Duration, selector, button string) error {
	if selector == "" {
		return errors.New("selector required")
	}
	btn := strings.ToLower(button)
	switch btn {
	case "", "left":
		btn = "left"
	case "middle", "right":
	default:
		return fmt.Errorf("unsupported button %q", button)
	}

	return b.run(timeout, "click", fmt.Sprintf("Clicking %s (%s button)", selector, btn), func(ctx context.Context) (string, error) {
		actions := chromedp.Tasks{chromedp.WaitVisible(selector, chromedp.ByQuery)}
		if btn == "left" {
			actions = append(actions, chromedp.Click(selector, chromedp.ByQuery))
		} else {
			var nodes []*cdp.Node
			actions = append(actions,
				chromedp.Nodes(selector, &nodes, chromedp.ByQuery, chromedp.NodeVisible),
				chromedp.ActionFunc(func(ctx context.Context) error {
					if len(nodes) == 0 {
						return fmt.Errorf("selector %s did not resolve to a node", selector)
					}

					var opts []chromedp.MouseOption
					switch btn {
					case "middle":
						opts = append(opts, chromedp.ButtonType(input.Middle))
					case "right":
						opts = append(opts, chromedp.ButtonType(input.Right))
					}

					return chromedp.MouseClickNode(nodes[0], opts...).Do(ctx)
				}),
			)
		}

		if err := chromedp.Run(ctx, actions); err != nil {
			return "", err
		}
		return "click dispatched", nil
	})
}

// Type writes text into a specific element.
func (b *Browser) Type(timeout time.Duration, selector, value string, clear bool) error {
	if selector == "" {
		return errors.New("selector required")
	}
	return b.run(timeout, "type", fmt.Sprintf("Typing into %s", selector), func(ctx context.Context) (string, error) {
		tasks := chromedp.Tasks{
			chromedp.WaitVisible(selector, chromedp.ByQuery),
			chromedp.Focus(selector, chromedp.ByQuery),
		}
		if clear {
			tasks = append(tasks, chromedp.Clear(selector, chromedp.ByQuery))
		}
		tasks = append(tasks, chromedp.SendKeys(selector, value, chromedp.ByQuery))
		if err := chromedp.Run(ctx, tasks); err != nil {
			return "", err
		}
		return "text entry complete", nil
	})
}

// GetText retrieves the text content of a specific element.
func (b *Browser) GetText(timeout time.Duration, selector string, visible bool) (string, error) {
	if selector == "" {
		return "", errors.New("selector required")
	}
	var text string
	err := b.run(timeout, "get_text", fmt.Sprintf("Reading text from %s", selector), func(ctx context.Context) (string, error) {
		action := chromedp.Text(selector, &text, chromedp.ByQuery)
		if visible {
			action = chromedp.Text(selector, &text, chromedp.ByQuery, chromedp.NodeVisible)
		}
		if err := chromedp.Run(ctx, action); err != nil {
			return "", err
		}
		return "text captured", nil
	})
	return text, err
}

// GetHTML retrieves the HTML content of a specific element.
func (b *Browser) GetHTML(timeout time.Duration, selector string) (string, error) {
	if selector == "" {
		return "", errors.New("selector required")
	}
	var html string
	err := b.run(timeout, "get_html", fmt.Sprintf("Retrieving HTML from %s", selector), func(ctx context.Context) (string, error) {
		if err := chromedp.Run(ctx, chromedp.InnerHTML(selector, &html, chromedp.ByQuery)); err != nil {
			return "", err
		}
		return "html captured", nil
	})
	return html, err
}

// GetAttribute retrieves the value of a specific attribute from an element.
func (b *Browser) GetAttribute(timeout time.Duration, selector, name string) (string, bool, error) {
	if selector == "" || name == "" {
		return "", false, errors.New("selector and attribute name required")
	}
	var value string
	var ok bool
	err := b.run(timeout, "get_attribute", fmt.Sprintf("Reading attribute %s from %s", name, selector), func(ctx context.Context) (string, error) {
		if err := chromedp.Run(ctx, chromedp.AttributeValue(selector, name, &value, &ok, chromedp.ByQuery)); err != nil {
			return "", err
		}
		if ok {
			return "attribute captured", nil
		}
		return "attribute not present", nil
	})
	return value, ok, err
}

// WaitForSelector waits for a specific element to become visible or ready.
func (b *Browser) WaitForSelector(timeout time.Duration, selector string, visible bool) error {
	if selector == "" {
		return errors.New("selector required")
	}
	actionLabel := "ready"
	if visible {
		actionLabel = "visible"
	}
	return b.run(timeout, "wait_for_selector", fmt.Sprintf("Waiting for %s to become %s", selector, actionLabel), func(ctx context.Context) (string, error) {
		var err error
		if visible {
			err = chromedp.Run(ctx, chromedp.WaitVisible(selector, chromedp.ByQuery))
		} else {
			err = chromedp.Run(ctx, chromedp.WaitReady(selector, chromedp.ByQuery))
		}
		if err != nil {
			return "", err
		}
		return "selector satisfied", nil
	})
}

// timeout normalises supplied durations against the configured default.
func (b *Browser) timeout(d time.Duration) time.Duration {
	if d <= 0 {
		return b.cfg.DefaultTimeout
	}
	return d
}

func (b *Browser) run(timeout time.Duration, name, startLine string, fn func(ctx context.Context) (string, error)) error {
	timeout = b.timeout(timeout)
	if strings.TrimSpace(startLine) != "" {
		b.publish("agent", startLine)
	}
	started := time.Now()

	ctx, cancel := context.WithTimeout(b.ctx, timeout)
	defer cancel()

	b.mu.Lock()
	defer b.mu.Unlock()

	successMsg, err := fn(ctx)
	if err != nil {
		b.publish("agent", fmt.Sprintf("%s failed: %v", name, err))
		return err
	}
	duration := time.Since(started).Round(time.Millisecond)
	if successMsg == "" {
		successMsg = fmt.Sprintf("%s completed in %s", name, duration)
	} else {
		successMsg = fmt.Sprintf("%s (in %s)", successMsg, duration)
	}
	b.publish("agent", successMsg)
	return nil
}

func (b *Browser) publish(stream, line string) {
	if strings.TrimSpace(line) == "" {
		return
	}
	b.log.Publish(LogEvent{
		Stream:    stream,
		Line:      line,
		Timestamp: time.Now().UTC(),
	})
}

func probeDevTools(port int) (devToolsInternal, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	urlStr := fmt.Sprintf("http://127.0.0.1:%d/json/version", port)

	type response struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
		Browser              string `json:"Browser"`
		UserAgent            string `json:"User-Agent"`
	}

	for i := 0; i < devtoolsProbeAttempts; i++ {
		resp, err := client.Get(urlStr)
		if err != nil {
			time.Sleep(devtoolsProbeRetryBackoff)
			continue
		}
		func() {
			defer resp.Body.Close()
		}()

		if resp.StatusCode != http.StatusOK {
			time.Sleep(devtoolsProbeRetryBackoff)
			continue
		}

		var payload response
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			time.Sleep(devtoolsProbeRetryBackoff)
			continue
		}
		if payload.WebSocketDebuggerURL == "" {
			time.Sleep(devtoolsProbeRetryBackoff)
			continue
		}

		parsed, err := url.Parse(payload.WebSocketDebuggerURL)
		if err != nil {
			return devToolsInternal{}, fmt.Errorf("browser: parse devtools url: %w", err)
		}

		return devToolsInternal{
			WebSocketURL:   payload.WebSocketDebuggerURL,
			WebSocketPath:  parsed.RequestURI(),
			BrowserVersion: payload.Browser,
			UserAgent:      payload.UserAgent,
		}, nil
	}
	return devToolsInternal{}, fmt.Errorf("browser: unable to discover devtools endpoint on port %d", port)
}

func resolveExecPath(requested string) (string, error) {
	candidates := make([]string, 0, len(defaultExecCandidates)+1)
	if path := strings.TrimSpace(requested); path != "" {
		candidates = append(candidates, path)
	}
	candidates = append(candidates, defaultExecCandidates...)

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("browser: could not find a headless Chrome binary; tried %s", strings.Join(candidates, ", "))
}

func jsString(value string) string {
	return strconv.Quote(value)
}

func evaluateExpression(expr string, await bool) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		e := cpruntime.Evaluate(expr)
		if await {
			e = e.WithAwaitPromise(true)
		}
		_, _, err := e.Do(ctx)
		return err
	})
}

func decodeRemoteObject(obj *cpruntime.RemoteObject) (interface{}, error) {
	if obj == nil {
		return nil, nil
	}
	switch obj.Type {
	case cpruntime.TypeUndefined:
		return nil, nil
	case cpruntime.TypeString, cpruntime.TypeBoolean, cpruntime.TypeNumber, cpruntime.TypeBigint:
		return obj.Value, nil
	case cpruntime.TypeObject:
		if obj.Subtype == cpruntime.SubtypeNull {
			return nil, nil
		}
		if obj.Value != nil {
			return obj.Value, nil
		}
		return obj.Description, nil
	default:
		if obj.Value != nil {
			return obj.Value, nil
		}
		return obj.Description, nil
	}
}

func truncateForLog(input string, limit int) string {
	if len(input) <= limit {
		return input
	}
	if limit < 3 {
		return input[:limit]
	}
	return input[:limit-3] + "..."
}
