package runtime

import (
	"context"
	"net/http"
	"time"

	"github.com/ccheshirecat/volant/internal/pluginspec"
)

type Options struct {
	DefaultTimeout time.Duration
	Config         map[string]string
	Manifest       *pluginspec.Manifest
}

type Runtime interface {
	Name() string
	MountRoutes(router Router) error
	Shutdown(ctx context.Context) error
}

type ManifestAware interface {
	Runtime
	MountRoutesWithManifest(router Router, manifest pluginspec.Manifest) error
}

type Router interface {
	Route(prefix string, fn func(Router))
	Handle(method, path string, handler http.Handler)
}
