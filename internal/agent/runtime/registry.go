package runtime

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

// Options captures configuration passed to a runtime implementation.
type Options struct {
	DefaultTimeout time.Duration
	Config         map[string]string
}

// Runtime represents a modular workload runtime hosted inside the agent.
type Runtime interface {
	Name() string
	DevToolsInfo() DevToolsInfo
	SubscribeLogs(buffer int) (<-chan LogEvent, func())
	MountRoutes(r chi.Router)
	Shutdown(ctx context.Context) error
}

// Factory constructs a runtime instance.
type Factory func(ctx context.Context, opts Options) (Runtime, error)

var (
	registryMu sync.RWMutex
	runtimeRegistry = map[string]Factory{}
)

// Register associates a runtime factory with the provided name.
func Register(name string, factory Factory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if name == "" {
		panic("runtime: empty name")
	}
	if factory == nil {
		panic(fmt.Sprintf("runtime: nil factory for %s", name))
	}
	runtimeRegistry[name] = factory
}

// New instantiates a runtime by name.
func New(ctx context.Context, name string, opts Options) (Runtime, error) {
	registryMu.RLock()
	factory, ok := runtimeRegistry[name]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("runtime: unknown runtime %q", name)
	}
	if opts.Config == nil {
		opts.Config = make(map[string]string)
	}
	return factory(ctx, opts)
}
