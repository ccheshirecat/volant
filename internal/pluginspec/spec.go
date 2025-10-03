// Copyright (c) 2025 HYPR. PTE. LTD.
//
// Business Source License 1.1
// See LICENSE file in the project root for details.

package pluginspec

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
)

const (
	// CmdlineKey is the kernel parameter key used to pass the manifest to the agent.
	CmdlineKey = "volant.manifest"
	// RuntimeKey is the kernel parameter key used to indicate runtime identifier.
	RuntimeKey = "volant.runtime"
	// PluginKey is the kernel parameter key used to indicate plugin name.
	PluginKey = "volant.plugin"
	// APIHostKey is the kernel parameter key for the host API hostname/IP.
	APIHostKey = "volant.api_host"
	// APIPortKey is the kernel parameter key for the host API port.
	APIPortKey = "volant.api_port"
	// RootFSKey is the kernel parameter key used to provide the plugin rootfs URL.
	RootFSKey = "volant.rootfs"
	// RootFSChecksumKey is the kernel parameter key for the rootfs checksum.
	RootFSChecksumKey = "volant.rootfs_checksum"
	// RootFSDeviceKey indicates the block device name for the root filesystem inside the guest.
	RootFSDeviceKey = "volant.rootfs_device"
	// RootFSFSTypeKey indicates the filesystem type for the root filesystem device.
	RootFSFSTypeKey = "volant.rootfs_fstype"
)

