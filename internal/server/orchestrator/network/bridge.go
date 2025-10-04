// Copyright (c) 2025 HYPR. PTE. LTD.
//
// Business Source License 1.1
// See LICENSE file in the project root for details.

package network

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"strings"

	"github.com/vishvananda/netlink"
)

// BridgeManager provisions tap devices and attaches them to a Linux bridge.
type BridgeManager struct {
	BridgeName string
}

// NewBridgeManager constructs a bridge-backed network manager.
func NewBridgeManager(bridge string) *BridgeManager {
	return &BridgeManager{BridgeName: bridge}
}

// ensureBridge ensures the bridge device exists and is up.
func (b *BridgeManager) ensureBridge(ctx context.Context) error {
	// Get bridge link by name
	link, err := netlink.LinkByName(b.BridgeName)
	if err != nil {
		return fmt.Errorf("bridge %s not present: %w", b.BridgeName, err)
	}

	// Bring bridge up if not already
	if link.Attrs().Flags&net.FlagUp == 0 {
		if err := netlink.LinkSetUp(link); err != nil {
			return fmt.Errorf("bring bridge up: %w", err)
		}
	}

	return nil
}

// PrepareTap creates a tap device, attaches it to the bridge, and brings it up.
func (b *BridgeManager) PrepareTap(ctx context.Context, vmName, mac string) (string, error) {
	tap := tapNameFrom(vmName)

	if err := b.ensureBridge(ctx); err != nil {
		return "", err
	}

	// Parse MAC address
	hwAddr, err := net.ParseMAC(mac)
	if err != nil {
		return "", fmt.Errorf("invalid mac address %s: %w", mac, err)
	}

	// Check if tap already exists
	existingLink, err := netlink.LinkByName(tap)
	if err == nil {
		// Tap exists, reset it
		_ = netlink.LinkSetDown(existingLink)
		_ = netlink.LinkSetNoMaster(existingLink)
		_ = netlink.LinkDel(existingLink)
	}

	// Create tap device
	la := netlink.NewLinkAttrs()
	la.Name = tap
	la.HardwareAddr = hwAddr
	tuntap := &netlink.Tuntap{
		LinkAttrs: la,
		Mode:      netlink.TUNTAP_MODE_TAP,
		Flags:     netlink.TUNTAP_DEFAULTS | netlink.TUNTAP_VNET_HDR,
	}

	if err := netlink.LinkAdd(tuntap); err != nil {
		return "", fmt.Errorf("create tap %s: %w", tap, err)
	}

	// Get bridge link
	bridge, err := netlink.LinkByName(b.BridgeName)
	if err != nil {
		_ = netlink.LinkDel(tuntap)
		return "", fmt.Errorf("get bridge link: %w", err)
	}

	// Attach tap to bridge
	if err := netlink.LinkSetMaster(tuntap, bridge); err != nil {
		_ = netlink.LinkDel(tuntap)
		return "", fmt.Errorf("attach tap to bridge: %w", err)
	}

	// Bring tap up
	if err := netlink.LinkSetUp(tuntap); err != nil {
		_ = netlink.LinkDel(tuntap)
		return "", fmt.Errorf("bring tap up: %w", err)
	}

	return tap, nil
}

// CleanupTap detaches and deletes the tap device.
func (b *BridgeManager) CleanupTap(ctx context.Context, tap string) error {
	link, err := netlink.LinkByName(tap)
	if err != nil {
		// Already gone, consider it cleaned up
		return nil
	}

	// Bring tap down
	if err := netlink.LinkSetDown(link); err != nil {
		return fmt.Errorf("tap down: %w", err)
	}

	// Delete tap
	if err := netlink.LinkDel(link); err != nil {
		return fmt.Errorf("delete tap: %w", err)
	}

	return nil
}

const (
	maxInterfaceNameLen = 15 // Linux IFNAMSIZ (16) minus null terminator
	tapPrefix           = "vttap-"
)

func tapNameFrom(vmName string) string {
	sanitized := sanitize(vmName)
	if sanitized == "" {
		sanitized = "vm"
	}

	// Calculate available space: 15 chars total - "vttap-" prefix = 9 chars
	maxSuffixLen := maxInterfaceNameLen - len(tapPrefix)
	if maxSuffixLen < 1 {
		maxSuffixLen = 1
	}

	// If name fits, use it directly
	if len(sanitized) <= maxSuffixLen {
		return tapPrefix + sanitized
	}

	// Otherwise, use prefix + hash to ensure uniqueness
	// Reserve 6 chars for hash, use remaining space for readable prefix
	hashLen := 6
	prefixLen := maxSuffixLen - hashLen
	if prefixLen < 1 {
		prefixLen = 1
	}

	// Generate hash from full VM name
	hash := sha256.Sum256([]byte(vmName))
	hashStr := hex.EncodeToString(hash[:])[:hashLen]

	// Use readable prefix + hash: e.g., "vttap-web3a4b5c"
	prefix := sanitized
	if len(prefix) > prefixLen {
		prefix = prefix[:prefixLen]
	}

	return tapPrefix + prefix + hashStr
}

func sanitize(input string) string {
	var b strings.Builder
	for _, r := range input {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if r >= 'A' && r <= 'Z' {
			b.WriteRune(r + 32)
		}
	}
	return b.String()
}
