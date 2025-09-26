package plugins

import (
	"fmt"
	"slices"
	"strings"
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

// Remove deletes a manifest from the registry.
func (r *Registry) Remove(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.manifests, name)
}

// SetEnabled toggles a manifest's enabled flag in memory.
func (r *Registry) SetEnabled(name string, enabled bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	manifest, ok := r.manifests[name]
	if !ok {
		return
	}
	manifest.Enabled = enabled
	r.manifests[name] = manifest
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
	if strings.TrimSpace(pluginName) == "" {
		return Manifest{}, Action{}, fmt.Errorf("plugin registry: plugin name required")
	}
	if strings.TrimSpace(actionName) == "" {
		return Manifest{}, Action{}, fmt.Errorf("plugin registry: action name required")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	manifest, ok := r.manifests[pluginName]
	if !ok {
		return Manifest{}, Action{}, fmt.Errorf("plugin registry: plugin %s not found", pluginName)
	}
	if !manifest.Enabled {
		return Manifest{}, Action{}, fmt.Errorf("plugin registry: plugin %s disabled", pluginName)
	}
	action, ok := manifest.Actions[actionName]
	if !ok {
		return Manifest{}, Action{}, fmt.Errorf("plugin registry: action %s not defined for plugin %s", actionName, pluginName)
	}
	return manifest, action, nil
}
