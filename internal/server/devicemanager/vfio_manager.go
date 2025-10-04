// Copyright (c) 2025 HYPR PTE. LTD.
//
// Business Source License 1.1
// See LICENSE file in the project root for details.

package devicemanager

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	// sysfs paths for PCI devices and IOMMU groups
	pciDevicesPath  = "/sys/bus/pci/devices"
	iommuGroupsPath = "/sys/kernel/iommu_groups"

	// VFIO driver and paths
	vfioPCIDriver = "vfio-pci"
	vfioDevPath   = "/dev/vfio"
)

var (
	// Regular expression to validate PCI address format: 0000:01:00.0
	pciAddressRegex = regexp.MustCompile(`^[0-9a-fA-F]{4}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}\.[0-7]$`)
)

// VFIOManager manages VFIO device passthrough operations
type VFIOManager interface {
	// ValidateDevices validates PCI addresses and checks against allowlist
	ValidateDevices(pciAddrs []string, allowlist []string) error

	// CheckIOMMUGroups returns IOMMU group information for the specified devices
	CheckIOMMUGroups(pciAddrs []string) ([]IOMMUGroup, error)

	// BindDevices binds devices to the vfio-pci driver
	BindDevices(pciAddrs []string) error

	// UnbindDevices unbinds devices from vfio-pci and restores original driver
	UnbindDevices(pciAddrs []string) error

	// GetVFIOGroupPaths returns /dev/vfio/GROUP_NUMBER paths for the devices
	GetVFIOGroupPaths(pciAddrs []string) ([]string, error)

	// GetDeviceInfo returns detailed information about a PCI device
	GetDeviceInfo(pciAddr string) (*PCIDevice, error)
}

// IOMMUGroup represents an IOMMU group with its devices
type IOMMUGroup struct {
	ID      string
	Devices []string
}

// PCIDevice represents a PCI device with its metadata
type PCIDevice struct {
	Address    string
	Vendor     string
	Device     string
	Driver     string
	IOMMUGroup string
	NumaNode   string
}

// vfioManager implements VFIOManager interface
type vfioManager struct {
	logger     *slog.Logger
	fileSystem FileSystem
}

// FileSystem interface for testability (allows mocking filesystem operations)
type FileSystem interface {
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte) error
	ReadDir(path string) ([]os.DirEntry, error)
	Exists(path string) bool
	Readlink(path string) (string, error)
}

// realFileSystem implements FileSystem using actual OS operations
type realFileSystem struct{}

func (fs *realFileSystem) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (fs *realFileSystem) WriteFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}

func (fs *realFileSystem) ReadDir(path string) ([]os.DirEntry, error) {
	return os.ReadDir(path)
}

func (fs *realFileSystem) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (fs *realFileSystem) Readlink(path string) (string, error) {
	return os.Readlink(path)
}

// NewVFIOManager creates a new VFIO manager with real filesystem operations
func NewVFIOManager(logger *slog.Logger) VFIOManager {
	return &vfioManager{
		logger:     logger.With("component", "vfio-manager"),
		fileSystem: &realFileSystem{},
	}
}

// newVFIOManagerWithFS creates a VFIO manager with custom filesystem (for testing)
func newVFIOManagerWithFS(logger *slog.Logger, fs FileSystem) VFIOManager {
	return &vfioManager{
		logger:     logger,
		fileSystem: fs,
	}
}

// ValidateDevices validates PCI addresses and checks against allowlist
func (m *vfioManager) ValidateDevices(pciAddrs []string, allowlist []string) error {
	if len(pciAddrs) == 0 {
		return nil // No devices to validate
	}

	m.logger.Debug("validating VFIO devices", "count", len(pciAddrs), "allowlist", allowlist)

	for _, addr := range pciAddrs {
		// Validate PCI address format
		if !isValidPCIAddress(addr) {
			return fmt.Errorf("invalid PCI address format: %s (expected format: 0000:01:00.0)", addr)
		}

		// Check if device exists in sysfs
		devicePath := filepath.Join(pciDevicesPath, addr)
		if !m.fileSystem.Exists(devicePath) {
			return fmt.Errorf("PCI device not found: %s", addr)
		}

		// Get device vendor:device ID
		vendor, device, err := m.getDeviceID(addr)
		if err != nil {
			return fmt.Errorf("failed to read device ID for %s: %w", addr, err)
		}

		// Check against allowlist if provided
		if len(allowlist) > 0 {
			if !matchesAllowlist(vendor, device, allowlist) {
				return fmt.Errorf("device %s (%s:%s) not in allowlist", addr, vendor, device)
			}
		}

		m.logger.Debug("device validated", "address", addr, "vendor", vendor, "device", device)
	}

	return nil
}

