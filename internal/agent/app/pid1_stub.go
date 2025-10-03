// Copyright (c) 2025 HYPR. PTE. LTD.
//
// Business Source License 1.1
// See LICENSE file in the project root for details.

//go:build !linux

package app

// bootstrapPID1 is a no-op on non-Linux platforms to allow local builds
// on macOS/Windows. The real implementation lives in pid1.go with linux tag.
func (a *App) bootstrapPID1() error { return nil }
