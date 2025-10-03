// Copyright (c) 2025 HYPR. PTE. LTD.
//
// Business Source License 1.1
// See LICENSE file in the project root for details.

package orchestrator

import (
	"testing"

	"github.com/volantvm/volant/internal/pluginspec"
	"github.com/volantvm/volant/internal/server/orchestrator/vmconfig"
)

func TestResolveNetworkConfig(t *testing.T) {
	tests := []struct {
		name     string
		manifest *pluginspec.Manifest
		vmConfig *vmconfig.Config
		want     *pluginspec.NetworkConfig
	}{
		{
			name:     "both nil returns nil",
			manifest: nil,
			vmConfig: nil,
			want:     nil,
		},
		{
			name: "manifest only",
			manifest: &pluginspec.Manifest{
				Network: &pluginspec.NetworkConfig{
					Mode: pluginspec.NetworkModeVsock,
				},
			},
			vmConfig: nil,
			want: &pluginspec.NetworkConfig{
				Mode: pluginspec.NetworkModeVsock,
			},
		},
		{
			name:     "vm config only",
			manifest: nil,
			vmConfig: &vmconfig.Config{
				Network: &pluginspec.NetworkConfig{
					Mode: pluginspec.NetworkModeBridged,
				},
			},
			want: &pluginspec.NetworkConfig{
				Mode: pluginspec.NetworkModeBridged,
			},
		},
		{
			name: "vm config overrides manifest",
			manifest: &pluginspec.Manifest{
				Network: &pluginspec.NetworkConfig{
					Mode: pluginspec.NetworkModeVsock,
				},
			},
			vmConfig: &vmconfig.Config{
				Network: &pluginspec.NetworkConfig{
					Mode: pluginspec.NetworkModeDHCP,
				},
			},
			want: &pluginspec.NetworkConfig{
				Mode: pluginspec.NetworkModeDHCP,
			},
		},
		{
			name: "vm config without network falls back to manifest",
			manifest: &pluginspec.Manifest{
				Network: &pluginspec.NetworkConfig{
					Mode: pluginspec.NetworkModeVsock,
				},
			},
			vmConfig: &vmconfig.Config{
				Plugin: "test",
			},
			want: &pluginspec.NetworkConfig{
				Mode: pluginspec.NetworkModeVsock,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveNetworkConfig(tt.manifest, tt.vmConfig)
			if tt.want == nil && got != nil {
				t.Errorf("resolveNetworkConfig() = %v, want nil", got)
				return
			}
			if tt.want != nil && got == nil {
				t.Errorf("resolveNetworkConfig() = nil, want %v", tt.want)
				return
			}
			if tt.want != nil && got != nil && got.Mode != tt.want.Mode {
				t.Errorf("resolveNetworkConfig() mode = %v, want %v", got.Mode, tt.want.Mode)
			}
		})
	}
}

func TestNeedsIPAllocation(t *testing.T) {
	tests := []struct {
		name   string
		netCfg *pluginspec.NetworkConfig
		want   bool
	}{
		{
			name:   "nil config needs IP (default)",
			netCfg: nil,
			want:   true,
		},
		{
			name: "vsock mode does not need IP",
			netCfg: &pluginspec.NetworkConfig{
				Mode: pluginspec.NetworkModeVsock,
			},
			want: false,
		},
		{
			name: "bridged mode needs IP",
			netCfg: &pluginspec.NetworkConfig{
				Mode: pluginspec.NetworkModeBridged,
			},
			want: true,
		},
		{
			name: "dhcp mode does not need host-managed IP",
			netCfg: &pluginspec.NetworkConfig{
				Mode: pluginspec.NetworkModeDHCP,
			},
			want: false,
		},
		{
			name: "empty mode defaults to needing IP",
			netCfg: &pluginspec.NetworkConfig{
				Mode: "",
			},
			want: true,
		},
		{
			name: "uppercase vsock mode",
			netCfg: &pluginspec.NetworkConfig{
				Mode: "VSOCK",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := needsIPAllocation(tt.netCfg); got != tt.want {
				t.Errorf("needsIPAllocation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNeedsTapDevice(t *testing.T) {
	tests := []struct {
		name   string
		netCfg *pluginspec.NetworkConfig
		want   bool
	}{
		{
			name:   "nil config needs tap (default)",
			netCfg: nil,
			want:   true,
		},
		{
			name: "vsock mode does not need tap",
			netCfg: &pluginspec.NetworkConfig{
				Mode: pluginspec.NetworkModeVsock,
			},
			want: false,
		},
		{
			name: "bridged mode needs tap",
			netCfg: &pluginspec.NetworkConfig{
				Mode: pluginspec.NetworkModeBridged,
			},
			want: true,
		},
		{
			name: "dhcp mode needs tap",
			netCfg: &pluginspec.NetworkConfig{
				Mode: pluginspec.NetworkModeDHCP,
			},
			want: true,
		},
		{
			name: "empty mode defaults to needing tap",
			netCfg: &pluginspec.NetworkConfig{
				Mode: "",
			},
			want: true,
		},
		{
			name: "uppercase vsock mode",
			netCfg: &pluginspec.NetworkConfig{
				Mode: "VSOCK",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := needsTapDevice(tt.netCfg); got != tt.want {
				t.Errorf("needsTapDevice() = %v, want %v", got, tt.want)
			}
		})
	}
}
