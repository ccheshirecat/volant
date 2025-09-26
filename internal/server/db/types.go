package db

import (
	"context"
	"errors"
	"time"
)

// VMStatus enumerates the lifecycle phases tracked for microVMs.
type VMStatus string

const (
	VMStatusPending  VMStatus = "pending"
	VMStatusStarting VMStatus = "starting"
	VMStatusRunning  VMStatus = "running"
	VMStatusStopped  VMStatus = "stopped"
	VMStatusCrashed  VMStatus = "crashed"
)

// VM models the database representation of a managed microVM.
type VM struct {
	ID            int64
	Name          string
	Status        VMStatus
	Runtime       string
	PID           *int64
	IPAddress     string
	MACAddress    string
	CPUCores      int
	MemoryMB      int
	KernelCmdline string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// IPStatus communicates whether an address is available or leased.
type IPStatus string

const (
	IPStatusAvailable IPStatus = "available"
	IPStatusLeased    IPStatus = "leased"
)

// IPAllocation captures pool state for deterministic static IP assignment.
type IPAllocation struct {
	IPAddress string
	VMID      *int64
	Status    IPStatus
	LeasedAt  *time.Time
}

// ErrNoAvailableIPs is returned when the allocator cannot find a free address.
var ErrNoAvailableIPs = errors.New("db: no available ip addresses")

// Store describes the persistence surface consumed by the orchestrator.
type Store interface {
	Close(ctx context.Context) error
	Queries() Queries
	WithTx(ctx context.Context, fn func(Queries) error) error
}

type Plugin struct {
	ID          int64
	Name        string
	Version     string
	Enabled     bool
	Metadata    []byte
	InstalledAt time.Time
	UpdatedAt   time.Time
}

type PluginRepository interface {
	Upsert(ctx context.Context, plugin Plugin) error
	List(ctx context.Context) ([]Plugin, error)
	GetByName(ctx context.Context, name string) (*Plugin, error)
	SetEnabled(ctx context.Context, name string, enabled bool) error
	Delete(ctx context.Context, name string) error
}

// Queries exposes repository accessors bound to a specific connection scope
// (either the root connection or a transaction).
type Queries interface {
	VirtualMachines() VMRepository
	IPAllocations() IPRepository
	Plugins() PluginRepository
}

// VMRepository manages CRUD and lifecycle updates for VMs.
type VMRepository interface {
	Create(ctx context.Context, vm *VM) (int64, error)
	GetByName(ctx context.Context, name string) (*VM, error)
	List(ctx context.Context) ([]VM, error)
	UpdateRuntimeState(ctx context.Context, id int64, status VMStatus, pid *int64) error
	UpdateKernelCmdline(ctx context.Context, id int64, cmdline string) error
	Delete(ctx context.Context, id int64) error
}

// IPRepository manages deterministic IP allocation.
type IPRepository interface {
	EnsurePool(ctx context.Context, ips []string) error
	LeaseNextAvailable(ctx context.Context) (*IPAllocation, error)
	LeaseSpecific(ctx context.Context, ip string) (*IPAllocation, error)
	Assign(ctx context.Context, ip string, vmID int64) error
	Release(ctx context.Context, ip string) error
	Lookup(ctx context.Context, ip string) (*IPAllocation, error)
}
