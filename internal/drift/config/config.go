package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultHTTPListen    = "0.0.0.0:9090"
	defaultMetricsListen = "127.0.0.1:9091"
	defaultBridgeName    = "vbr0"
	defaultStateDir      = "~/.volant/drift"
	defaultBPFObject     = "drift_l4.bpf.o"
)

// Config captures runtime settings for the Drift control daemon.
type Config struct {
	HTTPListen    string
	MetricsListen string
	BridgeName    string
	StateDir      string
	RoutesPath    string
	BPFObjectPath string
	APIKey        string
}

// FromEnv loads configuration using environment variables with defaults.
func FromEnv() (Config, error) {
	cfg := Config{
		HTTPListen:    getenv("DRIFT_HTTP_LISTEN", defaultHTTPListen),
		MetricsListen: getenv("DRIFT_METRICS_LISTEN", defaultMetricsListen),
		BridgeName:    getenv("DRIFT_BRIDGE", defaultBridgeName),
		StateDir:      expandPath(getenv("DRIFT_STATE_DIR", defaultStateDir)),
		RoutesPath:    expandPath(getenv("DRIFT_ROUTES_PATH", "")),
		BPFObjectPath: expandPath(getenv("DRIFT_BPF_OBJECT", defaultBPFObject)),
		APIKey:        strings.TrimSpace(os.Getenv("DRIFT_API_KEY")),
	}

	if cfg.HTTPListen = strings.TrimSpace(cfg.HTTPListen); cfg.HTTPListen == "" {
		return Config{}, fmt.Errorf("http listen address required")
	}

	if cfg.MetricsListen = strings.TrimSpace(cfg.MetricsListen); cfg.MetricsListen == "" {
		cfg.MetricsListen = cfg.HTTPListen
	}

	if cfg.StateDir == "" {
		return Config{}, fmt.Errorf("state directory required")
	}

	if cfg.RoutesPath == "" {
		cfg.RoutesPath = filepath.Join(cfg.StateDir, "routes.json")
	}

	if cfg.BPFObjectPath == "" {
		return Config{}, fmt.Errorf("bpf object path required")
	}

	cfg.BPFObjectPath = resolveBPFObjectPath(cfg.BPFObjectPath, cfg.StateDir)

	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
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

func resolveBPFObjectPath(path, stateDir string) string {
	cleaned := filepath.Clean(path)
	if filepath.IsAbs(cleaned) {
		return cleaned
	}

	if exe, err := os.Executable(); err == nil {
		base := filepath.Dir(exe)
		candidate := filepath.Join(base, cleaned)
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate
		}
	}

	if stateDir != "" {
		return filepath.Join(stateDir, cleaned)
	}

	return cleaned
}