// Manifest captures the metadata required to register and boot a runtime plugin.
type Manifest struct {
	SchemaVersion string            `json:"schema_version"`
	Name          string            `json:"name"`
	Version       string            `json:"version"`
	Runtime       string            `json:"runtime"`
	RootFS        RootFS            `json:"rootfs"`
	Initramfs     Initramfs         `json:"initramfs"`
	Disks         []Disk            `json:"disks,omitempty"`
	Image         string            `json:"image,omitempty"`
	ImageDigest   string            `json:"image_digest,omitempty"`
	Resources     ResourceSpec      `json:"resources"`
	Actions       map[string]Action `json:"actions,omitempty"`
	HealthCheck   HealthCheck       `json:"health_check"`
	Workload      Workload          `json:"workload"`
	CloudInit     *CloudInit        `json:"cloud_init,omitempty"`
	Network       *NetworkConfig    `json:"network,omitempty"`
	Enabled       bool              `json:"enabled"`
	OpenAPI       string            `json:"openapi,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
}

type RootFS struct {
	URL      string `json:"url"`
	Checksum string `json:"checksum,omitempty"`
	Format   string `json:"format,omitempty"`
}

type Initramfs struct {
	URL      string `json:"url"`
	Checksum string `json:"checksum,omitempty"`
}

type Disk struct {
	Name     string `json:"name"`
	Source   string `json:"source"`
	Checksum string `json:"checksum,omitempty"`
	Format   string `json:"format,omitempty"`
	Readonly bool   `json:"readonly"`
	Target   string `json:"target,omitempty"`
}

type CloudInit struct {
	Datasource string       `json:"datasource,omitempty"`
	SeedMode   string       `json:"seed_mode,omitempty"`
	UserData   CloudInitDoc `json:"user_data,omitempty"`
	MetaData   CloudInitDoc `json:"meta_data,omitempty"`
	NetworkCfg CloudInitDoc `json:"network_config,omitempty"`
}

type CloudInitDoc struct {
	Inline  bool   `json:"inline,omitempty"`
	Content string `json:"content,omitempty"`
	Path    string `json:"path,omitempty"`
}

var allowedDiskFormats = map[string]struct{}{
	"raw":   {},
	"qcow2": {},
}

func normalizeFormat(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func (d *Disk) Normalize() {
	if d == nil {
		return
	}
	d.Name = strings.TrimSpace(d.Name)
	d.Source = strings.TrimSpace(d.Source)
	d.Format = normalizeFormat(d.Format)
	d.Target = strings.TrimSpace(d.Target)
}

func (d Disk) Validate() error {
	if strings.TrimSpace(d.Name) == "" {
		return fmt.Errorf("disk name required")
	}
	if strings.TrimSpace(d.Source) == "" {
		return fmt.Errorf("disk %s: source required", d.Name)
	}
	format := normalizeFormat(d.Format)
	if format == "" {
		format = "raw"
	}
	if _, ok := allowedDiskFormats[format]; !ok {
		return fmt.Errorf("disk %s: unsupported format %q", d.Name, d.Format)
	}
	return nil
}

func (c *CloudInit) Normalize() {
	if c == nil {
		return
	}
	c.Datasource = strings.TrimSpace(c.Datasource)
	c.SeedMode = strings.TrimSpace(strings.ToLower(c.SeedMode))
	c.UserData.Normalize()
	c.MetaData.Normalize()
	c.NetworkCfg.Normalize()
}

func (c CloudInit) Validate() error {
	if strings.TrimSpace(c.Datasource) == "" {
		return fmt.Errorf("cloud_init: datasource required")
	}
	seedMode := strings.TrimSpace(strings.ToLower(c.SeedMode))
	if seedMode == "" {
		seedMode = "vfat"
	}
	if seedMode != "vfat" {
		return fmt.Errorf("cloud_init: unsupported seed_mode %q", c.SeedMode)
	}
	if err := c.UserData.Validate("user_data"); err != nil {
		return err
	}
	if err := c.MetaData.Validate("meta_data"); err != nil {
		return err
	}
	if err := c.NetworkCfg.Validate("network_config"); err != nil {
		return err
	}
	return nil
}

func (d *CloudInitDoc) Normalize() {
	if d == nil {
		return
	}
	d.Content = strings.TrimSpace(d.Content)
	d.Path = strings.TrimSpace(d.Path)
}

func (d CloudInitDoc) Validate(field string) error {
	if d.Content != "" && d.Path != "" {
		return fmt.Errorf("cloud_init.%s: content and path cannot both be set", field)
	}
	if d.Content != "" && !d.Inline {
		return fmt.Errorf("cloud_init.%s: inline must be true when content provided", field)
	}
	if d.Path != "" && d.Inline {
		return fmt.Errorf("cloud_init.%s: inline must be false when path provided", field)
	}
	return nil
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
	WorkDir    string            `json:"workdir,omitempty"`
}

// Validate reports an error when required manifest fields are missing or inconsistent.
func (m Manifest) Validate() error {
	normalized := m
	normalized.Normalize()

	if strings.TrimSpace(normalized.Name) == "" {
		return fmt.Errorf("plugin manifest: name required")
	}
	if strings.TrimSpace(normalized.Version) == "" {
		return fmt.Errorf("plugin manifest: version required")
	}
	if strings.TrimSpace(normalized.Runtime) == "" {
		return fmt.Errorf("plugin manifest: runtime required")
	}
	if normalized.Resources.CPUCores <= 0 {
		return fmt.Errorf("plugin manifest: cpu_cores must be > 0")
	}
	if normalized.Resources.MemoryMB <= 0 {
		return fmt.Errorf("plugin manifest: memory_mb must be > 0")
	}
	for name, action := range normalized.Actions {
		if strings.TrimSpace(action.Method) == "" {
			return fmt.Errorf("plugin manifest: action %s missing method", name)
		}
		if strings.TrimSpace(action.Path) == "" {
			return fmt.Errorf("plugin manifest: action %s missing path", name)
		}
	}
	if err := normalized.Workload.Validate(); err != nil {
		return err
	}
	// Enforce that exactly one of rootfs or initramfs is specified for boot media by default.
	// Advanced users may override at runtime via per-VM configuration.
	if err := normalized.RootFS.Validate(); err != nil {
		return err
	}
	if err := normalized.Initramfs.Validate(); err != nil {
		return err
	}
	rootfsSet := strings.TrimSpace(normalized.RootFS.URL) != ""
	initramfsSet := strings.TrimSpace(normalized.Initramfs.URL) != ""
	if rootfsSet == initramfsSet { // both true or both false
		return fmt.Errorf("plugin manifest: exactly one of rootfs.url or initramfs.url must be set")
	}
	for _, disk := range normalized.Disks {
		if err := disk.Validate(); err != nil {
			return fmt.Errorf("plugin manifest: %w", err)
		}
	}
	if normalized.CloudInit != nil {
		if err := normalized.CloudInit.Validate(); err != nil {
			return fmt.Errorf("plugin manifest: %w", err)
		}
	}
	if normalized.Network != nil {
		if err := normalized.Network.Validate(); err != nil {
			return fmt.Errorf("plugin manifest: %w", err)
		}
	}
	return nil
}

// Normalize trims whitespace, sets sensible defaults, and ensures mandatory labels are present.
func (m *Manifest) Normalize() {
	if m == nil {
		return
	}
	m.SchemaVersion = strings.TrimSpace(m.SchemaVersion)
	m.Name = strings.TrimSpace(m.Name)
	m.Version = strings.TrimSpace(m.Version)
	m.Runtime = strings.TrimSpace(m.Runtime)
	if m.Runtime == "" {
		m.Runtime = m.Name
	}
	m.Image = strings.TrimSpace(m.Image)
	m.ImageDigest = strings.TrimSpace(m.ImageDigest)
	m.OpenAPI = strings.TrimSpace(m.OpenAPI)
	m.RootFS.URL = strings.TrimSpace(m.RootFS.URL)
	m.RootFS.Checksum = strings.TrimSpace(m.RootFS.Checksum)
	m.RootFS.Format = normalizeFormat(m.RootFS.Format)
	if m.RootFS.Format == "" {
		m.RootFS.Format = "raw"
	}
	m.Initramfs.URL = strings.TrimSpace(m.Initramfs.URL)
	m.Initramfs.Checksum = strings.TrimSpace(m.Initramfs.Checksum)
	if len(m.Disks) > 0 {
		for i := range m.Disks {
			m.Disks[i].Normalize()
		}
	}
	if m.CloudInit != nil {
		m.CloudInit.Normalize()
		if strings.TrimSpace(m.CloudInit.Datasource) == "" {
			m.CloudInit.Datasource = "NoCloud"
		}
		if strings.TrimSpace(m.CloudInit.SeedMode) == "" {
			m.CloudInit.SeedMode = "vfat"
		}
	}

	if m.Network != nil {
		m.Network.Normalize()
	}

	m.Workload.Type = strings.TrimSpace(m.Workload.Type)
	m.Workload.BaseURL = strings.TrimSpace(m.Workload.BaseURL)
	m.Workload.WorkDir = strings.TrimSpace(m.Workload.WorkDir)
	if len(m.Workload.Entrypoint) > 0 {
		trimmed := make([]string, 0, len(m.Workload.Entrypoint))
		for _, arg := range m.Workload.Entrypoint {
			value := strings.TrimSpace(arg)
			if value != "" {
				trimmed = append(trimmed, value)
			}
		}
		m.Workload.Entrypoint = trimmed
	}
	if len(m.Workload.Env) > 0 {
		for key, value := range m.Workload.Env {
			trimmedKey := strings.TrimSpace(key)
			trimmedValue := strings.TrimSpace(value)
			if trimmedKey == "" {
				delete(m.Workload.Env, key)
				continue
			}
			if trimmedKey != key || trimmedValue != value {
				delete(m.Workload.Env, key)
				m.Workload.Env[trimmedKey] = trimmedValue
			} else {
				m.Workload.Env[key] = trimmedValue
			}
		}
	}

	if len(m.Actions) > 0 {
		for name, action := range m.Actions {
			trimmedName := strings.TrimSpace(name)
			if trimmedName == "" {
				delete(m.Actions, name)
				continue
			}
			action.Description = strings.TrimSpace(action.Description)
			action.Method = strings.TrimSpace(action.Method)
			action.Path = strings.TrimSpace(action.Path)
			m.Actions[trimmedName] = action
			if trimmedName != name {
				delete(m.Actions, name)
			}
		}
		if len(m.Actions) == 0 {
			m.Actions = nil
		}
	}

	if m.Labels == nil {
		m.Labels = make(map[string]string)
	}
	if m.Name != "" {
		m.Labels["volant.plugin"] = m.Name
	}
	if m.Runtime != "" {
		m.Labels["volant.runtime"] = m.Runtime
	}
	if len(m.Labels) == 0 {
		m.Labels = nil
	} else {
		normalized := make(map[string]string, len(m.Labels))
		keys := make([]string, 0, len(m.Labels))
		for key := range m.Labels {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			normalized[key] = strings.TrimSpace(m.Labels[key])
		}
		m.Labels = normalized
	}
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
		if len(w.Entrypoint) == 0 || strings.TrimSpace(w.Entrypoint[0]) == "" {
			return fmt.Errorf("plugin manifest: workload.entrypoint required for http workload")
		}
	default:
		return fmt.Errorf("plugin manifest: workload type %q not supported", w.Type)
	}
	return nil
}

func (r RootFS) Validate() error {
	url := strings.TrimSpace(r.URL)
	if url == "" {
		return nil
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "file://") && !strings.HasPrefix(url, "/") {
		return fmt.Errorf("plugin manifest: rootfs url must be http(s), file://, or absolute path")
	}
	checksum := strings.TrimSpace(r.Checksum)
	if checksum != "" && !strings.Contains(checksum, ":") && len(checksum) < 32 {
		return fmt.Errorf("plugin manifest: rootfs checksum should include algorithm prefix or be a sha256")
	}
	format := normalizeFormat(r.Format)
	if format == "" {
		format = "raw"
	}
	if _, ok := allowedDiskFormats[format]; !ok {
		return fmt.Errorf("plugin manifest: rootfs format %q not supported", r.Format)
	}
	return nil
}

func (i Initramfs) Validate() error {
	url := strings.TrimSpace(i.URL)
	if url == "" {
		return nil
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "file://") && !strings.HasPrefix(url, "/") {
		return fmt.Errorf("plugin manifest: initramfs url must be http(s), file://, or absolute path")
	}
	checksum := strings.TrimSpace(i.Checksum)
	if checksum != "" && !strings.Contains(checksum, ":") && len(checksum) < 32 {
		return fmt.Errorf("plugin manifest: initramfs checksum should include algorithm prefix or be a sha256")
	}
	return nil
}

// Encode encodes the manifest as JSON, base64url encoded, for kernel cmdline transport.
func Encode(m Manifest) (string, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		return "", err
	}
	if err := zw.Close(); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf.Bytes()), nil
}

// Decode decodes a base64url manifest string into a Manifest.
func Decode(value string) (Manifest, error) {
	var manifest Manifest
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return manifest, err
	}
	reader, err := gzip.NewReader(bytes.NewReader(decoded))
	if err != nil {
		// Fallback to interpreting the payload as raw JSON (legacy encoding)
		if err := json.Unmarshal(decoded, &manifest); err != nil {
			return manifest, err
		}
		return manifest, nil
	}
	decompressed, err := io.ReadAll(reader)
	if err != nil {
		return manifest, err
	}
	if err := reader.Close(); err != nil {
		return manifest, err
	}
	if err := json.Unmarshal(decompressed, &manifest); err != nil {
		return manifest, err
	}
	return manifest, nil
}

// NetworkMode defines how the VM connects to the network.
type NetworkMode string

const (
	// NetworkModeVsock provides isolated communication via vsock (no IP networking).
	NetworkModeVsock NetworkMode = "vsock"
	// NetworkModeBridged attaches the VM to a bridge with IP networking.
	NetworkModeBridged NetworkMode = "bridged"
	// NetworkModeDHCP attaches the VM to a bridge with DHCP-based IP assignment.
	NetworkModeDHCP NetworkMode = "dhcp"
)

// NetworkConfig defines plugin-level network configuration defaults.
type NetworkConfig struct {
	Mode       NetworkMode `json:"mode"`
	Subnet     string      `json:"subnet,omitempty"`      // For bridged mode: CIDR (e.g., "10.1.0.0/24")
	Gateway    string      `json:"gateway,omitempty"`     // For bridged mode: gateway IP
	AutoAssign bool        `json:"auto_assign,omitempty"` // For bridged mode: auto-allocate IPs from subnet
}

// Normalize trims and normalizes network configuration fields.
func (n *NetworkConfig) Normalize() {
	if n == nil {
		return
	}
	n.Mode = NetworkMode(strings.ToLower(strings.TrimSpace(string(n.Mode))))
	n.Subnet = strings.TrimSpace(n.Subnet)
	n.Gateway = strings.TrimSpace(n.Gateway)
}

// Validate checks network configuration for semantic correctness.
func (n NetworkConfig) Validate() error {
	mode := strings.ToLower(strings.TrimSpace(string(n.Mode)))
	switch NetworkMode(mode) {
	case NetworkModeVsock:
		// No additional fields needed for vsock
	case NetworkModeBridged:
		// Subnet and gateway are optional (can be auto-assigned by orchestrator)
		if n.Subnet != "" && n.Gateway == "" && !n.AutoAssign {
			return fmt.Errorf("network: bridged mode with subnet requires gateway or auto_assign")
		}
	case NetworkModeDHCP:
		// DHCP mode needs no additional config
	default:
		return fmt.Errorf("network: unsupported mode %q (must be vsock, bridged, or dhcp)", n.Mode)
	}
	return nil
}
