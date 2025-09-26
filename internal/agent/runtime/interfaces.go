package runtime

import (
	"context"

	"github.com/go-chi/chi/v5"
)

// Runtime defines the lifecycle and routing hooks for agent runtimes.
type Runtime interface {
	Name() string
	DevToolsInfo() (DevToolsInfo, bool)
	SubscribeLogs(buffer int) (<-chan LogEvent, func())
	MountRoutes(r chi.Router)
	Shutdown(ctx context.Context) error
}

// BrowserProvider exposes the underlying Browser implementation when available.
type BrowserProvider interface {
	BrowserInstance() *Browser
}
