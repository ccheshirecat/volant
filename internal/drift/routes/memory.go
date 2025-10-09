package routes

import (
	"context"
	"fmt"
	"sync"
)

// MemoryStore provides an in-memory implementation of Store for early development.
type MemoryStore struct {
	mu    sync.RWMutex
	items map[string]Route
}

// NewMemoryStore constructs an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{items: make(map[string]Route)}
}

// List returns all stored routes.
func (m *MemoryStore) List(_ context.Context) ([]Route, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]Route, 0, len(m.items))
	for _, route := range m.items {
		result = append(result, route)
	}
	return result, nil
}

// Get returns a single route if present.
func (m *MemoryStore) Get(_ context.Context, hostPort uint16, protocol string) (*Route, error) {
	key := storageKey(hostPort, protocol)
	m.mu.RLock()
	defer m.mu.RUnlock()
	if route, ok := m.items[key]; ok {
		copy := route
		return &copy, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrNotFound, key)
}

// Upsert stores or replaces a route.
func (m *MemoryStore) Upsert(_ context.Context, route Route) error {
	key := storageKey(route.HostPort, route.Protocol)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[key] = route
	return nil
}

// Delete removes a route by host port and protocol.
func (m *MemoryStore) Delete(_ context.Context, hostPort uint16, protocol string) error {
	key := storageKey(hostPort, protocol)
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.items[key]; !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, key)
	}
	delete(m.items, key)
	return nil
}

func storageKey(port uint16, protocol string) string {
	return fmt.Sprintf("%d/%s", port, protocol)
}
