package config

import (
	"fmt"
	"net"
	"os"
)

const (
    defaultDBPath        = "~/.volant/state.db"
    defaultAPIListenAddr = "127.0.0.1:7777"
    defaultBridgeName    = "vbr0"
    defaultSubnetCIDR    = "192.168.127.0/24"
    defaultHostIP        = "192.168.127.1"
    defaultRuntimeDir    = "~/.volant/run"
    defaultLogDir        = "~/.volant/logs"
)

// ServerConfig captures the runtime configuration required by the daemon.
type ServerConfig struct {
	DatabasePath     string
	APIListenAddr    string
	BridgeName       string
	SubnetCIDR       string
    KernelImagePath  string
    InitramfsPath    string
    HypervisorBinary string
    HostIP           string
    RuntimeDir       string
    LogDir           string
}

// FromEnv loads server configuration from environment variables, applying
// opinionated defaults when unset.
func FromEnv() (ServerConfig, error) {
	cfg := ServerConfig{
		DatabasePath:     getenv("  VOLANT_DB_PATH", defaultDBPath),
		APIListenAddr:    getenv("VOLANT_API_LISTEN", defaultAPIListenAddr),
		BridgeName:       getenv("VOLANT_BRIDGE", defaultBridgeName),
        SubnetCIDR:       getenv("VOLANT_SUBNET", defaultSubnetCIDR),
        HostIP:           getenv("VOLANT_HOST_IP", defaultHostIP),
        KernelImagePath:  os.Getenv("VOLANT_KERNEL"),
        InitramfsPath:    os.Getenv("VOLANT_INITRAMFS"),
        HypervisorBinary: getenv("VOLANT_HYPERVISOR", "cloud-hypervisor"),
        RuntimeDir:       getenv("VOLANT_RUNTIME_DIR", defaultRuntimeDir),
        LogDir:           getenv("VOLANT_LOG_DIR", defaultLogDir),
    }

	if _, _, err := net.ParseCIDR(cfg.SubnetCIDR); err != nil {
		return ServerConfig{}, fmt.Errorf("invalid subnet cidr %q: %w", cfg.SubnetCIDR, err)
	}

    if cfg.KernelImagePath == "" || cfg.InitramfsPath == "" {
        return ServerConfig{}, fmt.Errorf("kernel/initramfs paths must be provided via environment")
    }

    if net.ParseIP(cfg.HostIP) == nil {
        return ServerConfig{}, fmt.Errorf("invalid host ip %q", cfg.HostIP)
    }

    return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
