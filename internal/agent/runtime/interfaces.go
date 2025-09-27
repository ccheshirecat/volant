package runtime

import (
	"context"
	"net/http"
)

// Runtime defines the lifecycle and routing hooks for agent runtimes.
type Runtime interface {
	Name() string
	MountRoutes(router Router) error
	Shutdown(ctx context.Context) error
}

// Router abstracts an HTTP router for route registration.
type Router interface {
	Route(prefix string, fn func(Router))
	Handle(method, path string, handler http.Handler)
}
