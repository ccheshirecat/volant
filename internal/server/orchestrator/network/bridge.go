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

// PrepareTap creates a tap device, attaches it to the bridge, and brings it up.
func (b *BridgeManager) PrepareTap(ctx context.Context, vmName, mac string) (string, error) {
	tap := tapNameFrom(vmName)

	// ip tuntap add dev <tap> mode tap
	if err := run(ctx, "ip", "tuntap", "add", "dev", tap, "mode", "tap"); err != nil {
		return "", fmt.Errorf("create tap %s: %w", tap, err)
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

func tapNameFrom(vmName string) string {
	sanitized := sanitize(vmName)
	if len(sanitized) > 8 {
		sanitized = sanitized[:8]
	}
	if sanitized == "" {
		sanitized = "vm"
	}
	return fmt.Sprintf("vipertap-%s", sanitized)
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
