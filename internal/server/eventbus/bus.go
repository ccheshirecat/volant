// Copyright (c) 2025 HYPR. PTE. LTD.
//
// Business Source License 1.1
// See LICENSE file in the project root for details.

package eventbus

import "context"

// Bus is a thin abstraction over the internal event distribution mechanism.
type Bus interface {
	Publish(ctx context.Context, topic string, payload any) error
	Subscribe(topic string, ch chan<- any) (unsubscribe func(), err error)
}
