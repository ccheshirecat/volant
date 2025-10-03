package vmconfig

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/volantvm/volant/internal/pluginspec"
	"github.com/volantvm/volant/internal/server/db"
)

// Resources captures compute resource settings for a VM.
type Resources struct {
	CPUCores int `json:"cpu_cores"`
	MemoryMB int `json:"memory_mb"`
}

// API stores host-side connectivity preferences for the VM agent.
type API struct {
	Host string `json:"host,omitempty"`
	Port string `json:"port,omitempty"`
}

// Expose defines a workload port exposure rule.
type Expose struct {
	Name     string `json:"name,omitempty"`
	Protocol string `json:"protocol,omitempty"`
	Port     int    `json:"port"`
	HostPort int    `json:"host_port,omitempty"`
}

// Config represents the persisted, user-editable configuration of a VM.
type Config struct {
	Plugin         string                    `json:"plugin"`
	Runtime        string                    `json:"runtime,omitempty"`
	KernelCmdline  string                    `json:"kernel_cmdline,omitempty"`
	KernelOverride string                    `json:"kernel_override,omitempty"`
	Resources      Resources                 `json:"resources"`
	API            API                       `json:"api,omitempty"`
	Manifest       *pluginspec.Manifest      `json:"manifest,omitempty"`
	Metadata       map[string]any            `json:"metadata,omitempty"`
	Expose         []Expose                  `json:"expose,omitempty"`
	CloudInit      *pluginspec.CloudInit     `json:"cloud_init,omitempty"`
	Network        *pluginspec.NetworkConfig `json:"network,omitempty"`
	Initramfs      *pluginspec.Initramfs     `json:"initramfs,omitempty"`
	RootFS         *pluginspec.RootFS        `json:"rootfs,omitempty"`
}

// Versioned associates a configuration with its version metadata.
type Versioned struct {
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
	Config    Config    `json:"config"`
}

// HistoryEntry captures an historical configuration snapshot.
type HistoryEntry struct {
	ID        int64     `json:"id"`
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
	Config    Config    `json:"config"`
}

// Patch represents a partial configuration update request.
type Patch struct {
	Runtime       *string                   `json:"runtime,omitempty"`
	KernelCmdline *string                   `json:"kernel_cmdline,omitempty"`
	Resources     *ResourcesPatch           `json:"resources,omitempty"`
	API           *APIPatch                 `json:"api,omitempty"`
	Manifest      *pluginspec.Manifest      `json:"manifest,omitempty"`
	Metadata      *map[string]any           `json:"metadata,omitempty"`
	Expose        *[]Expose                 `json:"expose,omitempty"`
	CloudInit     *pluginspec.CloudInit     `json:"cloud_init,omitempty"`
	Network       *pluginspec.NetworkConfig `json:"network,omitempty"`
	// Optional boot media overrides
	KernelOverride *string               `json:"kernel_override,omitempty"`
	Initramfs      *pluginspec.Initramfs `json:"initramfs,omitempty"`
	RootFS         *pluginspec.RootFS    `json:"rootfs,omitempty"`
}

// ResourcesPatch allows partial updates of compute resources.
type ResourcesPatch struct {
	CPUCores *int `json:"cpu_cores,omitempty"`
	MemoryMB *int `json:"memory_mb,omitempty"`
}

// APIPatch allows partial API host/port updates.
type APIPatch struct {
	Host *string `json:"host,omitempty"`
	Port *string `json:"port,omitempty"`
}

// Clone creates a deep copy of the configuration.
func (c Config) Clone() Config {
	clone := c
	if c.Manifest != nil {
		manifestCopy := *c.Manifest
		clone.Manifest = &manifestCopy
	}
	if c.CloudInit != nil {
		cloudCopy := *c.CloudInit
		cloudCopy.Normalize()
		clone.CloudInit = &cloudCopy
	}
	if c.Initramfs != nil {
		initCopy := *c.Initramfs
		clone.Initramfs = &initCopy
	}
	if c.RootFS != nil {
		rootCopy := *c.RootFS
		clone.RootFS = &rootCopy
	}
	if c.Metadata != nil {
		metaCopy := make(map[string]any, len(c.Metadata))
		for k, v := range c.Metadata {
			metaCopy[k] = v
		}
		clone.Metadata = metaCopy
	}
	if len(c.Expose) > 0 {
		exposeCopy := make([]Expose, len(c.Expose))
		copy(exposeCopy, c.Expose)
		clone.Expose = exposeCopy
	}
	return clone
}

