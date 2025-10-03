// Copyright (c) 2025 HYPR. PTE. LTD.
//
// Business Source License 1.1
// See LICENSE file in the project root for details.

package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"database/sql"

	"github.com/volantvm/volant/internal/server/db"
)

func TestVMRepositoryCRUD(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	t.Cleanup(func() { _ = store.Close(ctx) })

	vmRepo := store.Queries().VirtualMachines()
	ipRepo := store.Queries().IPAllocations()

	if err := ipRepo.EnsurePool(ctx, []string{"192.168.127.2", "192.168.127.3"}); err != nil {
		t.Fatalf("ensure pool: %v", err)
	}

	vm := &db.VM{
		Name:       "vm-1",
		Status:     db.VMStatusPending,
		Runtime:    "browser",
		IPAddress:  "192.168.127.2",
		MACAddress: "02:00:00:00:00:01",
		CPUCores:   2,
		MemoryMB:   2048,
	}

	id, err := vmRepo.Create(ctx, vm)
	if err != nil {
		t.Fatalf("create vm: %v", err)
	}

	fetched, err := vmRepo.GetByName(ctx, "vm-1")
	if err != nil {
		t.Fatalf("get vm: %v", err)
	}
	if fetched == nil {
		t.Fatalf("expected vm, got nil")
	}
	if fetched.ID != id || fetched.Status != db.VMStatusPending {
		t.Fatalf("unexpected vm fetched: %+v", fetched)
	}
	if fetched.CreatedAt.IsZero() || fetched.UpdatedAt.IsZero() {
		t.Fatalf("timestamps not populated: %+v", fetched)
	}

	pid := int64(4321)
	if err := vmRepo.UpdateRuntimeState(ctx, id, db.VMStatusRunning, &pid); err != nil {
		t.Fatalf("update runtime: %v", err)
	}

	updated, err := vmRepo.GetByName(ctx, "vm-1")
	if err != nil {
		t.Fatalf("get updated vm: %v", err)
	}
	if updated.Status != db.VMStatusRunning {
		t.Fatalf("status not updated: %v", updated.Status)
	}
	if updated.PID == nil || *updated.PID != pid {
		t.Fatalf("pid not updated: %+v", updated.PID)
	}

	if err := vmRepo.UpdateKernelCmdline(ctx, id, "ip=192.168.127.2"); err != nil {
		t.Fatalf("update cmdline: %v", err)
	}

	cmdlineCheck, err := vmRepo.GetByName(ctx, "vm-1")
	if err != nil {
		t.Fatalf("get vm after cmdline: %v", err)
	}
	if cmdlineCheck.KernelCmdline != "ip=192.168.127.2" {
		t.Fatalf("cmdline not updated: %s", cmdlineCheck.KernelCmdline)
	}

	if err := vmRepo.Delete(ctx, id); err != nil {
		t.Fatalf("delete vm: %v", err)
	}

	removed, err := vmRepo.GetByName(ctx, "vm-1")
	if err != nil {
		t.Fatalf("get removed vm: %v", err)
	}
	if removed != nil {
		t.Fatalf("expected nil after delete, got %+v", removed)
	}
}

