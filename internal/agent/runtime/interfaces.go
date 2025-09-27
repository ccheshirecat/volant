package runtime

import (
	"context"
	"net/http"

	"github.com/ccheshirecat/volant/internal/pluginspec"
)

// Runtime defines the lifecycle and routing hooks for agent runtimes.
type Runtime interface {
	Name() string
	MountRoutes(router Router) error
	Shutdown(ctx context.Context) error
}

// ManifestAware runtimes can receive the manifest contents when mounting routes.
type ManifestAware interface {
	Runtime
	MountRoutesWithManifest(router Router, manifest pluginspec.Manifest) error
}

// Router abstracts an HTTP router for route registration.
type Router interface {
	Route(prefix string, fn func(Router))
	Handle(method, path string, handler http.Handler)
}
