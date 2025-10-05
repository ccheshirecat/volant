// Copyright (c) 2025 HYPR. PTE. LTD.
//
// Business Source License 1.1
// See LICENSE file in the project root for details.
//go:build !linux

package network

// NewBridgeManager returns a no-op manager on non-Linux hosts so that
// non-Linux builds can compile without Linux-specific netlink symbols.
func NewBridgeManager(bridge string) Manager { // bridge kept for API symmetry
    _ = bridge
    return NewNoop()
}
