package dataplane

import (
	"context"
	"log/slog"
	"net"
)

// Options configures dataplane manager construction.
type Options struct {
	ObjectPath string
	Interface  string
	Logger     *slog.Logger
}

// Interface describes bridge dataplane interactions.
type Interface interface {
	ApplyBridge(ctx context.Context, proto uint8, hostPort uint16, destIP net.IP, destPort uint16) error
	Remove(ctx context.Context, proto uint8, hostPort uint16) error
	Close() error
}

// ErrUnsupported indicates the dataplane is not available on the current platform.
var ErrUnsupported = errorUnsupported{}

type errorUnsupported struct{}

func (errorUnsupported) Error() string { return "dataplane: unsupported platform" }

// New constructs a platform-appropriate dataplane manager.
func New(opts Options) (Interface, error) {
	return newManager(opts)
}