// CheckIOMMUGroups returns IOMMU group information for the specified devices
func (m *vfioManager) CheckIOMMUGroups(pciAddrs []string) ([]IOMMUGroup, error) {
	groups := make(map[string]*IOMMUGroup)

	for _, addr := range pciAddrs {
		groupID, err := m.getIOMMUGroup(addr)
		if err != nil {
			return nil, fmt.Errorf("failed to get IOMMU group for %s: %w", addr, err)
		}

		if groupID == "" {
			return nil, fmt.Errorf("device %s has no IOMMU group (IOMMU may not be enabled)", addr)
		}

		if _, exists := groups[groupID]; !exists {
			groups[groupID] = &IOMMUGroup{
				ID:      groupID,
				Devices: []string{},
			}
		}
		groups[groupID].Devices = append(groups[groupID].Devices, addr)

		m.logger.Debug("device IOMMU group", "address", addr, "group", groupID)
	}

	// Convert map to slice
	result := make([]IOMMUGroup, 0, len(groups))
	for _, group := range groups {
		result = append(result, *group)
	}

	return result, nil
}

// BindDevices binds devices to the vfio-pci driver
func (m *vfioManager) BindDevices(pciAddrs []string) error {
	m.logger.Info("binding devices to vfio-pci driver", "count", len(pciAddrs))

	for _, addr := range pciAddrs {
		// Check current driver
		currentDriver, err := m.getCurrentDriver(addr)
		if err != nil {
			m.logger.Warn("failed to get current driver", "device", addr, "error", err)
		}

		// If already bound to vfio-pci, skip
		if currentDriver == vfioPCIDriver {
			m.logger.Debug("device already bound to vfio-pci", "address", addr)
			continue
		}

		// Unbind from current driver if any
		if currentDriver != "" {
			if err := m.unbindDriver(addr, currentDriver); err != nil {
				return fmt.Errorf("failed to unbind %s from %s: %w", addr, currentDriver, err)
			}
			m.logger.Debug("unbound from current driver", "address", addr, "driver", currentDriver)
		}

		// Get device vendor:device ID for driver override
		vendor, device, err := m.getDeviceID(addr)
		if err != nil {
			return fmt.Errorf("failed to read device ID for %s: %w", addr, err)
		}

		// Add device ID to vfio-pci driver's new_id to enable binding
		newIDPath := "/sys/bus/pci/drivers/vfio-pci/new_id"
		idString := fmt.Sprintf("%s %s", vendor, device)
		if err := m.fileSystem.WriteFile(newIDPath, []byte(idString)); err != nil {
			// Ignore "file exists" errors - device may already be registered
			if !strings.Contains(err.Error(), "exist") {
				m.logger.Warn("failed to add device ID to vfio-pci", "error", err)
			}
		}

		// Bind to vfio-pci driver
		bindPath := "/sys/bus/pci/drivers/vfio-pci/bind"
		if err := m.fileSystem.WriteFile(bindPath, []byte(addr)); err != nil {
			// Check if device is now bound (bind may fail if already bound)
			if newDriver, _ := m.getCurrentDriver(addr); newDriver == vfioPCIDriver {
				m.logger.Debug("device now bound to vfio-pci", "address", addr)
			} else {
				return fmt.Errorf("failed to bind %s to vfio-pci: %w", addr, err)
			}
		}

		m.logger.Info("successfully bound device to vfio-pci", "address", addr)
	}

	return nil
}

// UnbindDevices unbinds devices from vfio-pci driver
func (m *vfioManager) UnbindDevices(pciAddrs []string) error {
	m.logger.Info("unbinding devices from vfio-pci", "count", len(pciAddrs))

	for _, addr := range pciAddrs {
		currentDriver, err := m.getCurrentDriver(addr)
		if err != nil {
			m.logger.Warn("failed to get current driver during unbind", "device", addr, "error", err)
			continue
		}

		if currentDriver == vfioPCIDriver {
			if err := m.unbindDriver(addr, vfioPCIDriver); err != nil {
				m.logger.Warn("failed to unbind device", "address", addr, "error", err)
				// Continue with other devices
			} else {
				m.logger.Debug("unbound device from vfio-pci", "address", addr)
			}
		}
	}

	return nil
}

// GetVFIOGroupPaths returns /dev/vfio/GROUP_NUMBER paths for the devices
func (m *vfioManager) GetVFIOGroupPaths(pciAddrs []string) ([]string, error) {
	groupPaths := make(map[string]bool)

	for _, addr := range pciAddrs {
		groupID, err := m.getIOMMUGroup(addr)
		if err != nil {
			return nil, fmt.Errorf("failed to get IOMMU group for %s: %w", addr, err)
		}

		if groupID == "" {
			return nil, fmt.Errorf("device %s has no IOMMU group", addr)
		}

		groupPath := filepath.Join(vfioDevPath, groupID)

		// Verify the VFIO group device exists
		if !m.fileSystem.Exists(groupPath) {
			return nil, fmt.Errorf("VFIO group device not found: %s (device may not be bound to vfio-pci)", groupPath)
		}

		groupPaths[groupPath] = true
		m.logger.Debug("found VFIO group path", "device", addr, "group", groupID, "path", groupPath)
	}

	// Convert map to sorted slice
	result := make([]string, 0, len(groupPaths))
	for path := range groupPaths {
		result = append(result, path)
	}

	return result, nil
}

