package vsockproxy

import (
	"context"
	"log/slog"
)

// Options configure the vsock proxy manager.
type Options struct {
	BindAddress string
	Logger      *slog.Logger
}

// Manager manages host TCP listeners that forward to vsock endpoints.
type Manager interface {
	Upsert(ctx context.Context, proto string, hostPort uint16, cid uint32, guestPort uint16) error
	Remove(ctx context.Context, proto string, hostPort uint16) error
	Close() error
}

// ErrUnsupported indicates vsock forwarding isn't available on this platform.
var ErrUnsupported = errorUnsupported{}

type errorUnsupported struct{}

func (errorUnsupported) Error() string { return "vsock proxy: unsupported platform" }

// New constructs a platform-appropriate manager.
func New(opts Options) (Manager, error) {
	return newManager(opts)
}
