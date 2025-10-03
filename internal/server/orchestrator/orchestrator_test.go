// Copyright (c) 2025 HYPR. PTE. LTD.
//
// Business Source License 1.1
// See LICENSE file in the project root for details.

package orchestrator

import (
	"context"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"sync"
	"testing"

	"github.com/volantvm/volant/internal/pluginspec"
	"github.com/volantvm/volant/internal/server/db/sqlite"
	"github.com/volantvm/volant/internal/server/orchestrator/network"
	"github.com/volantvm/volant/internal/server/orchestrator/runtime"
	"github.com/volantvm/volant/internal/server/orchestrator/vmconfig"
)

func TestEngineCreateAndDestroyVM(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer func() { _ = store.Close(ctx) }()

	subnet, host := testSubnet(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	fakeLauncher := &testLauncher{}
	fakeNetwork := &testNetworkManager{}

	engine, err := New(Params{
		Store:            store,
		Logger:           logger,
		Subnet:           subnet,
		HostIP:           host,
		APIListenAddr:    "127.0.0.1:7777",
		APIAdvertiseAddr: "127.0.0.1:7777",
		RuntimeDir:       t.TempDir(),
		Launcher:         fakeLauncher,
		Network:          fakeNetwork,
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	if err := engine.Start(ctx); err != nil {
		t.Fatalf("engine start: %v", err)
	}

	vm, err := engine.CreateVM(ctx, CreateVMRequest{
		Name:     "vm-test-1",
		Plugin:   "browser",
		Runtime:  "browser",
		CPUCores: 2,
		MemoryMB: 2048,
		Manifest: &pluginspec.Manifest{Name: "browser", Runtime: "browser"},
	})
	if err != nil {
		t.Fatalf("create vm: %v", err)
	}
	if vm == nil {
		t.Fatalf("expected vm, got nil")
	}
	if vm.IPAddress == "" {
		t.Fatalf("vm ip not assigned")
	}
	if vm.Status != "running" {
		t.Fatalf("expected running status, got %s", vm.Status)
	}
	if vm.PID == nil {
		t.Fatalf("expected pid to be set")
	}

	if len(fakeLauncher.calls) != 1 {
		t.Fatalf("launcher not invoked")
	}

	vms, err := engine.ListVMs(ctx)
	if err != nil {
		t.Fatalf("list vms: %v", err)
	}
	if len(vms) != 1 {
		t.Fatalf("expected 1 vm, got %d", len(vms))
	}

	if err := engine.DestroyVM(ctx, "vm-test-1"); err != nil {
		t.Fatalf("destroy vm: %v", err)
	}

	gone, err := engine.GetVM(ctx, "vm-test-1")
	if err != nil {
		t.Fatalf("get destroyed vm: %v", err)
	}
	if gone != nil {
		t.Fatalf("expected nil after destroy, got %+v", gone)
	}

	if !fakeNetwork.cleaned {
		t.Fatalf("expected network cleanup")
	}
}

func TestDeploymentScaling(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	defer func() { _ = store.Close(ctx) }()

	subnet, host := testSubnet(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	fakeLauncher := &testLauncher{}
	fakeNetwork := &testNetworkManager{}

	engine, err := New(Params{
		Store:            store,
		Logger:           logger,
		Subnet:           subnet,
		HostIP:           host,
		APIListenAddr:    "127.0.0.1:7777",
		APIAdvertiseAddr: "127.0.0.1:7777",
		RuntimeDir:       t.TempDir(),
		Launcher:         fakeLauncher,
		Network:          fakeNetwork,
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}

	if err := engine.Start(ctx); err != nil {
		t.Fatalf("engine start: %v", err)
	}

	config := vmconfig.Config{
		Plugin:  "browser",
		Runtime: "browser",
		Resources: vmconfig.Resources{
			CPUCores: 1,
			MemoryMB: 512,
		},
		Manifest: &pluginspec.Manifest{Name: "browser", Runtime: "browser"},
	}

	deployment, err := engine.CreateDeployment(ctx, CreateDeploymentRequest{
		Name:     "demo",
		Replicas: 2,
		Config:   config,
	})
	if err != nil {
		t.Fatalf("create deployment: %v", err)
	}
	if deployment.DesiredReplicas != 2 {
		t.Fatalf("expected desired replicas 2, got %d", deployment.DesiredReplicas)
	}
	if len(fakeLauncher.calls) != 2 {
		t.Fatalf("expected 2 launch calls, got %d", len(fakeLauncher.calls))
	}

	deployment, err = engine.ScaleDeployment(ctx, "demo", 3)
	if err != nil {
		t.Fatalf("scale deployment up: %v", err)
	}
	if deployment.DesiredReplicas != 3 {
		t.Fatalf("expected desired replicas 3, got %d", deployment.DesiredReplicas)
	}

	deployment, err = engine.ScaleDeployment(ctx, "demo", 1)
	if err != nil {
		t.Fatalf("scale deployment down: %v", err)
	}
	if deployment.DesiredReplicas != 1 {
		t.Fatalf("expected desired replicas 1, got %d", deployment.DesiredReplicas)
	}

	if err := engine.DeleteDeployment(ctx, "demo"); err != nil {
		t.Fatalf("delete deployment: %v", err)
	}

	if _, err := engine.GetDeployment(ctx, "demo"); err == nil {
		t.Fatalf("expected error fetching deleted deployment")
	}
}

func openTestStore(t *testing.T) *sqlite.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := sqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	return store
}

func testSubnet(t *testing.T) (*net.IPNet, net.IP) {
	t.Helper()
	_, subnet, err := net.ParseCIDR("192.168.127.0/24")
	if err != nil {
		t.Fatalf("parse subnet: %v", err)
	}
	host := net.ParseIP("192.168.127.1")
	if host == nil {
		t.Fatalf("parse host ip failed")
	}
	return subnet, host
}

// testLauncher implements runtime.Launcher for unit tests.
type testLauncher struct {
	mu    sync.Mutex
	pid   int
	calls []runtime.LaunchSpec
}

func (t *testLauncher) Launch(ctx context.Context, spec runtime.LaunchSpec) (runtime.Instance, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pid++
	t.calls = append(t.calls, spec)
	inst := &testInstance{
		name: spec.Name,
		pid:  t.pid,
		done: make(chan error, 1),
	}
	return inst, nil
}

type testInstance struct {
	name string
	pid  int
	done chan error
	once sync.Once
}

func (i *testInstance) Name() string          { return i.name }
func (i *testInstance) PID() int              { return i.pid }
func (i *testInstance) APISocketPath() string { return "" }
func (i *testInstance) Wait() <-chan error    { return i.done }
func (i *testInstance) Stop(ctx context.Context) error {
	i.once.Do(func() {
		i.done <- nil
		close(i.done)
	})
	return nil
}

// testNetworkManager provides deterministic tap handling for tests.
type testNetworkManager struct {
	cleaned bool
}

func (n *testNetworkManager) PrepareTap(ctx context.Context, vmName, mac string) (string, error) {
	return "tap-test", nil
}

func (n *testNetworkManager) CleanupTap(ctx context.Context, tapName string) error {
	n.cleaned = true
	return nil
}

var _ runtime.Launcher = (*testLauncher)(nil)
var _ runtime.Instance = (*testInstance)(nil)
var _ network.Manager = (*testNetworkManager)(nil)
