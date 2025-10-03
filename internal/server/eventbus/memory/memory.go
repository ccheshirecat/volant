// Copyright (c) 2025 HYPR. PTE. LTD.
//
// Business Source License 1.1
// See LICENSE file in the project root for details.

package memory

import (
	"context"
	"errors"
	"sync"

	"github.com/volantvm/volant/internal/server/eventbus"
)

// Bus is an in-memory event bus suitable for single-node development testing.
type Bus struct {
	mu     sync.RWMutex
	topics map[string][]chan<- any
}

var _ eventbus.Bus = (*Bus)(nil)

// New creates a new Bus instance.
func New() *Bus {
	return &Bus{topics: make(map[string][]chan<- any)}
}

// Publish fan-outs payloads to subscribers.
func (b *Bus) Publish(ctx context.Context, topic string, payload any) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.topics[topic] {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ch <- payload:
		default:
		}
	}
	return nil
}

// Subscribe registers a channel for a topic.
func (b *Bus) Subscribe(topic string, ch chan<- any) (func(), error) {
	if ch == nil {
		return nil, errors.New("eventbus: channel must not be nil")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.topics[topic] = append(b.topics[topic], ch)
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		subs := b.topics[topic]
		for i := range subs {
			if subs[i] == ch {
				b.topics[topic] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		if len(b.topics[topic]) == 0 {
			delete(b.topics, topic)
		}
	}, nil
}
