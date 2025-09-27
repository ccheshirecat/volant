package pluginspec

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

const (
	CmdlineKey = "volant.manifest"
	RuntimeKey = "volant.runtime"
	PluginKey  = "volant.plugin"
)

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

type ResourceSpec struct {
	CPUCores int `json:"cpu_cores"`
	MemoryMB int `json:"memory_mb"`
}

type Action struct {
	Description string `json:"description"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	TimeoutMs   int64  `json:"timeout_ms"`
}

type HealthCheck struct {
	Endpoint string `json:"endpoint"`
	Timeout  int64  `json:"timeout_ms"`
}

type Workload struct {
	Type       string            `json:"type"`
	BaseURL    string            `json:"base_url,omitempty"`
	Entrypoint []string          `json:"entrypoint,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
}

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

func Encode(m Manifest) (string, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

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
