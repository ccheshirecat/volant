package routes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FileStore persists routes to a JSON file on disk.
type FileStore struct {
	path  string
	mu    sync.RWMutex
	items map[string]Route
}

// NewFileStore loads existing routes from path or creates a new empty store.
func NewFileStore(path string) (*FileStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("routes: file path required")
	}
	store := &FileStore{
		path:  path,
		items: make(map[string]Route),
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (f *FileStore) load() error {
	data, err := os.ReadFile(f.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("routes: read file: %w", err)
	}

	var routes []Route
	if len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, &routes); err != nil {
		return fmt.Errorf("routes: decode file: %w", err)
	}

	for _, route := range routes {
		key := storageKey(route.HostPort, route.Protocol)
		f.items[key] = route
	}
	return nil
}

// List returns all persisted routes.
func (f *FileStore) List(context.Context) ([]Route, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	result := make([]Route, 0, len(f.items))
	for _, route := range f.items {
		result = append(result, route)
	}
	return result, nil
}

// Get fetches a route by host port and protocol.
func (f *FileStore) Get(_ context.Context, hostPort uint16, protocol string) (*Route, error) {
	key := storageKey(hostPort, protocol)
	f.mu.RLock()
	defer f.mu.RUnlock()
	if route, ok := f.items[key]; ok {
		copy := route
		return &copy, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrNotFound, key)
}

// Upsert writes or replaces a route on disk.
func (f *FileStore) Upsert(_ context.Context, route Route) error {
	key := storageKey(route.HostPort, route.Protocol)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items[key] = route
	return f.persistLocked()
}

// Delete removes a route from disk.
func (f *FileStore) Delete(_ context.Context, hostPort uint16, protocol string) error {
	key := storageKey(hostPort, protocol)
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.items[key]; !ok {
		return fmt.Errorf("%w: %s", ErrNotFound, key)
	}
	delete(f.items, key)
	return f.persistLocked()
}

func (f *FileStore) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
		return fmt.Errorf("routes: ensure directory: %w", err)
	}

	entries := make([]Route, 0, len(f.items))
	for _, route := range f.items {
		entries = append(entries, route)
	}

	tmp, err := os.CreateTemp(filepath.Dir(f.path), "routes-*.json")
	if err != nil {
		return fmt.Errorf("routes: create temp file: %w", err)
	}
	encoder := json.NewEncoder(tmp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(entries); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return fmt.Errorf("routes: encode file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("routes: close temp file: %w", err)
	}
	if err := os.Rename(tmp.Name(), f.path); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("routes: replace file: %w", err)
	}
	return nil
}