// Normalize trims fields and normalizes embedded manifests.
func (c *Config) Normalize() {
	if c == nil {
		return
	}
	c.Plugin = strings.TrimSpace(c.Plugin)
	c.Runtime = strings.TrimSpace(c.Runtime)
	c.KernelCmdline = strings.TrimSpace(c.KernelCmdline)
	c.KernelOverride = strings.TrimSpace(c.KernelOverride)
	c.API.Host = strings.TrimSpace(c.API.Host)
	c.API.Port = strings.TrimSpace(c.API.Port)
	for i := range c.Expose {
		c.Expose[i].Name = strings.TrimSpace(c.Expose[i].Name)
		c.Expose[i].Protocol = strings.TrimSpace(strings.ToLower(c.Expose[i].Protocol))
	}
	if c.Manifest != nil {
		manifestCopy := *c.Manifest
		manifestCopy.Normalize()
		c.Manifest = &manifestCopy
	}
	if c.CloudInit != nil {
		cloudCopy := *c.CloudInit
		cloudCopy.Normalize()
		c.CloudInit = &cloudCopy
	}
	if c.Initramfs != nil {
		initCopy := *c.Initramfs
		initCopy.URL = strings.TrimSpace(initCopy.URL)
		initCopy.Checksum = strings.TrimSpace(initCopy.Checksum)
		c.Initramfs = &initCopy
	}
	if c.RootFS != nil {
		rootCopy := *c.RootFS
		rootCopy.URL = strings.TrimSpace(rootCopy.URL)
		rootCopy.Checksum = strings.TrimSpace(rootCopy.Checksum)
		rootCopy.Format = strings.TrimSpace(strings.ToLower(rootCopy.Format))
		c.RootFS = &rootCopy
	}
}

// Validate performs semantic validation on the configuration.
func (c Config) Validate() error {
	if strings.TrimSpace(c.Plugin) == "" {
		return fmt.Errorf("vmconfig: plugin is required")
	}
	if strings.TrimSpace(c.Runtime) == "" {
		return fmt.Errorf("vmconfig: runtime is required")
	}
	if c.Manifest == nil {
		return fmt.Errorf("vmconfig: manifest is required")
	}
	if c.Resources.CPUCores <= 0 {
		return fmt.Errorf("vmconfig: cpu_cores must be greater than zero")
	}
	if c.Resources.MemoryMB <= 0 {
		return fmt.Errorf("vmconfig: memory_mb must be greater than zero")
	}
	for _, rule := range c.Expose {
		if rule.Port <= 0 {
			return fmt.Errorf("vmconfig: expose port must be greater than zero")
		}
		if rule.HostPort < 0 {
			return fmt.Errorf("vmconfig: expose host_port cannot be negative")
		}
	}
	if c.CloudInit != nil {
		if err := c.CloudInit.Validate(); err != nil {
			return fmt.Errorf("vmconfig: %w", err)
		}
	}
	if c.Network != nil {
		if err := c.Network.Validate(); err != nil {
			return fmt.Errorf("vmconfig: %w", err)
		}
	}
	// Optional boot media overrides: allow at most one of initramfs or rootfs
	initramfsSet := c.Initramfs != nil && strings.TrimSpace(c.Initramfs.URL) != ""
	rootfsSet := c.RootFS != nil && strings.TrimSpace(c.RootFS.URL) != ""
	if initramfsSet && rootfsSet {
		return fmt.Errorf("vmconfig: cannot set both initramfs and rootfs overrides")
	}
	if c.Initramfs != nil {
		if err := c.Initramfs.Validate(); err != nil {
			return fmt.Errorf("vmconfig: %w", err)
		}
	}
	if c.RootFS != nil {
		if err := c.RootFS.Validate(); err != nil {
			return fmt.Errorf("vmconfig: %w", err)
		}
	}
	return nil
}

