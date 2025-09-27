package plugins

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/ccheshirecat/volant/internal/pluginspec"
)

// Registry maintains plugin manifests in memory.
type Registry struct {
	mu        sync.RWMutex
	manifests map[string]pluginspec.Manifest
}

// NewRegistry constructs an empty registry populated with built-ins.
func NewRegistry(builtins ...pluginspec.Manifest) *Registry {
	r := &Registry{manifests: make(map[string]pluginspec.Manifest)}
	for _, manifest := range builtins {
		r.Register(manifest)
	}
	return r
}

// Register adds or updates a manifest entry.
func (r *Registry) Register(manifest pluginspec.Manifest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.manifests == nil {
		r.manifests = make(map[string]pluginspec.Manifest)
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
func (r *Registry) Get(name string) (pluginspec.Manifest, bool) {
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
func (r *Registry) ResolveAction(pluginName, actionName string) (pluginspec.Manifest, pluginspec.Action, error) {
	if strings.TrimSpace(pluginName) == "" {
		return pluginspec.Manifest{}, pluginspec.Action{}, fmt.Errorf("plugin registry: plugin name required")
	}
	if strings.TrimSpace(actionName) == "" {
		return pluginspec.Manifest{}, pluginspec.Action{}, fmt.Errorf("plugin registry: action name required")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	manifest, ok := r.manifests[pluginName]
	if !ok {
		return pluginspec.Manifest{}, pluginspec.Action{}, fmt.Errorf("plugin registry: plugin %s not found", pluginName)
	}
	if !manifest.Enabled {
		return pluginspec.Manifest{}, pluginspec.Action{}, fmt.Errorf("plugin registry: plugin %s disabled", pluginName)
	}
	action, ok := manifest.Actions[actionName]
	if !ok {
		return pluginspec.Manifest{}, pluginspec.Action{}, fmt.Errorf("plugin registry: action %s not defined for plugin %s", actionName, pluginName)
	}
	return manifest, action, nil
}
