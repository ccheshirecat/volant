package runtime

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

const BrowserRuntimeName = "browser"

// browserRuntime adapts Browser to the generic Runtime interface.
type browserRuntime struct {
	real           *Browser
	defaultTimeout time.Duration
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

	return &browserRuntime{
		real:           browser,
		defaultTimeout: cfg.DefaultTimeout,
	}, nil
}

func (b *browserRuntime) Name() string { return BrowserRuntimeName }

func (b *browserRuntime) DevToolsInfo() (DevToolsInfo, bool) { return b.real.DevToolsInfo(), true }

func (b *browserRuntime) SubscribeLogs(buffer int) (<-chan LogEvent, func()) {
	return b.real.SubscribeLogs(buffer)
}

func (b *browserRuntime) BrowserInstance() *Browser { return b.real }

func (b *browserRuntime) MountRoutes(r chi.Router) {
	r.Route("/browser", b.mountBrowserRoutes)
	r.Route("/dom", b.mountDOMRoutes)
	r.Route("/script", b.mountScriptRoutes)
	r.Route("/actions", b.mountActionRoutes)
	r.Route("/profile", b.mountProfileRoutes)
}

func (b *browserRuntime) Shutdown(ctx context.Context) error {
	b.real.Close()
	return nil
}

func (b *browserRuntime) duration(ms int64) time.Duration {
	if ms <= 0 {
		if b.defaultTimeout > 0 {
			return b.defaultTimeout
		}
		return DefaultActionTimeout
	}
	return time.Duration(ms) * time.Millisecond
}

// Helper utilities ---------------------------------------------------------

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
