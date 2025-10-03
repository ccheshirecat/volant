package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultDBPath        = "~/.volant/state.db"
	defaultAPIPort       = "7777"
	defaultAPIListenAddr = "0.0.0.0:" + defaultAPIPort
	defaultBridgeName    = "vbr0"
	defaultSubnetCIDR    = "192.168.127.0/24"
	defaultHostIP        = "192.168.127.1"
	defaultRuntimeDir    = "~/.volant/run"
	defaultLogDir        = "~/.volant/logs"
	defaultBZImagePath   = "/var/lib/volant/kernel/bzImage"
	defaultVMLinuxPath   = "/var/lib/volant/kernel/vmlinux"
)

// ServerConfig captures the runtime configuration required by the daemon.
type ServerConfig struct {
	DatabasePath     string
	APIListenAddr    string
	APIAdvertiseAddr string
	BridgeName       string
	SubnetCIDR       string
	BZImagePath      string
	VMLinuxPath      string
	HypervisorBinary string
	HostIP           string
	RuntimeDir       string
	LogDir           string
}

// FromEnv loads server configuration from environment variables, applying
// opinionated defaults when unset.
func FromEnv() (ServerConfig, error) {
	cfg := ServerConfig{
		DatabasePath:     getenv("VOLANT_DB_PATH", defaultDBPath),
		APIListenAddr:    getenv("VOLANT_API_LISTEN", defaultAPIListenAddr),
		APIAdvertiseAddr: getenv("VOLANT_API_ADVERTISE", ""),
		BridgeName:       getenv("VOLANT_BRIDGE", defaultBridgeName),
		SubnetCIDR:       getenv("VOLANT_SUBNET", defaultSubnetCIDR),
		HostIP:           getenv("VOLANT_HOST_IP", defaultHostIP),
		HypervisorBinary: getenv("VOLANT_HYPERVISOR", "cloud-hypervisor"),
		RuntimeDir:       getenv("VOLANT_RUNTIME_DIR", defaultRuntimeDir),
		LogDir:           getenv("VOLANT_LOG_DIR", defaultLogDir),
	}

	// New dual-kernel config
	bz := strings.TrimSpace(os.Getenv("VOLANT_KERNEL_BZIMAGE"))
	vm := strings.TrimSpace(os.Getenv("VOLANT_KERNEL_VMLINUX"))
	if bz == "" {
		bz = defaultBZImagePath
	}
	if vm == "" {
		vm = defaultVMLinuxPath
	}
	bz = expandPath(bz)
	vm = expandPath(vm)
	// Only require at least one to exist
	bzExists := fileExists(bz)
	vmExists := fileExists(vm)
	if !bzExists && !vmExists {
		return ServerConfig{}, fmt.Errorf("no kernel images found: expected bzImage at %s or vmlinux at %s", bz, vm)
	}
	cfg.BZImagePath = bz
	cfg.VMLinuxPath = vm

	if _, _, err := net.ParseCIDR(cfg.SubnetCIDR); err != nil {
		return ServerConfig{}, fmt.Errorf("invalid subnet cidr %q: %w", cfg.SubnetCIDR, err)
	}

	if net.ParseIP(cfg.HostIP) == nil {
		return ServerConfig{}, fmt.Errorf("invalid host ip %q", cfg.HostIP)
	}

	listenAddr := strings.TrimSpace(cfg.APIListenAddr)
	if listenAddr == "" {
		return ServerConfig{}, fmt.Errorf("api listen address required")
	}
	listenHost, listenPort, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return ServerConfig{}, fmt.Errorf("invalid api listen address %q: %w", listenAddr, err)
	}
	if strings.TrimSpace(listenPort) == "" {
		listenPort = defaultAPIPort
	}
	if strings.TrimSpace(cfg.APIAdvertiseAddr) == "" {
		advHost := cfg.HostIP
		trimmedHost := strings.TrimSpace(listenHost)
		if isRoutableAdvertiseHost(trimmedHost) {
			advHost = trimmedHost
		}
		cfg.APIAdvertiseAddr = net.JoinHostPort(advHost, listenPort)
	}

	return cfg, nil
}

func isRoutableAdvertiseHost(host string) bool {
	if host == "" {
		return false
	}
	lower := strings.ToLower(host)
	switch lower {
	case "localhost", "0.0.0.0", "::", "[::]":
		return false
	}
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = strings.Trim(host, "[]")
	}
	if ip := net.ParseIP(host); ip != nil {
		return !(ip.IsLoopback() || ip.IsUnspecified())
	}
	return true
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func expandPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}
	return filepath.Clean(path)
}

func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	if _, err := os.Stat(path); err == nil {
		return true
	}
	return false
}
