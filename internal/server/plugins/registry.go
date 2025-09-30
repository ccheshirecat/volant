package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/ccheshirecat/volant/internal/pluginspec"
	"github.com/ccheshirecat/volant/internal/server/db"
)

// Registry manages plugin manifests at runtime.
type Registry struct {
	mu        sync.RWMutex
	backend   db.PluginRepository
	manifests map[string]pluginspec.Manifest
}

func NewRegistry(repo db.PluginRepository) *Registry {
	return &Registry{backend: repo, manifests: make(map[string]pluginspec.Manifest)}
}

func (r *Registry) Register(manifest pluginspec.Manifest) {
	manifest.Normalize()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.manifests[manifest.Name] = manifest
}

func (r *Registry) Get(name string) (pluginspec.Manifest, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	manifest, ok := r.manifests[name]
	return manifest, ok
}

func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.manifests))
	for name := range r.manifests {
		names = append(names, name)
	}
	return names
}

func (r *Registry) ResolveAction(plugin, action string) (pluginspec.Manifest, pluginspec.Action, error) {
	manifest, ok := r.Get(plugin)
	if !ok {
		return pluginspec.Manifest{}, pluginspec.Action{}, fmt.Errorf("plugin %s not found", plugin)
	}
	actionSpec, ok := manifest.Actions[action]
	if !ok {
		return pluginspec.Manifest{}, pluginspec.Action{}, fmt.Errorf("action %s not found", action)
	}
	return manifest, actionSpec, nil
}

func (r *Registry) Fetch(ctx context.Context, name string) (pluginspec.Manifest, error) {
	if r.backend == nil {
		return pluginspec.Manifest{}, errors.New("registry backend not configured")
	}
	plugin, err := r.backend.GetByName(ctx, name)
	if err != nil {
		return pluginspec.Manifest{}, err
	}
	if plugin == nil {
		return pluginspec.Manifest{}, fmt.Errorf("plugin %s not found", name)
	}
	var manifest pluginspec.Manifest
	if len(plugin.Metadata) > 0 {
		if err := json.Unmarshal(plugin.Metadata, &manifest); err != nil {
			return pluginspec.Manifest{}, err
		}
	}
	manifest.Name = plugin.Name
	manifest.Version = plugin.Version
	manifest.Enabled = plugin.Enabled
	manifest.Normalize()
	return manifest, nil
}

func (r *Registry) Persist(ctx context.Context, manifest pluginspec.Manifest, enabled bool) error {
	if r.backend == nil {
		return errors.New("registry backend not configured")
	}
	manifest.Normalize()
	if err := manifest.Validate(); err != nil {
		return err
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	return r.backend.Upsert(ctx, db.Plugin{
		Name:     manifest.Name,
		Version:  manifest.Version,
		Enabled:  enabled,
		Metadata: data,
	})
}

func (r *Registry) Remove(ctx context.Context, name string) error {
	if r.backend == nil {
		return errors.New("registry backend not configured")
	}
	if err := r.backend.Delete(ctx, name); err != nil {
		return err
	}
	r.mu.Lock()
	delete(r.manifests, name)
	r.mu.Unlock()
	return nil
}

func cloneLabelMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
