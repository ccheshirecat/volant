package pluginspec

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

const (
	// CmdlineKey is the kernel parameter key used to pass the manifest to the agent.
	CmdlineKey = "volant.manifest"
	// RuntimeKey is the kernel parameter key used to indicate runtime identifier.
	RuntimeKey = "volant.runtime"
	// PluginKey is the kernel parameter key used to indicate plugin name.
	PluginKey = "volant.plugin"
)

// Manifest captures the metadata required to register and boot a runtime plugin.
type Manifest struct {
	SchemaVersion string            `json:"schema_version"`
	Name          string            `json:"name"`
	Version       string            `json:"version"`
	Runtime       string            `json:"runtime"`
	Image         string            `json:"image,omitempty"`
	ImageDigest   string            `json:"image_digest,omitempty"`
	Resources     ResourceSpec      `json:"resources"`
	Actions       map[string]Action `json:"actions"`
	HealthCheck   HealthCheck       `json:"health_check"`
	Workload      Workload          `json:"workload"`
	Enabled       bool              `json:"enabled"`
	OpenAPI       string            `json:"openapi,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
}

// ResourceSpec captures runtime requirements for a plugin workload.
type ResourceSpec struct {
	CPUCores int `json:"cpu_cores"`
	MemoryMB int `json:"memory_mb"`
}

// Action describes an API surface exposed by the plugin.
type Action struct {
	Description string `json:"description"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	TimeoutMs   int64  `json:"timeout_ms"`
}

// HealthCheck defines a basic probe configuration.
type HealthCheck struct {
	Endpoint string `json:"endpoint"`
	Timeout  int64  `json:"timeout_ms"`
}

// Workload defines how the agent should interact with the plugin runtime.
type Workload struct {
	Type       string            `json:"type"`
	BaseURL    string            `json:"base_url,omitempty"`
	Entrypoint []string          `json:"entrypoint,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
}

// Validate reports an error when required manifest fields are missing or inconsistent.
func (m Manifest) Validate() error {
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("plugin manifest: name required")
	}
	if strings.TrimSpace(m.Version) == "" {
		return fmt.Errorf("plugin manifest: version required")
	}
	if strings.TrimSpace(m.Runtime) == "" {
		return fmt.Errorf("plugin manifest: runtime required")
	}
	if m.Resources.CPUCores <= 0 {
		return fmt.Errorf("plugin manifest: cpu_cores must be > 0")
	}
	if m.Resources.MemoryMB <= 0 {
		return fmt.Errorf("plugin manifest: memory_mb must be > 0")
	}
	if len(m.Actions) == 0 {
		return fmt.Errorf("plugin manifest: at least one action required")
	}
	for name, action := range m.Actions {
		if strings.TrimSpace(action.Method) == "" {
			return fmt.Errorf("plugin manifest: action %s missing method", name)
		}
		if strings.TrimSpace(action.Path) == "" {
			return fmt.Errorf("plugin manifest: action %s missing path", name)
		}
	}
	if err := m.Workload.Validate(); err != nil {
		return err
	}
	return nil
}

// Validate ensures the workload entry is coherent.
func (w Workload) Validate() error {
	typeNormalized := strings.TrimSpace(strings.ToLower(w.Type))
	if typeNormalized == "" {
		return fmt.Errorf("plugin manifest: workload type required")
	}
	switch typeNormalized {
	case "http":
		if strings.TrimSpace(w.BaseURL) == "" {
			return fmt.Errorf("plugin manifest: workload.base_url required for http workload")
		}
		if _, err := url.Parse(w.BaseURL); err != nil {
			return fmt.Errorf("plugin manifest: workload.base_url invalid: %w", err)
		}
	default:
		return fmt.Errorf("plugin manifest: workload type %q not supported", w.Type)
	}
	return nil
}

// Encode encodes the manifest as JSON, base64url encoded, for kernel cmdline transport.
func Encode(m Manifest) (string, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

// Decode decodes a base64url manifest string into a Manifest.
func Decode(value string) (Manifest, error) {
	var manifest Manifest
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return manifest, err
	}
	if err := json.Unmarshal(decoded, &manifest); err != nil {
		return manifest, err
	}
	return manifest, nil
}
