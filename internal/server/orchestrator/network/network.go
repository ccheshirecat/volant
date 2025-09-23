package network

import "context"

// Manager prepares host networking resources (tap devices, bridge attachments) for microVMs.
type Manager interface {
	PrepareTap(ctx context.Context, vmName, mac string) (string, error)
	CleanupTap(ctx context.Context, tapName string) error
}
