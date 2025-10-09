//go:build !linux

package dataplane

import (
	"context"
	"net"
)

func newManager(Options) (Interface, error) {
	return nil, ErrUnsupported
}

type noopManager struct{}

func (noopManager) ApplyBridge(context.Context, uint8, uint16, net.IP, uint16) error { return nil }
func (noopManager) Remove(context.Context, uint8, uint16) error                      { return nil }
func (noopManager) Close() error                                                     { return nil }
