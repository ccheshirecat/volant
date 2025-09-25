package plugins

import (
	"fmt"
	"slices"
	"sync"
)

// Registry maintains plugin manifests in memory.
type Registry struct {
	mu        sync.RWMutex
	manifests map[string]Manifest
}

// NewRegistry constructs an empty registry populated with built-ins.
func NewRegistry() *Registry {
	r := &Registry{manifests: make(map[string]Manifest)}
	r.Register(BuiltInBrowserManifest)
	return r
}

// Register adds or updates a manifest entry.
func (r *Registry) Register(manifest Manifest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.manifests == nil {
		r.manifests = make(map[string]Manifest)
	}
	r.manifests[manifest.Name] = manifest
}

// Get retrieves the manifest for a plugin.
func (r *Registry) Get(name string) (Manifest, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	manifest, ok := r.manifests[name]
	return manifest, ok
}

// List returns the registered plugin names.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.manifests))
	for name := range r.manifests {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// ResolveAction determines which plugin and action should handle the request.
func (r *Registry) ResolveAction(pluginName, actionName string) (Manifest, Action, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	manifest, ok := r.manifests[pluginName]
	if !ok {
		return Manifest{}, Action{}, fmt.Errorf("plugin registry: plugin %s not found", pluginName)
	}
	action, ok := manifest.Actions[actionName]
	if !ok {
		return Manifest{}, Action{}, fmt.Errorf("plugin registry: action %s not defined for plugin %s", actionName, pluginName)
	}
	return manifest, action, nil
}