// GetDeviceInfo returns detailed information about a PCI device
func (m *vfioManager) GetDeviceInfo(pciAddr string) (*PCIDevice, error) {
	if !isValidPCIAddress(pciAddr) {
		return nil, fmt.Errorf("invalid PCI address: %s", pciAddr)
	}

	devicePath := filepath.Join(pciDevicesPath, pciAddr)
	if !m.fileSystem.Exists(devicePath) {
		return nil, fmt.Errorf("device not found: %s", pciAddr)
	}

	vendor, device, err := m.getDeviceID(pciAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to read device ID: %w", err)
	}

	driver, _ := m.getCurrentDriver(pciAddr)
	iommuGroup, _ := m.getIOMMUGroup(pciAddr)
	numaNode := m.getNumaNode(pciAddr)

	return &PCIDevice{
		Address:    pciAddr,
		Vendor:     vendor,
		Device:     device,
		Driver:     driver,
		IOMMUGroup: iommuGroup,
		NumaNode:   numaNode,
	}, nil
}

// Helper functions

// isValidPCIAddress validates PCI address format (e.g., 0000:01:00.0)
func isValidPCIAddress(addr string) bool {
	return pciAddressRegex.MatchString(addr)
}

// getDeviceID reads vendor and device ID from sysfs
func (m *vfioManager) getDeviceID(pciAddr string) (vendor, device string, err error) {
	vendorPath := filepath.Join(pciDevicesPath, pciAddr, "vendor")
	devicePath := filepath.Join(pciDevicesPath, pciAddr, "device")

	vendorBytes, err := m.fileSystem.ReadFile(vendorPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to read vendor ID: %w", err)
	}

	deviceBytes, err := m.fileSystem.ReadFile(devicePath)
	if err != nil {
		return "", "", fmt.Errorf("failed to read device ID: %w", err)
	}

	// Parse hex values (format: 0x10de\n)
	vendor = strings.TrimSpace(strings.TrimPrefix(string(vendorBytes), "0x"))
	device = strings.TrimSpace(strings.TrimPrefix(string(deviceBytes), "0x"))

	return vendor, device, nil
}

// matchesAllowlist checks if device vendor:device matches any pattern in allowlist
// Supports exact matches (10de:2204) and wildcards (10de:*)
func matchesAllowlist(vendor, device string, allowlist []string) bool {
	deviceStr := fmt.Sprintf("%s:%s", vendor, device)

	for _, pattern := range allowlist {
		pattern = strings.TrimSpace(pattern)

		// Wildcard match: "10de:*" matches all NVIDIA devices
		if strings.HasSuffix(pattern, ":*") {
			vendorPrefix := strings.TrimSuffix(pattern, ":*")
			if strings.EqualFold(vendor, vendorPrefix) {
				return true
			}
		} else if strings.EqualFold(deviceStr, pattern) {
			// Exact match
			return true
		}
	}

	return false
}

// getCurrentDriver returns the current driver bound to a device
func (m *vfioManager) getCurrentDriver(pciAddr string) (string, error) {
	driverPath := filepath.Join(pciDevicesPath, pciAddr, "driver")

	// Check if driver symlink exists
	if !m.fileSystem.Exists(driverPath) {
		return "", nil // No driver bound
	}

	// Read symlink to get driver name
	target, err := m.fileSystem.Readlink(driverPath)
	if err != nil {
		return "", err
	}

	// Extract driver name from path (e.g., ../../../bus/pci/drivers/nvidia -> nvidia)
	driverName := filepath.Base(target)
	return driverName, nil
}

// getIOMMUGroup returns the IOMMU group ID for a device
func (m *vfioManager) getIOMMUGroup(pciAddr string) (string, error) {
	groupPath := filepath.Join(pciDevicesPath, pciAddr, "iommu_group")

	if !m.fileSystem.Exists(groupPath) {
		return "", nil // No IOMMU group
	}

	target, err := m.fileSystem.Readlink(groupPath)
	if err != nil {
		return "", err
	}

	// Extract group number from path (e.g., ../../../../kernel/iommu_groups/42 -> 42)
	groupID := filepath.Base(target)
	return groupID, nil
}

// getNumaNode returns the NUMA node for a device
func (m *vfioManager) getNumaNode(pciAddr string) string {
	numaPath := filepath.Join(pciDevicesPath, pciAddr, "numa_node")

	data, err := m.fileSystem.ReadFile(numaPath)
	if err != nil {
		return "-1" // Unknown/not applicable
	}

	return strings.TrimSpace(string(data))
}

// unbindDriver unbinds a device from its current driver
func (m *vfioManager) unbindDriver(pciAddr, driver string) error {
	unbindPath := filepath.Join("/sys/bus/pci/drivers", driver, "unbind")
	return m.fileSystem.WriteFile(unbindPath, []byte(pciAddr))
}
