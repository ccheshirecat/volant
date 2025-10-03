// Copyright (c) 2025 HYPR. PTE. LTD.
//
// Business Source License 1.1
// See LICENSE file in the project root for details.

package network

import "context"

// Manager prepares host networking resources (tap devices, bridge attachments) for microVMs.
type Manager interface {
	PrepareTap(ctx context.Context, vmName, mac string) (string, error)
	CleanupTap(ctx context.Context, tapName string) error
}
