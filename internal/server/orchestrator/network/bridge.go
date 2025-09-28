package network

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
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
	if err := run(ctx, "ip", "link", "show", b.BridgeName); err != nil {
		return fmt.Errorf("bridge %s not present: %w", b.BridgeName, err)
	}
	if err := run(ctx, "ip", "link", "set", b.BridgeName, "up"); err != nil {
		return fmt.Errorf("bring bridge up: %w", err)
	}
	return nil
}

// PrepareTap creates a tap device, attaches it to the bridge, and brings it up.
func (b *BridgeManager) PrepareTap(ctx context.Context, vmName, mac string) (string, error) {
	tap := tapNameFrom(vmName)

	if err := b.ensureBridge(ctx); err != nil {
		return "", err
	}

	// ip tuntap add dev <tap> mode tap
	if err := run(ctx, "ip", "tuntap", "add", "dev", tap, "mode", "tap"); err != nil {
		if !strings.Contains(err.Error(), "File exists") {
			return "", fmt.Errorf("create tap %s: %w", tap, err)
		}
		// Tap already exists; reset it
		_ = run(ctx, "ip", "link", "set", "dev", tap, "down")
		_ = run(ctx, "ip", "link", "set", "dev", tap, "nomaster")
	}

	// ip link set dev <tap> address <mac>
	if err := run(ctx, "ip", "link", "set", "dev", tap, "address", mac); err != nil {
		_ = run(ctx, "ip", "link", "del", tap)
		return "", fmt.Errorf("set tap mac: %w", err)
	}

	// ip link set dev <tap> master <bridge>
	if err := run(ctx, "ip", "link", "set", "dev", tap, "master", b.BridgeName); err != nil {
		_ = run(ctx, "ip", "link", "del", tap)
		return "", fmt.Errorf("attach tap to bridge: %w", err)
	}

	// ip link set dev <tap> up
	if err := run(ctx, "ip", "link", "set", "dev", tap, "up"); err != nil {
		_ = run(ctx, "ip", "link", "del", tap)
		return "", fmt.Errorf("bring tap up: %w", err)
	}

	return tap, nil
}

// CleanupTap detaches and deletes the tap device.
func (b *BridgeManager) CleanupTap(ctx context.Context, tap string) error {
	// ip link set dev <tap> down
	if err := run(ctx, "ip", "link", "set", "dev", tap, "down"); err != nil {
		return fmt.Errorf("tap down: %w", err)
	}
	if err := run(ctx, "ip", "link", "del", tap); err != nil {
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

	// Ensure we don't exceed the kernel's interface name limit.
	maxSuffixLen := maxInterfaceNameLen - len(tapPrefix)
	if maxSuffixLen < 1 {
		maxSuffixLen = 1
	}
	if len(sanitized) > maxSuffixLen {
		sanitized = sanitized[:maxSuffixLen]
	}

	return tapPrefix + sanitized
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

func run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %s: %v - %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}
