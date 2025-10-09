package orchestrator

import (
	"testing"

	driftroutes "github.com/volantvm/volant/internal/drift/routes"
	"github.com/volantvm/volant/internal/pluginspec"
	"github.com/volantvm/volant/internal/server/db"
	"github.com/volantvm/volant/internal/server/orchestrator/vmconfig"
)

func TestComputeDriftRoutes_DefaultBridged(t *testing.T) {
	eng := &engine{}
	vm := db.VM{Name: "vm-1", IPAddress: "10.0.0.5"}
	exposes := []vmconfig.Expose{{HostPort: 8080, Port: 80, Protocol: ""}}

	computed, err := eng.computeDriftRoutes(vm, nil, exposes)
	if err != nil {
		t.Fatalf("computeDriftRoutes returned error: %v", err)
	}
	if len(computed) != 1 {
		t.Fatalf("expected 1 route, got %d", len(computed))
	}
	route := computed[0]
	if route.HostPort != 8080 {
		t.Fatalf("unexpected host port: %d", route.HostPort)
	}
	if route.Protocol != "tcp" {
		t.Fatalf("expected protocol tcp, got %s", route.Protocol)
	}
	if route.Backend.Type != driftroutes.BackendBridge {
		t.Fatalf("expected backend bridge, got %s", route.Backend.Type)
	}
	if route.Backend.IP != "10.0.0.5" || route.Backend.Port != 80 {
		t.Fatalf("unexpected backend data: %+v", route.Backend)
	}
}

func TestComputeDriftRoutes_VsockRequiresTCP(t *testing.T) {
	eng := &engine{}
	vm := db.VM{Name: "vm-2", VsockCID: 33}
	mode := string(pluginspec.NetworkModeVsock)
	exposes := []vmconfig.Expose{{HostPort: 9000, Port: 9000, Mode: mode, Protocol: "udp"}}

	if _, err := eng.computeDriftRoutes(vm, nil, exposes); err == nil {
		t.Fatalf("expected error for vsock with non-tcp protocol")
	}
}

func TestComputeDriftRoutes_DeduplicatesHostPorts(t *testing.T) {
	eng := &engine{}
	vm := db.VM{Name: "vm-3", IPAddress: "10.0.0.10"}
	netCfg := &pluginspec.NetworkConfig{Mode: pluginspec.NetworkModeBridged}
	exposes := []vmconfig.Expose{
		{HostPort: 8080, Port: 80, Protocol: "tcp"},
		{HostPort: 8080, Port: 80, Protocol: "TCP"},
	}

	computed, err := eng.computeDriftRoutes(vm, netCfg, exposes)
	if err != nil {
		t.Fatalf("computeDriftRoutes returned error: %v", err)
	}
	if len(computed) != 1 {
		t.Fatalf("expected deduplicated routes, got %d", len(computed))
	}
}
