//go:build !linux

package vsockproxy

import "context"

type noopManager struct{}

func newManager(Options) (Manager, error) {
	return nil, ErrUnsupported
}

func (noopManager) Upsert(context.Context, string, uint16, uint32, uint16) error { return nil }
func (noopManager) Remove(context.Context, string, uint16) error                 { return nil }
func (noopManager) Close() error                                                 { return nil }
