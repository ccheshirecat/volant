package plugins

import "fmt"

// Manifest captures the metadata required to register a runtime plugin.
type Manifest struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Runtime     string            `json:"runtime"`
	Image       string            `json:"image"`
	Resources   ResourceSpec      `json:"resources"`
	Actions     map[string]Action `json:"actions"`
	HealthCheck HealthCheck       `json:"health_check"`
	Enabled     bool              `json:"enabled"`
	OpenAPI     string            `json:"openapi,omitempty"`
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

// Validate reports an error when required manifest fields are missing.
func (m Manifest) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("plugin manifest: name required")
	}
	if m.Runtime == "" {
		return fmt.Errorf("plugin manifest: runtime required")
	}
	if m.Image == "" {
		return fmt.Errorf("plugin manifest: image required")
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
		if action.Method == "" {
			return fmt.Errorf("plugin manifest: action %s missing method", name)
		}
		if action.Path == "" {
			return fmt.Errorf("plugin manifest: action %s missing path", name)
		}
	}
	return nil
}
