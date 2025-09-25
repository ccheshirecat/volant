package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/go-chi/chi/v5"
)

const BrowserRuntimeName = "browser"

// browserRuntime adapts Browser to the generic Runtime interface.
type browserRuntime struct {
	real    *Browser
	logger *logEmitter
}

func newBrowserRuntime(ctx context.Context, opts Options) (Runtime, error) {
	cfg := BrowserConfig{}
	if addr := opts.Config["remote_debugging_addr"]; addr != "" {
		cfg.RemoteDebuggingAddr = addr
	}
	if port := opts.Config["remote_debugging_port"]; port != "" {
		if parsed, err := parseInt(port); err == nil {
			cfg.RemoteDebuggingPort = parsed
		}
	}
	cfg.UserDataDir = opts.Config["user_data_dir"]
	cfg.ExecPath = opts.Config["exec_path"]
	cfg.DefaultTimeout = opts.DefaultTimeout

	browser, err := NewBrowser(ctx, cfg)
	if err != nil {
		return nil, err
	}

	runtime := &browserRuntime{real: browser, logger: newLogEmitter()}
	browser.RegisterLogSubscriber(runtime.logger)
	return runtime, nil
}

func (b *browserRuntime) Name() string { return BrowserRuntimeName }

func (b *browserRuntime) DevToolsInfo() DevToolsInfo { return b.real.DevToolsInfo() }

func (b *browserRuntime) SubscribeLogs(buffer int) (<-chan LogEvent, func()) {
	return b.logger.Subscribe(buffer)
}

func (b *browserRuntime) MountRoutes(r chi.Router) {
	r.Post("/actions/navigate", b.wrapAction(func(timeout time.Duration, payload map[string]any) (any, error) {
		url, _ := payload["url"].(string)
		return nil, b.real.Navigate(timeout, url)
	}))
}

func (b *browserRuntime) Shutdown(ctx context.Context) error {
	b.logger.Close()
	return b.real.Close()
}

func (b *browserRuntime) wrapAction(handler func(timeout time.Duration, payload map[string]any) (any, error)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		// TODO: implement request decoding + timeout handling
	}
}

func parseInt(value string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("browser runtime: invalid int %q: %w", value, err)
	}
	return parsed, nil
}

func init() {
	Register(BrowserRuntimeName, newBrowserRuntime)
}