// Marshal serialises the configuration to JSON with normalization and validation.
func Marshal(c Config) ([]byte, error) {
	clone := c.Clone()
	clone.Normalize()
	if err := clone.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(clone)
}

// Unmarshal decodes JSON into a configuration while enforcing normalization.
func Unmarshal(data []byte) (Config, error) {
	if len(data) == 0 {
		return Config{}, fmt.Errorf("vmconfig: empty configuration payload")
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("vmconfig: decode: %w", err)
	}
	cfg.Normalize()
	icopy := cfg.Clone()
	if err := icopy.Validate(); err != nil {
		return Config{}, err
	}
	return icopy, nil
}

// Apply merges a patch into the base configuration.
func (p Patch) Apply(base Config) (Config, error) {
	updated := base.Clone()
	if p.Runtime != nil {
		updated.Runtime = strings.TrimSpace(*p.Runtime)
	}
	if p.KernelCmdline != nil {
		updated.KernelCmdline = strings.TrimSpace(*p.KernelCmdline)
	}
	if p.KernelOverride != nil {
		updated.KernelOverride = strings.TrimSpace(*p.KernelOverride)
	}
	if p.Resources != nil {
		if p.Resources.CPUCores != nil {
			updated.Resources.CPUCores = *p.Resources.CPUCores
		}
		if p.Resources.MemoryMB != nil {
			updated.Resources.MemoryMB = *p.Resources.MemoryMB
		}
	}
	if p.API != nil {
		if p.API.Host != nil {
			updated.API.Host = strings.TrimSpace(*p.API.Host)
		}
		if p.API.Port != nil {
			updated.API.Port = strings.TrimSpace(*p.API.Port)
		}
	}
	if p.Manifest != nil {
		manifestCopy := *p.Manifest
		manifestCopy.Normalize()
		updated.Manifest = &manifestCopy
	}
	if p.Metadata != nil {
		if *p.Metadata == nil {
			updated.Metadata = nil
		} else {
			metaCopy := make(map[string]any, len(*p.Metadata))
			for k, v := range *p.Metadata {
				metaCopy[k] = v
			}
			updated.Metadata = metaCopy
		}
	}
	if p.Expose != nil {
		if len(*p.Expose) == 0 {
			updated.Expose = nil
		} else {
			exposeCopy := make([]Expose, len(*p.Expose))
			copy(exposeCopy, *p.Expose)
			updated.Expose = exposeCopy
		}
	}
	if p.CloudInit != nil {
		cloudCopy := *p.CloudInit
		cloudCopy.Normalize()
		updated.CloudInit = &cloudCopy
	}
	if p.Initramfs != nil {
		initCopy := *p.Initramfs
		updated.Initramfs = &initCopy
	}
	if p.RootFS != nil {
		rootCopy := *p.RootFS
		updated.RootFS = &rootCopy
	}

	updated.Normalize()
	if err := updated.Validate(); err != nil {
		return Config{}, err
	}
	return updated.Clone(), nil
}

// FromDB converts a database record into a versioned configuration.
func FromDB(record db.VMConfig) (Versioned, error) {
	cfg, err := Unmarshal(record.ConfigJSON)
	if err != nil {
		return Versioned{}, err
	}
	return Versioned{
		Version:   record.Version,
		UpdatedAt: record.UpdatedAt,
		Config:    cfg,
	}, nil
}

// FromHistory converts a database history entry into a configuration snapshot.
func FromHistory(entry db.VMConfigHistoryEntry) (HistoryEntry, error) {
	cfg, err := Unmarshal(entry.ConfigJSON)
	if err != nil {
		return HistoryEntry{}, err
	}
	return HistoryEntry{
		ID:        entry.ID,
		Version:   entry.Version,
		UpdatedAt: entry.UpdatedAt,
		Config:    cfg,
	}, nil
}
