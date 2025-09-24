package network

import (
	"context"
	"fmt"
	"regexp"
)

var nonAlnum = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// NoopManager returns deterministic tap names without touching system networking.
type NoopManager struct{}

// NewNoop creates a manager suitable for development environments where tap creation is handled manually.
func NewNoop() *NoopManager { return &NoopManager{} }

// PrepareTap returns a sanitized tap name but performs no host configuration.
func (n *NoopManager) PrepareTap(ctx context.Context, vmName, mac string) (string, error) {
	_ = ctx
	sanitized := nonAlnum.ReplaceAllString(vmName, "")
	if sanitized == "" {
		sanitized = "vm"
	}
	return fmt.Sprintf("hype-tap-%s", sanitized), nil
}

// CleanupTap is a no-op for the development manager.
func (n *NoopManager) CleanupTap(ctx context.Context, tapName string) error {
	_ = ctx
	_ = tapName
	return nil
}
