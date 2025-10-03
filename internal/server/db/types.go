// Copyright (c) 2025 HYPR. PTE. LTD.
//
// Business Source License 1.1
// See LICENSE file in the project root for details.

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
	VsockCID      uint32 // Vsock Context ID for vsock communication
	CPUCores      int
	MemoryMB      int
	KernelCmdline string
	SerialSocket  string
	GroupID       *int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// VMGroup represents a deployment/group of VMs managed together.
type VMGroup struct {
	ID         int64
	Name       string
	ConfigJSON []byte
	Replicas   int
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type PluginArtifact struct {
	ID           int64
	PluginName   string
	Version      string
	ArtifactName string
	Kind         string
	SourceURL    string
	Checksum     string
	Format       string
	LocalPath    string
	SizeBytes    int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type VMCloudInit struct {
	VMID          int64
	UserData      string
	MetaData      string
	NetworkConfig string
	SeedPath      string
	UpdatedAt     time.Time
}

// VMConfig captures the serialized configuration stored for a VM.
type VMConfig struct {
	VMID       int64
	Version    int
	ConfigJSON []byte
	UpdatedAt  time.Time
}

// VMConfigHistoryEntry represents a historical version of a VM configuration.
type VMConfigHistoryEntry struct {
	ID         int64
	VMID       int64
	Version    int
	ConfigJSON []byte
	UpdatedAt  time.Time
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
	VMConfigs() VMConfigRepository
	VMGroups() VMGroupRepository
	PluginArtifacts() PluginArtifactRepository
	VMCloudInit() VMCloudInitRepository
}

// VMRepository manages CRUD and lifecycle updates for VMs.
type VMRepository interface {
	Create(ctx context.Context, vm *VM) (int64, error)
	GetByName(ctx context.Context, name string) (*VM, error)
	List(ctx context.Context) ([]VM, error)
	ListByGroupID(ctx context.Context, groupID int64) ([]VM, error)
	UpdateRuntimeState(ctx context.Context, id int64, status VMStatus, pid *int64) error
	UpdateKernelCmdline(ctx context.Context, id int64, cmdline string) error
	UpdateSockets(ctx context.Context, id int64, serial string) error
	UpdateSpec(ctx context.Context, id int64, runtime string, cpuCores, memoryMB int, kernelCmdline string) error
	Delete(ctx context.Context, id int64) error
}

// VMConfigRepository manages serialized VM configuration payloads.
type VMConfigRepository interface {
	GetCurrent(ctx context.Context, vmID int64) (*VMConfig, error)
	Upsert(ctx context.Context, vmID int64, payload []byte) (*VMConfig, error)
	History(ctx context.Context, vmID int64, limit int) ([]VMConfigHistoryEntry, error)
}

// VMGroupRepository manages VM deployment groups.
type VMGroupRepository interface {
	Create(ctx context.Context, group *VMGroup) (int64, error)
	Update(ctx context.Context, id int64, configJSON []byte, replicas int) error
	UpdateReplicas(ctx context.Context, id int64, replicas int) error
	Delete(ctx context.Context, id int64) error
	GetByName(ctx context.Context, name string) (*VMGroup, error)
	GetByID(ctx context.Context, id int64) (*VMGroup, error)
	List(ctx context.Context) ([]VMGroup, error)
}

type PluginArtifactRepository interface {
	Upsert(ctx context.Context, artifact PluginArtifact) error
	ListByPlugin(ctx context.Context, plugin string) ([]PluginArtifact, error)
	ListByPluginVersion(ctx context.Context, plugin, version string) ([]PluginArtifact, error)
	Get(ctx context.Context, plugin, version, artifactName string) (*PluginArtifact, error)
	DeleteByPluginVersion(ctx context.Context, plugin, version string) error
	DeleteByPlugin(ctx context.Context, plugin string) error
}

type VMCloudInitRepository interface {
	Upsert(ctx context.Context, record VMCloudInit) error
	Get(ctx context.Context, vmID int64) (*VMCloudInit, error)
	Delete(ctx context.Context, vmID int64) error
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