func TestIPRepositoryLeaseAndAssign(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	t.Cleanup(func() { _ = store.Close(ctx) })

	ipRepo := store.Queries().IPAllocations()
	if err := ipRepo.EnsurePool(ctx, []string{"192.168.127.10", "192.168.127.11"}); err != nil {
		t.Fatalf("ensure pool: %v", err)
	}

	var leasedIP string
	err := store.WithTx(ctx, func(q db.Queries) error {
		allocation, err := q.IPAllocations().LeaseNextAvailable(ctx)
		if err != nil {
			return err
		}
		leasedIP = allocation.IPAddress

		vmRepo := q.VirtualMachines()
		vm := &db.VM{
			Name:       "vm-lease",
			Status:     db.VMStatusPending,
			Runtime:    "browser",
			IPAddress:  allocation.IPAddress,
			MACAddress: "02:00:00:00:00:aa",
			CPUCores:   2,
			MemoryMB:   1024,
		}
		id, err := vmRepo.Create(ctx, vm)
		if err != nil {
			return err
		}
		return q.IPAllocations().Assign(ctx, allocation.IPAddress, id)
	})
	if err != nil {
		t.Fatalf("transaction: %v", err)
	}

	lookup, err := ipRepo.Lookup(ctx, leasedIP)
	if err != nil {
		t.Fatalf("lookup ip: %v", err)
	}
	if lookup == nil || lookup.Status != db.IPStatusLeased || lookup.VMID == nil {
		t.Fatalf("unexpected allocation after assign: %+v", lookup)
	}

	if err := ipRepo.Release(ctx, leasedIP); err != nil {
		t.Fatalf("release ip: %v", err)
	}

	postRelease, err := ipRepo.Lookup(ctx, leasedIP)
	if err != nil {
		t.Fatalf("lookup post release: %v", err)
	}
	if postRelease == nil || postRelease.Status != db.IPStatusAvailable || postRelease.VMID != nil {
		t.Fatalf("release did not reset allocation: %+v", postRelease)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	return store
}

func TestEnsurePoolIdempotent(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	t.Cleanup(func() { _ = store.Close(ctx) })

	ipRepo := store.Queries().IPAllocations()

	ips := []string{"192.168.127.50", "192.168.127.51"}
	for i := 0; i < 3; i++ {
		if err := ipRepo.EnsurePool(ctx, ips); err != nil {
			t.Fatalf("ensure pool iteration %d: %v", i, err)
		}
	}

	for _, ip := range ips {
		alloc, err := ipRepo.Lookup(ctx, ip)
		if err != nil {
			t.Fatalf("lookup %s: %v", ip, err)
		}
		if alloc == nil || alloc.Status != db.IPStatusAvailable {
			t.Fatalf("ip %s not available after ensure: %+v", ip, alloc)
		}
	}
}

func TestLeaseSpecificUnavailable(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	t.Cleanup(func() { _ = store.Close(ctx) })

	ipRepo := store.Queries().IPAllocations()
	if err := ipRepo.EnsurePool(ctx, []string{"192.168.127.80"}); err != nil {
		t.Fatalf("ensure pool: %v", err)
	}

	if _, err := ipRepo.LeaseSpecific(ctx, "192.168.127.80"); err != nil {
		t.Fatalf("lease specific first attempt: %v", err)
	}
	if _, err := ipRepo.LeaseSpecific(ctx, "192.168.127.80"); err != db.ErrNoAvailableIPs {
		t.Fatalf("expected ErrNoAvailableIPs on second lease, got %v", err)
	}
}

func TestLeaseNextAvailableEmptyPool(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	t.Cleanup(func() { _ = store.Close(ctx) })

	_, err := store.Queries().IPAllocations().LeaseNextAvailable(ctx)
	if err != db.ErrNoAvailableIPs {
		t.Fatalf("expected ErrNoAvailableIPs, got %v", err)
	}
}

func TestTimestampCoercionHandlesRFC3339(t *testing.T) {
	ts, err := coerceTime("2025-09-23T12:34:56Z")
	if err != nil {
		t.Fatalf("coerceTime: %v", err)
	}
	if ts.UTC().Format(time.RFC3339) != "2025-09-23T12:34:56Z" {
		t.Fatalf("unexpected coerced time: %s", ts)
	}
}

func TestPluginRepository(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	t.Cleanup(func() { _ = store.Close(ctx) })

	repo := store.Queries().Plugins()

	plugin := db.Plugin{
		Name:     "browser",
		Version:  "1.0.0",
		Enabled:  true,
		Metadata: []byte(`{"image":"browser-runtime"}`),
	}
	if err := repo.Upsert(ctx, plugin); err != nil {
		t.Fatalf("upsert plugin: %v", err)
	}

	stored, err := repo.GetByName(ctx, "browser")
	if err != nil {
		t.Fatalf("get plugin: %v", err)
	}
	if stored == nil {
		t.Fatalf("expected plugin, got nil")
	}
	if stored.Version != "1.0.0" || !stored.Enabled {
		t.Fatalf("unexpected stored plugin: %+v", stored)
	}
	if string(stored.Metadata) != string(plugin.Metadata) {
		t.Fatalf("metadata mismatch: %q", stored.Metadata)
	}

	time.Sleep(10 * time.Millisecond)
	if err := repo.Upsert(ctx, db.Plugin{Name: "browser", Version: "1.1.0", Enabled: false}); err != nil {
		t.Fatalf("update plugin: %v", err)
	}

	updated, err := repo.GetByName(ctx, "browser")
	if err != nil {
		t.Fatalf("get updated plugin: %v", err)
	}
	if updated == nil || updated.Version != "1.1.0" || updated.Enabled {
		t.Fatalf("plugin not updated: %+v", updated)
	}
	if !updated.UpdatedAt.After(updated.InstalledAt) && !updated.UpdatedAt.Equal(updated.InstalledAt) {
		t.Fatalf("timestamps not updated: install=%v updated=%v", updated.InstalledAt, updated.UpdatedAt)
	}

	plugins, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list plugins: %v", err)
	}
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}

	if err := repo.SetEnabled(ctx, "browser", true); err != nil {
		t.Fatalf("set enabled: %v", err)
	}
	enabled, err := repo.GetByName(ctx, "browser")
	if err != nil {
		t.Fatalf("get after enable: %v", err)
	}
	if enabled == nil || !enabled.Enabled {
		t.Fatalf("expected plugin enabled, got %+v", enabled)
	}

	if err := repo.Delete(ctx, "browser"); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("delete plugin: %v", err)
		}
	}

	missing, err := repo.GetByName(ctx, "browser")
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if missing != nil {
		t.Fatalf("expected nil after delete, got %+v", missing)
	}
}
