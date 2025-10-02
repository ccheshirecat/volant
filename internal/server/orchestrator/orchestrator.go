package orchestrator

import (
	"context"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ccheshirecat/volant/internal/pluginspec"
	"github.com/ccheshirecat/volant/internal/server/db"
	"github.com/ccheshirecat/volant/internal/server/eventbus"
	"github.com/ccheshirecat/volant/internal/server/orchestrator/cloudinit"
	orchestratorevents "github.com/ccheshirecat/volant/internal/server/orchestrator/events"
	"github.com/ccheshirecat/volant/internal/server/orchestrator/network"
	"github.com/ccheshirecat/volant/internal/server/orchestrator/runtime"
	"github.com/ccheshirecat/volant/internal/server/orchestrator/vmconfig"
)

// Engine represents the VM orchestration core.
type Engine interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error

	CreateVM(ctx context.Context, req CreateVMRequest) (*db.VM, error)
	DestroyVM(ctx context.Context, name string) error
	ListVMs(ctx context.Context) ([]db.VM, error)
	GetVM(ctx context.Context, name string) (*db.VM, error)
	GetVMConfig(ctx context.Context, name string) (*vmconfig.Versioned, error)
	UpdateVMConfig(ctx context.Context, name string, patch vmconfig.Patch) (*vmconfig.Versioned, error)
	GetVMConfigHistory(ctx context.Context, name string, limit int) ([]vmconfig.HistoryEntry, error)
	StartVM(ctx context.Context, name string) (*db.VM, error)
	StopVM(ctx context.Context, name string) (*db.VM, error)
	RestartVM(ctx context.Context, name string) (*db.VM, error)
	CreateDeployment(ctx context.Context, req CreateDeploymentRequest) (*Deployment, error)
	ListDeployments(ctx context.Context) ([]Deployment, error)
	GetDeployment(ctx context.Context, name string) (*Deployment, error)
	ScaleDeployment(ctx context.Context, name string, replicas int) (*Deployment, error)
	DeleteDeployment(ctx context.Context, name string) error
	Store() db.Store
	ControlPlaneListenAddr() string
	ControlPlaneAdvertiseAddr() string
	HostIP() net.IP
}

// CreateVMRequest captures the inputs required to instantiate a VM lifecycle.
type CreateVMRequest struct {
	Name              string
	Plugin            string
	Runtime           string
	CPUCores          int
	MemoryMB          int
	KernelCmdlineHint string
	Manifest          *pluginspec.Manifest
	APIHost           string
	APIPort           string
	Config            *vmconfig.Config
	GroupID           *int64
}

// Deployment represents a managed group of VM replicas.
type Deployment struct {
	Name            string
	DesiredReplicas int
	ReadyReplicas   int
	Config          vmconfig.Config
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// CreateDeploymentRequest defines the inputs required to create a deployment.
type CreateDeploymentRequest struct {
	Name     string
	Replicas int
	Config   vmconfig.Config
}

// Params wires dependencies for the native orchestrator engine.
type Params struct {
	Store            db.Store
	Logger           *slog.Logger
	Subnet           *net.IPNet
	HostIP           net.IP
	APIListenAddr    string
	APIAdvertiseAddr string
	RuntimeDir       string
	Launcher         runtime.Launcher
	Network          network.Manager
	Bus              eventbus.Bus
}

// New constructs the production orchestrator engine.
func New(params Params) (Engine, error) {
	if params.Store == nil {
		return nil, fmt.Errorf("orchestrator: store is required")
	}
	if params.Logger == nil {
		return nil, fmt.Errorf("orchestrator: logger is required")
	}
	if params.Subnet == nil {
		return nil, fmt.Errorf("orchestrator: subnet is required")
	}
	if params.HostIP == nil {
		return nil, fmt.Errorf("orchestrator: host IP is required")
	}
	listenAddr := strings.TrimSpace(params.APIListenAddr)
	advertiseAddr := strings.TrimSpace(params.APIAdvertiseAddr)
	if listenAddr == "" {
		return nil, fmt.Errorf("orchestrator: API listen address is required")
	}
	if advertiseAddr == "" {
		advertiseAddr = listenAddr
	}
	_, advertisePort, err := net.SplitHostPort(advertiseAddr)
	if err != nil {
		return nil, fmt.Errorf("orchestrator: parse api advertise addr: %w", err)
	}
	if params.Launcher == nil {
		return nil, fmt.Errorf("orchestrator: launcher is required")
	}
	if params.Network == nil {
		params.Network = network.NewNoop()
	}
	if !params.Subnet.Contains(params.HostIP) {
		return nil, fmt.Errorf("orchestrator: host IP %s not in subnet %s", params.HostIP, params.Subnet)
	}

	pool, err := deriveIPPool(params.Subnet, params.HostIP)
	if err != nil {
		return nil, fmt.Errorf("orchestrator: derive ip pool: %w", err)
	}

	runtimeDir := strings.TrimSpace(params.RuntimeDir)
	if runtimeDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("orchestrator: determine user home: %w", err)
		}
		runtimeDir = filepath.Join(home, ".volant", "run")
	}
	switch {
	case runtimeDir == "~":
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("orchestrator: expand runtime dir: %w", err)
		}
		runtimeDir = home
	case strings.HasPrefix(runtimeDir, "~/"):
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("orchestrator: expand runtime dir: %w", err)
		}
		runtimeDir = filepath.Join(home, runtimeDir[2:])
	}
	runtimeDir = filepath.Clean(runtimeDir)
	if !filepath.IsAbs(runtimeDir) {
		absRuntime, err := filepath.Abs(runtimeDir)
		if err != nil {
			return nil, fmt.Errorf("orchestrator: resolve runtime dir: %w", err)
		}
		runtimeDir = absRuntime
	}

	return &engine{
		store:                params.Store,
		logger:               params.Logger.With("component", "orchestrator"),
		subnet:               params.Subnet,
		hostIP:               params.HostIP,
		controlListenAddr:    listenAddr,
		controlAdvertiseAddr: advertiseAddr,
		controlPort:          advertisePort,
		ipPool:               pool,
		runtimeDir:           runtimeDir,
		launcher:             params.Launcher,
		network:              params.Network,
		bus:                  params.Bus,
		instances:            make(map[string]processHandle),
	}, nil
}

type engine struct {
	store                db.Store
	logger               *slog.Logger
	subnet               *net.IPNet
	hostIP               net.IP
	controlListenAddr    string
	controlAdvertiseAddr string
	controlPort          string
	ipPool               []string
	runtimeDir           string
	launcher             runtime.Launcher
	network              network.Manager
	bus                  eventbus.Bus

	mu         sync.Mutex
	instances  map[string]processHandle
	procCtx    context.Context
	procCancel context.CancelFunc
}

type processHandle struct {
	instance runtime.Instance
	tapName  string
	serial   string
	seedPath string
}

var (
	// ErrVMExists indicates a VM with the same name already exists.
	ErrVMExists = errors.New("orchestrator: vm already exists")
	// ErrVMNotFound indicates the requested VM does not exist.
	ErrVMNotFound = errors.New("orchestrator: vm not found")
	// ErrDeploymentExists indicates a deployment with the same name already exists.
	ErrDeploymentExists = errors.New("orchestrator: deployment already exists")
	// ErrDeploymentNotFound indicates the requested deployment does not exist.
	ErrDeploymentNotFound = errors.New("orchestrator: deployment not found")
)

func (e *engine) Start(ctx context.Context) error {
	if err := e.store.WithTx(ctx, func(q db.Queries) error {
		return q.IPAllocations().EnsurePool(ctx, e.ipPool)
	}); err != nil {
		return err
	}

	parent := context.Background()
	if ctx != nil {
		parent = ctx
	}
	procCtx, cancel := context.WithCancel(parent)

	e.mu.Lock()
	if e.procCancel != nil {
		e.procCancel()
	}
	e.procCtx = procCtx
	e.procCancel = cancel
	e.mu.Unlock()

	return nil
}

func (e *engine) Stop(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	var errs []error
	for name, handle := range e.instances {
		if err := handle.instance.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("stop %s: %w", name, err))
		}
		if err := e.network.CleanupTap(ctx, handle.tapName); err != nil {
			errs = append(errs, fmt.Errorf("cleanup tap %s: %w", handle.tapName, err))
		}
		delete(e.instances, name)
	}

	if e.procCancel != nil {
		e.procCancel()
		e.procCancel = nil
		e.procCtx = nil
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (e *engine) CreateVM(ctx context.Context, req CreateVMRequest) (*db.VM, error) {
	if err := validateCreateRequest(req); err != nil {
		return nil, err
	}

	var manifestRuntime string
	pluginName := ""
	if req.Manifest != nil {
		req.Manifest.Normalize()
		manifestRuntime = strings.TrimSpace(req.Manifest.Runtime)
		pluginName = strings.TrimSpace(req.Manifest.Name)
	}

	req.Runtime = strings.TrimSpace(req.Runtime)
	if req.Runtime == "" {
		req.Runtime = manifestRuntime
	}
	if req.Runtime == "" {
		req.Runtime = pluginName
	}
	if req.Runtime == "" {
		return nil, fmt.Errorf("orchestrator: runtime required")
	}
	if manifestRuntime != "" && req.Runtime != manifestRuntime {
		return nil, fmt.Errorf("orchestrator: runtime mismatch between request (%s) and manifest (%s)", req.Runtime, manifestRuntime)
	}

	netmask := formatNetmask(e.subnet.Mask)
	hostname := sanitizeHostname(req.Name)

	var (
		insertedID int64
		vmRecord   *db.VM
	)

	// Resolve effective network configuration
	networkCfg := resolveNetworkConfig(req.Manifest, req.Config)

	err := e.store.WithTx(ctx, func(q db.Queries) error {
		vmRepo := q.VirtualMachines()
		existing, err := vmRepo.GetByName(ctx, req.Name)
		if err != nil {
			return err
		}
		if existing != nil {
			return fmt.Errorf("%w: %s", ErrVMExists, req.Name)
		}

		// Conditionally allocate IP based on network mode
		var ipAddress string
		if needsIPAllocation(networkCfg) {
			allocation, err := q.IPAllocations().LeaseNextAvailable(ctx)
			if err != nil {
				return err
			}
			ipAddress = allocation.IPAddress
		} else {
			// vsock or dhcp mode: no host-managed IP
			ipAddress = ""
		}

		// Allocate unique vsock CID for this VM
		// CIDs 0-2 are reserved (0=hypervisor, 1=local, 2=host)
		// Start from 3 and find next available
		vsockCID, err := e.allocateNextCID(ctx, vmRepo)
		if err != nil {
			return fmt.Errorf("allocate vsock cid: %w", err)
		}

		mac := deriveMAC(req.Name, ipAddress)
		baseCmdline := buildKernelCmdline(ipAddress, e.hostIP.String(), netmask, hostname, req.KernelCmdlineHint)
		fullCmdline := appendKernelArgs(baseCmdline, map[string]string{})

		vm := &db.VM{
			Name:          req.Name,
			Status:        db.VMStatusStarting,
			Runtime:       req.Runtime,
			IPAddress:     ipAddress,
			MACAddress:    mac,
			VsockCID:      vsockCID,
			CPUCores:      req.CPUCores,
			MemoryMB:      req.MemoryMB,
			KernelCmdline: fullCmdline,
			GroupID:       req.GroupID,
		}

		id, err := vmRepo.Create(ctx, vm)
		if err != nil {
			return err
		}
		if ipAddress != "" {
			if err := q.IPAllocations().Assign(ctx, ipAddress, id); err != nil {
				return err
			}
		}
		insertedID = id
		vm.ID = id
		vmRecord = vm
		return nil
	})
	if err != nil {
		return nil, err
	}

	e.publishEvent(ctx, orchestratorevents.TypeVMCreated, orchestratorevents.VMStatusStarting, vmRecord, "vm record created")

	apiHost := strings.TrimSpace(req.APIHost)
	apiPort := strings.TrimSpace(req.APIPort)
	if apiPort == "0" {
		apiPort = ""
	}
	if apiHost == "" || apiPort == "" {
		host, port := e.apiEndpoints()
		if apiHost == "" {
			apiHost = host
		}
		if apiPort == "" {
			apiPort = port
		}
	}
	if apiHost == "" {
		apiHost = e.hostIP.String()
	}
	if strings.TrimSpace(apiPort) == "" {
		apiPort = e.controlPort
	}

	var manifestForConfig *pluginspec.Manifest
	if req.Manifest != nil {
		manifestCopy := *req.Manifest
		manifestCopy.Normalize()
		manifestForConfig = &manifestCopy
	}

	additionalDisks := buildAdditionalDisks(req.Manifest)

	configToStore := vmconfig.Config{}
	if req.Config != nil {
		configToStore = req.Config.Clone()
	}
	configToStore.Plugin = pluginName
	configToStore.Runtime = req.Runtime
	extraCmdline := strings.TrimSpace(req.KernelCmdlineHint)
	if extraCmdline == "" && req.Config != nil {
		extraCmdline = strings.TrimSpace(req.Config.KernelCmdline)
	}
	configToStore.KernelCmdline = extraCmdline
	configToStore.Resources = vmconfig.Resources{
		CPUCores: vmRecord.CPUCores,
		MemoryMB: vmRecord.MemoryMB,
	}
	configToStore.API = vmconfig.API{
		Host: apiHost,
		Port: apiPort,
	}
	if configToStore.Manifest == nil && manifestForConfig != nil {
		manifestCopy := *manifestForConfig
		configToStore.Manifest = &manifestCopy
	} else if configToStore.Manifest != nil {
		manifestCopy := *configToStore.Manifest
		manifestCopy.Normalize()
		configToStore.Manifest = &manifestCopy
	}

	var seedDisk *runtime.Disk
	var cloudInitRecord *db.VMCloudInit
	overrideCloudInit := (*pluginspec.CloudInit)(nil)
	if configToStore.CloudInit != nil {
		overrideCopy := *configToStore.CloudInit
		overrideCopy.Normalize()
		overrideCloudInit = &overrideCopy
	}
	effectiveCloudInit, record, preparedSeedDisk, err := e.prepareCloudInitSeed(ctx, vmRecord, manifestForConfig, overrideCloudInit)
	if err != nil {
		e.rollbackCreate(ctx, vmRecord)
		return nil, err
	}
	configToStore.CloudInit = effectiveCloudInit
	seedDisk = preparedSeedDisk
	cloudInitRecord = record

	configPayload, err := vmconfig.Marshal(configToStore)
	if err != nil {
		if seedDisk != nil {
			_ = os.Remove(seedDisk.Path)
		}
		e.rollbackCreate(ctx, vmRecord)
		return nil, err
	}

	if err := e.store.WithTx(ctx, func(q db.Queries) error {
		if _, upsertErr := q.VMConfigs().Upsert(ctx, vmRecord.ID, configPayload); upsertErr != nil {
			return upsertErr
		}
		if cloudInitRecord != nil {
			record := *cloudInitRecord
			record.VMID = vmRecord.ID
			if err := q.VMCloudInit().Upsert(ctx, record); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		if seedDisk != nil {
			_ = os.Remove(seedDisk.Path)
		}
		e.rollbackCreate(ctx, vmRecord)
		return nil, err
	}

	// Conditionally prepare tap device based on network mode
	tapName := ""
	if needsTapDevice(networkCfg) {
		tap, err := e.network.PrepareTap(ctx, vmRecord.Name, vmRecord.MACAddress)
		if err != nil {
			e.rollbackCreate(ctx, vmRecord)
			return nil, err
		}
		tapName = tap
	}

	serialPath := filepath.Join(e.runtimeDir, fmt.Sprintf("%s.serial", vmRecord.Name))
	serialPath = filepath.Clean(serialPath)
	if !filepath.IsAbs(serialPath) {
		absSerial, absErr := filepath.Abs(serialPath)
		if absErr != nil {
			e.rollbackCreate(ctx, vmRecord)
			return nil, fmt.Errorf("orchestrator: resolve serial socket path: %w", absErr)
		}
		serialPath = absSerial
	}

	spec := runtime.LaunchSpec{
		Name:          vmRecord.Name,
		CPUCores:      vmRecord.CPUCores,
		MemoryMB:      vmRecord.MemoryMB,
		KernelCmdline: vmRecord.KernelCmdline,
		TapDevice:     tapName,
		MACAddress:    vmRecord.MACAddress,
		IPAddress:     vmRecord.IPAddress,
		Gateway:       e.hostIP.String(),
		Netmask:       netmask,
		VsockCID:      vmRecord.VsockCID,
		SerialSocket:  serialPath,
	}
	spec.Disks = additionalDisks
	if seedDisk != nil {
		spec.SeedDisk = seedDisk
	}

	cmdArgs := map[string]string{
		pluginspec.RuntimeKey:      req.Runtime,
		pluginspec.APIHostKey:      apiHost,
		pluginspec.APIPortKey:      apiPort,
		pluginspec.RootFSDeviceKey: "vda",
		pluginspec.RootFSFSTypeKey: "ext4",
	}
	if pluginName != "" {
		cmdArgs[pluginspec.PluginKey] = pluginName
	}
	if req.Manifest != nil {
		encodedManifest, err := pluginspec.Encode(*req.Manifest)
		if err != nil {
			e.logger.Error("encode manifest", "vm", req.Name, "error", err)
			return nil, fmt.Errorf("orchestrator: encode manifest: %w", err)
		}
		cmdArgs[pluginspec.CmdlineKey] = encodedManifest
	}
	spec.Args = cmdArgs

	if req.Manifest != nil {
		spec.RootFS = strings.TrimSpace(req.Manifest.RootFS.URL)
		spec.RootFSChecksum = strings.TrimSpace(req.Manifest.RootFS.Checksum)
	}
	e.logger.Info("launch kernel cmdline", "vm", req.Name, "cmdline", spec.KernelCmdline)

	launchCtx := e.launchContext()

	instance, err := e.launcher.Launch(launchCtx, spec)
	if err != nil {
		if seedDisk != nil {
			_ = os.Remove(seedDisk.Path)
		}
		_ = e.network.CleanupTap(ctx, tapName)
		e.rollbackCreate(ctx, vmRecord)
		return nil, err
	}
	vmRecord.SerialSocket = spec.SerialSocket

	pid := int64(instance.PID())
	if err := e.store.WithTx(ctx, func(q db.Queries) error {
		repo := q.VirtualMachines()
		if err := repo.UpdateRuntimeState(ctx, insertedID, db.VMStatusRunning, &pid); err != nil {
			return err
		}
		return repo.UpdateSockets(ctx, insertedID, spec.SerialSocket)
	}); err != nil {
		_ = instance.Stop(ctx)
		_ = e.network.CleanupTap(ctx, tapName)
		if seedDisk != nil {
			_ = os.Remove(seedDisk.Path)
		}
		return nil, err
	}

	e.mu.Lock()
	seedPath := ""
	if seedDisk != nil {
		seedPath = seedDisk.Path
	}
	handle := processHandle{instance: instance, tapName: tapName, serial: spec.SerialSocket, seedPath: seedPath}
	e.instances[vmRecord.Name] = handle
	e.mu.Unlock()

	e.monitorInstance(vmRecord.Name, handle)

	vmRecord.Status = db.VMStatusRunning
	vmRecord.PID = &pid
	e.publishEvent(ctx, orchestratorevents.TypeVMRunning, orchestratorevents.VMStatusRunning, vmRecord, "vm running")
	return vmRecord, nil
}

func (e *engine) DestroyVM(ctx context.Context, name string) error {
	_, err := e.destroyVM(ctx, name, true)
	return err
}

func (e *engine) destroyVM(ctx context.Context, name string, reconcile bool) (*db.VM, error) {
	var (
		vmRecord    *db.VM
		cloudRecord *db.VMCloudInit
	)
	err := e.store.WithTx(ctx, func(q db.Queries) error {
		vmRepo := q.VirtualMachines()
		vm, err := vmRepo.GetByName(ctx, name)
		if err != nil {
			return err
		}
		if vm == nil {
			return fmt.Errorf("%w: %s", ErrVMNotFound, name)
		}
		vmRecord = vm
		if record, err := q.VMCloudInit().Get(ctx, vm.ID); err == nil {
			cloudRecord = record
		} else if err != nil {
			return err
		}
		if err := vmRepo.Delete(ctx, vm.ID); err != nil {
			return err
		}
		if err := q.IPAllocations().Release(ctx, vm.IPAddress); err != nil {
			return err
		}
		if err := q.VMCloudInit().Delete(ctx, vm.ID); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	e.mu.Lock()
	handle, exists := e.instances[name]
	if exists {
		delete(e.instances, name)
	}
	e.mu.Unlock()

	if exists {
		if err := handle.instance.Stop(ctx); err != nil {
			e.logger.Error("stop instance", "vm", name, "error", err)
		}
		// Only cleanup tap if one was created
		if handle.tapName != "" {
			if err := e.network.CleanupTap(ctx, handle.tapName); err != nil {
				e.logger.Error("cleanup tap", "tap", handle.tapName, "error", err)
			}
		}
		if socket := handle.instance.APISocketPath(); socket != "" {
			if removeErr := os.Remove(socket); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				e.logger.Debug("remove api socket", "path", socket, "error", removeErr)
			}
		}
	}

	seedPath := ""
	if cloudRecord != nil && strings.TrimSpace(cloudRecord.SeedPath) != "" {
		seedPath = strings.TrimSpace(cloudRecord.SeedPath)
	}
	if exists && handle.seedPath != "" {
		seedPath = handle.seedPath
	}
	if seedPath != "" {
		if err := os.Remove(seedPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			e.logger.Debug("remove seed image", "path", seedPath, "error", err)
		}
	}

	if vmRecord != nil {
		vmRecord.Status = db.VMStatusStopped
		vmRecord.PID = nil
	}

	e.publishEvent(ctx, orchestratorevents.TypeVMDeleted, orchestratorevents.VMStatusStopped, vmRecord, "vm deleted")

	if reconcile && vmRecord != nil && vmRecord.GroupID != nil {
		if _, recErr := e.reconcileDeploymentByID(ctx, *vmRecord.GroupID); recErr != nil {
			e.logger.Error("reconcile deployment after vm delete", "vm", name, "error", recErr)
		}
	}

	return vmRecord, nil
}

func (e *engine) ListVMs(ctx context.Context) ([]db.VM, error) {
	return e.store.Queries().VirtualMachines().List(ctx)
}

func (e *engine) GetVM(ctx context.Context, name string) (*db.VM, error) {
	return e.store.Queries().VirtualMachines().GetByName(ctx, name)
}

func (e *engine) GetVMConfig(ctx context.Context, name string) (*vmconfig.Versioned, error) {
	queries := e.store.Queries()
	vm, err := queries.VirtualMachines().GetByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if vm == nil {
		return nil, fmt.Errorf("%w: %s", ErrVMNotFound, name)
	}
	record, err := queries.VMConfigs().GetCurrent(ctx, vm.ID)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, fmt.Errorf("orchestrator: configuration for vm %s not found", name)
	}
	versioned, err := vmconfig.FromDB(*record)
	if err != nil {
		return nil, err
	}
	return &versioned, nil
}

func (e *engine) GetVMConfigHistory(ctx context.Context, name string, limit int) ([]vmconfig.HistoryEntry, error) {
	queries := e.store.Queries()
	vm, err := queries.VirtualMachines().GetByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if vm == nil {
		return nil, fmt.Errorf("%w: %s", ErrVMNotFound, name)
	}
	rows, err := queries.VMConfigs().History(ctx, vm.ID, limit)
	if err != nil {
		return nil, err
	}
	result := make([]vmconfig.HistoryEntry, 0, len(rows))
	for _, entry := range rows {
		history, convErr := vmconfig.FromHistory(entry)
		if convErr != nil {
			return nil, convErr
		}
		result = append(result, history)
	}
	return result, nil
}

func (e *engine) UpdateVMConfig(ctx context.Context, name string, patch vmconfig.Patch) (*vmconfig.Versioned, error) {
	var updated vmconfig.Versioned

	err := e.store.WithTx(ctx, func(q db.Queries) error {
		vmRepo := q.VirtualMachines()
		vm, err := vmRepo.GetByName(ctx, name)
		if err != nil {
			return err
		}
		if vm == nil {
			return fmt.Errorf("%w: %s", ErrVMNotFound, name)
		}
		record, err := q.VMConfigs().GetCurrent(ctx, vm.ID)
		if err != nil {
			return err
		}
		if record == nil {
			return fmt.Errorf("orchestrator: configuration for vm %s not found", name)
		}
		current, err := vmconfig.FromDB(*record)
		if err != nil {
			return err
		}
		merged, err := patch.Apply(current.Config)
		if err != nil {
			return err
		}
		extraCmdline := strings.TrimSpace(merged.KernelCmdline)
		finalCmdline := buildKernelCmdline(vm.IPAddress, e.hostIP.String(), formatNetmask(e.subnet.Mask), sanitizeHostname(vm.Name), extraCmdline)
		merged.KernelCmdline = extraCmdline
		payload, err := vmconfig.Marshal(merged)
		if err != nil {
			return err
		}
		if err := vmRepo.UpdateSpec(ctx, vm.ID, merged.Runtime, merged.Resources.CPUCores, merged.Resources.MemoryMB, finalCmdline); err != nil {
			return err
		}
		vm.KernelCmdline = finalCmdline
		newRecord, err := q.VMConfigs().Upsert(ctx, vm.ID, payload)
		if err != nil {
			return err
		}
		versioned, err := vmconfig.FromDB(*newRecord)
		if err != nil {
			return err
		}
		updated = versioned
		vm.Runtime = merged.Runtime
		vm.CPUCores = merged.Resources.CPUCores
		vm.MemoryMB = merged.Resources.MemoryMB
		vm.KernelCmdline = finalCmdline
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

func (e *engine) StartVM(ctx context.Context, name string) (*db.VM, error) {
	e.mu.Lock()
	if _, exists := e.instances[name]; exists {
		e.mu.Unlock()
		return nil, fmt.Errorf("orchestrator: vm %s already running", name)
	}
	e.mu.Unlock()

	var (
		vmRecord         *db.VM
		cfg              vmconfig.Config
		cloudRecord      *db.VMCloudInit
		cloudInitToStore *db.VMCloudInit
	)

	err := e.store.WithTx(ctx, func(q db.Queries) error {
		vmRepo := q.VirtualMachines()
		vm, err := vmRepo.GetByName(ctx, name)
		if err != nil {
			return err
		}
		if vm == nil {
			return fmt.Errorf("%w: %s", ErrVMNotFound, name)
		}
		record, err := q.VMConfigs().GetCurrent(ctx, vm.ID)
		if err != nil {
			return err
		}
		if record == nil {
			return fmt.Errorf("orchestrator: configuration for vm %s not found", name)
		}
		versioned, err := vmconfig.FromDB(*record)
		if err != nil {
			return err
		}
		cfg = versioned.Config.Clone()
		if existing, err := q.VMCloudInit().Get(ctx, vm.ID); err == nil {
			cloudRecord = existing
		} else if err != nil {
			return err
		}
		if err := vmRepo.UpdateRuntimeState(ctx, vm.ID, db.VMStatusStarting, nil); err != nil {
			return err
		}
		vmRecord = vm
		return nil
	})
	if err != nil {
		return nil, err
	}

	apiHost := strings.TrimSpace(cfg.API.Host)
	apiPort := strings.TrimSpace(cfg.API.Port)
	if apiPort == "0" {
		apiPort = ""
	}
	if apiHost == "" || apiPort == "" {
		host, port := e.apiEndpoints()
		if apiHost == "" {
			apiHost = host
		}
		if apiPort == "" {
			apiPort = port
		}
	}
	if apiHost == "" {
		apiHost = e.hostIP.String()
	}
	if strings.TrimSpace(apiPort) == "" {
		apiPort = e.controlPort
	}

	// Resolve network configuration for this VM
	networkCfg := resolveNetworkConfig(cfg.Manifest, &cfg)

	// Conditionally prepare tap device based on network mode
	tapName := ""
	if needsTapDevice(networkCfg) {
		tap, err := e.network.PrepareTap(ctx, vmRecord.Name, vmRecord.MACAddress)
		if err != nil {
			e.setVMState(ctx, vmRecord.ID, db.VMStatusStopped, nil)
			return nil, err
		}
		tapName = tap
	}

	serialPath := filepath.Join(e.runtimeDir, fmt.Sprintf("%s.serial", vmRecord.Name))
	serialPath = filepath.Clean(serialPath)
	if !filepath.IsAbs(serialPath) {
		absSerial, absErr := filepath.Abs(serialPath)
		if absErr != nil {
			e.setVMState(ctx, vmRecord.ID, db.VMStatusStopped, nil)
			_ = e.network.CleanupTap(ctx, tapName)
			return nil, fmt.Errorf("orchestrator: resolve serial socket path: %w", absErr)
		}
		serialPath = absSerial
	}

	manifest := cfg.Manifest
	if manifest == nil {
		_ = e.network.CleanupTap(ctx, tapName)
		e.setVMState(ctx, vmRecord.ID, db.VMStatusStopped, nil)
		return nil, fmt.Errorf("orchestrator: manifest missing in configuration for vm %s", name)
	}

	additionalDisks := buildAdditionalDisks(manifest)
	overrideCloudInit := cfg.CloudInit
	mergedCloudInit, record, seedDisk, err := e.prepareCloudInitSeed(ctx, vmRecord, manifest, overrideCloudInit)
	if err != nil {
		_ = e.network.CleanupTap(ctx, tapName)
		e.setVMState(ctx, vmRecord.ID, db.VMStatusStopped, nil)
		return nil, err
	}
	cfg.CloudInit = mergedCloudInit
	cloudInitToStore = record

	netmask := formatNetmask(e.subnet.Mask)
	spec := runtime.LaunchSpec{
		Name:          vmRecord.Name,
		CPUCores:      cfg.Resources.CPUCores,
		MemoryMB:      cfg.Resources.MemoryMB,
		KernelCmdline: vmRecord.KernelCmdline,
		TapDevice:     tapName,
		MACAddress:    vmRecord.MACAddress,
		IPAddress:     vmRecord.IPAddress,
		Gateway:       e.hostIP.String(),
		Netmask:       netmask,
		VsockCID:      vmRecord.VsockCID,
		SerialSocket:  serialPath,
	}
	spec.Disks = additionalDisks
	if seedDisk != nil {
		spec.SeedDisk = seedDisk
	}

	cmdArgs := map[string]string{
		pluginspec.RuntimeKey:      cfg.Runtime,
		pluginspec.APIHostKey:      apiHost,
		pluginspec.APIPortKey:      apiPort,
		pluginspec.RootFSDeviceKey: "vda",
		pluginspec.RootFSFSTypeKey: "ext4",
	}
	pluginName := strings.TrimSpace(cfg.Plugin)
	if pluginName != "" {
		cmdArgs[pluginspec.PluginKey] = pluginName
	}
	encodedManifest, err := pluginspec.Encode(*manifest)
	if err != nil {
		_ = e.network.CleanupTap(ctx, tapName)
		e.setVMState(ctx, vmRecord.ID, db.VMStatusStopped, nil)
		return nil, fmt.Errorf("orchestrator: encode manifest: %w", err)
	}
	cmdArgs[pluginspec.CmdlineKey] = encodedManifest
	spec.Args = cmdArgs
	spec.RootFS = strings.TrimSpace(manifest.RootFS.URL)
	spec.RootFSChecksum = strings.TrimSpace(manifest.RootFS.Checksum)

	if cloudInitToStore != nil {
		cloudInitToStore.VMID = vmRecord.ID
		if err := e.store.WithTx(ctx, func(q db.Queries) error {
			return q.VMCloudInit().Upsert(ctx, *cloudInitToStore)
		}); err != nil {
			if seedDisk != nil {
				_ = os.Remove(seedDisk.Path)
			}
			_ = e.network.CleanupTap(ctx, tapName)
			e.setVMState(ctx, vmRecord.ID, db.VMStatusStopped, nil)
			return nil, err
		}
	} else if cloudRecord != nil {
		if err := e.store.WithTx(ctx, func(q db.Queries) error {
			return q.VMCloudInit().Delete(ctx, vmRecord.ID)
		}); err != nil {
			if seedDisk != nil {
				_ = os.Remove(seedDisk.Path)
			}
			_ = e.network.CleanupTap(ctx, tapName)
			e.setVMState(ctx, vmRecord.ID, db.VMStatusStopped, nil)
			return nil, err
		}
		if path := strings.TrimSpace(cloudRecord.SeedPath); path != "" {
			_ = os.Remove(path)
		}
	}

	launchCtx := e.launchContext()
	instance, err := e.launcher.Launch(launchCtx, spec)
	if err != nil {
		if seedDisk != nil {
			_ = os.Remove(seedDisk.Path)
		}
		_ = e.network.CleanupTap(ctx, tapName)
		e.setVMState(ctx, vmRecord.ID, db.VMStatusStopped, nil)
		return nil, err
	}

	pid := int64(instance.PID())
	if err := e.store.WithTx(ctx, func(q db.Queries) error {
		repo := q.VirtualMachines()
		if err := repo.UpdateRuntimeState(ctx, vmRecord.ID, db.VMStatusRunning, &pid); err != nil {
			return err
		}
		return repo.UpdateSockets(ctx, vmRecord.ID, spec.SerialSocket)
	}); err != nil {
		_ = instance.Stop(ctx)
		_ = e.network.CleanupTap(ctx, tapName)
		if seedDisk != nil {
			_ = os.Remove(seedDisk.Path)
		}
		return nil, err
	}

	e.mu.Lock()
	seedPath := ""
	if seedDisk != nil {
		seedPath = seedDisk.Path
	}
	handle := processHandle{instance: instance, tapName: tapName, serial: spec.SerialSocket, seedPath: seedPath}
	e.instances[vmRecord.Name] = handle
	e.mu.Unlock()

	e.monitorInstance(vmRecord.Name, handle)

	vmRecord.Status = db.VMStatusRunning
	vmRecord.PID = &pid
	vmRecord.SerialSocket = spec.SerialSocket
	vmRecord.CPUCores = cfg.Resources.CPUCores
	vmRecord.MemoryMB = cfg.Resources.MemoryMB

	e.publishEvent(ctx, orchestratorevents.TypeVMRunning, orchestratorevents.VMStatusRunning, vmRecord, "vm started")
	return vmRecord, nil
}

func (e *engine) StopVM(ctx context.Context, name string) (*db.VM, error) {
	var (
		handle   processHandle
		exists   bool
		vmRecord *db.VM
	)

	e.mu.Lock()
	if h, ok := e.instances[name]; ok {
		handle = h
		exists = true
		delete(e.instances, name)
	}
	e.mu.Unlock()

	err := e.store.WithTx(ctx, func(q db.Queries) error {
		vmRepo := q.VirtualMachines()
		vm, err := vmRepo.GetByName(ctx, name)
		if err != nil {
			return err
		}
		if vm == nil {
			return fmt.Errorf("%w: %s", ErrVMNotFound, name)
		}
		vmRecord = vm
		return vmRepo.UpdateRuntimeState(ctx, vm.ID, db.VMStatusStopped, nil)
	})
	if err != nil {
		if exists {
			// restore handle for consistency if update failed
			e.mu.Lock()
			e.instances[name] = handle
			e.mu.Unlock()
		}
		return nil, err
	}

	if exists {
		if stopErr := handle.instance.Stop(ctx); stopErr != nil {
			e.logger.Error("stop instance", "vm", "name", "error", stopErr)
		}
		// Only cleanup tap if one was created
		if handle.tapName != "" {
			if cleanupErr := e.network.CleanupTap(ctx, handle.tapName); cleanupErr != nil {
				e.logger.Error("cleanup tap", "tap", handle.tapName, "error", cleanupErr)
			}
		}
		if socket := handle.instance.APISocketPath(); socket != "" {
			if removeErr := os.Remove(socket); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				e.logger.Debug("remove api socket", "path", socket, "error", removeErr)
			}
		}
	}

	if vmRecord != nil {
		vmRecord.Status = db.VMStatusStopped
		vmRecord.PID = nil
	}

	e.publishEvent(ctx, orchestratorevents.TypeVMStopped, orchestratorevents.VMStatusStopped, vmRecord, "vm stopped")
	return vmRecord, nil
}

func (e *engine) RestartVM(ctx context.Context, name string) (*db.VM, error) {
	if _, err := e.StopVM(ctx, name); err != nil {
		return nil, err
	}
	return e.StartVM(ctx, name)
}

func (e *engine) CreateDeployment(ctx context.Context, req CreateDeploymentRequest) (*Deployment, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("orchestrator: deployment name required")
	}
	if req.Replicas < 0 {
		return nil, fmt.Errorf("orchestrator: replicas must be >= 0")
	}

	config, err := normalizeDeploymentConfig(req.Config)
	if err != nil {
		return nil, err
	}
	configPayload, err := vmconfig.Marshal(config)
	if err != nil {
		return nil, err
	}

	var groupID int64
	if err := e.store.WithTx(ctx, func(q db.Queries) error {
		repo := q.VMGroups()
		existing, err := repo.GetByName(ctx, name)
		if err != nil {
			return err
		}
		if existing != nil {
			return fmt.Errorf("%w: %s", ErrDeploymentExists, name)
		}
		group := db.VMGroup{
			Name:       name,
			ConfigJSON: configPayload,
			Replicas:   req.Replicas,
		}
		id, err := repo.Create(ctx, &group)
		if err != nil {
			return err
		}
		groupID = id
		return nil
	}); err != nil {
		return nil, err
	}

	return e.reconcileDeploymentByID(ctx, groupID)
}

func (e *engine) ListDeployments(ctx context.Context) ([]Deployment, error) {
	groups, err := e.store.Queries().VMGroups().List(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]Deployment, 0, len(groups))
	for _, group := range groups {
		deployment, err := e.buildDeployment(ctx, group)
		if err != nil {
			return nil, err
		}
		result = append(result, deployment)
	}
	return result, nil
}

func (e *engine) GetDeployment(ctx context.Context, name string) (*Deployment, error) {
	group, err := e.store.Queries().VMGroups().GetByName(ctx, strings.TrimSpace(name))
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, fmt.Errorf("%w: %s", ErrDeploymentNotFound, name)
	}
	deployment, err := e.buildDeployment(ctx, *group)
	if err != nil {
		return nil, err
	}
	return &deployment, nil
}

func (e *engine) ScaleDeployment(ctx context.Context, name string, replicas int) (*Deployment, error) {
	if replicas < 0 {
		return nil, fmt.Errorf("orchestrator: replicas must be >= 0")
	}

	var groupID int64
	if err := e.store.WithTx(ctx, func(q db.Queries) error {
		repo := q.VMGroups()
		group, err := repo.GetByName(ctx, strings.TrimSpace(name))
		if err != nil {
			return err
		}
		if group == nil {
			return fmt.Errorf("%w: %s", ErrDeploymentNotFound, name)
		}
		if err := repo.UpdateReplicas(ctx, group.ID, replicas); err != nil {
			return err
		}
		groupID = group.ID
		return nil
	}); err != nil {
		return nil, err
	}

	return e.reconcileDeploymentByID(ctx, groupID)
}

func (e *engine) DeleteDeployment(ctx context.Context, name string) error {
	var (
		group   *db.VMGroup
		vmNames []string
	)

	if err := e.store.WithTx(ctx, func(q db.Queries) error {
		repo := q.VMGroups()
		found, err := repo.GetByName(ctx, strings.TrimSpace(name))
		if err != nil {
			return err
		}
		if found == nil {
			return fmt.Errorf("%w: %s", ErrDeploymentNotFound, name)
		}
		group = found
		vms, err := q.VirtualMachines().ListByGroupID(ctx, group.ID)
		if err != nil {
			return err
		}
		for _, vm := range vms {
			vmNames = append(vmNames, vm.Name)
		}
		return nil
	}); err != nil {
		return err
	}

	for _, vmName := range vmNames {
		if _, err := e.destroyVM(ctx, vmName, false); err != nil {
			e.logger.Error("delete deployment vm", "deployment", name, "vm", vmName, "error", err)
		}
	}

	return e.store.WithTx(ctx, func(q db.Queries) error {
		return q.VMGroups().Delete(ctx, group.ID)
	})
}

func (e *engine) Store() db.Store {
	return e.store
}

func (e *engine) ControlPlaneListenAddr() string {
	return e.controlListenAddr
}

func (e *engine) ControlPlaneAdvertiseAddr() string {
	return e.controlAdvertiseAddr
}

func (e *engine) HostIP() net.IP {
	return e.hostIP
}

func (e *engine) rollbackCreate(ctx context.Context, vm *db.VM) {
	if vm == nil {
		return
	}
	if err := e.store.WithTx(ctx, func(q db.Queries) error {
		if err := q.VirtualMachines().Delete(ctx, vm.ID); err != nil {
			return err
		}
		return q.IPAllocations().Release(ctx, vm.IPAddress)
	}); err != nil {
		e.logger.Error("rollback create", "vm", vm.Name, "error", err)
	}
}

func (e *engine) monitorInstance(name string, handle processHandle) {
	go func() {
		waitCh := handle.instance.Wait()
		var exitErr error
		if waitCh != nil {
			if result, ok := <-waitCh; ok {
				exitErr = result
			}
		}

		e.mu.Lock()
		stored, exists := e.instances[name]
		if !exists || stored.instance != handle.instance {
			e.mu.Unlock()
			return
		}
		delete(e.instances, name)
		e.mu.Unlock()

		ctx := context.Background()
		status := db.VMStatusStopped
		if exitErr != nil {
			status = db.VMStatusCrashed
		}

		var vmRecord *db.VM
		if err := e.store.WithTx(ctx, func(q db.Queries) error {
			vm, err := q.VirtualMachines().GetByName(ctx, name)
			if err != nil {
				return err
			}
			if vm == nil {
				return nil
			}
			vmRecord = vm
			return q.VirtualMachines().UpdateRuntimeState(ctx, vm.ID, status, nil)
		}); err != nil {
			e.logger.Error("update vm state", "vm", name, "error", err)
		}

		if err := e.network.CleanupTap(ctx, stored.tapName); err != nil {
			e.logger.Error("cleanup tap", "tap", stored.tapName, "error", err)
		}
		if socket := stored.instance.APISocketPath(); socket != "" {
			if removeErr := os.Remove(socket); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				e.logger.Debug("remove api socket", "path", socket, "error", removeErr)
			}
		}
		if stored.seedPath != "" {
			if err := os.Remove(stored.seedPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				e.logger.Debug("remove seed image", "path", stored.seedPath, "error", err)
			}
		}

		if exitErr != nil {
			e.logger.Warn("vm exited unexpectedly", "vm", name, "error", exitErr)
			if vmRecord != nil {
				vmRecord.Status = db.VMStatusCrashed
				vmRecord.PID = nil
			}
			e.publishEvent(ctx, orchestratorevents.TypeVMCrashed, orchestratorevents.VMStatusCrashed, vmRecord, exitErr.Error())
		} else {

			if vmRecord != nil && vmRecord.GroupID != nil {
				if _, err := e.reconcileDeploymentByID(ctx, *vmRecord.GroupID); err != nil {
					e.logger.Error("reconcile deployment after vm exit", "vm", name, "error", err)
				}
			}
			e.logger.Info("vm exited", "vm", name)
			if vmRecord != nil {
				vmRecord.Status = db.VMStatusStopped
				vmRecord.PID = nil
			}
			e.publishEvent(ctx, orchestratorevents.TypeVMStopped, orchestratorevents.VMStatusStopped, vmRecord, "vm exited cleanly")
		}
	}()
}

func (e *engine) publishEvent(ctx context.Context, typ string, status orchestratorevents.VMStatus, vm *db.VM, message string) {
	if e.bus == nil || vm == nil {
		return
	}

	event := orchestratorevents.VMEvent{
		Type:      typ,
		Name:      vm.Name,
		Status:    status,
		IPAddress: vm.IPAddress,
		MAC:       vm.MACAddress,
		Timestamp: time.Now().UTC(),
		Message:   message,
	}
	if vm.PID != nil {
		pid := *vm.PID
		event.PID = &pid
	}
	if err := e.bus.Publish(ctx, orchestratorevents.TopicVMEvents, event); err != nil {
		e.logger.Error("publish vm event", "type", typ, "vm", vm.Name, "error", err)
	}
}

func (e *engine) reconcileDeploymentByID(ctx context.Context, groupID int64) (*Deployment, error) {
	group, err := e.store.Queries().VMGroups().GetByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, fmt.Errorf("%w: id=%d", ErrDeploymentNotFound, groupID)
	}
	deployment, err := e.reconcileDeployment(ctx, *group)
	if err != nil {
		return nil, err
	}
	return &deployment, nil
}

func (e *engine) reconcileDeployment(ctx context.Context, group db.VMGroup) (Deployment, error) {
	config, err := vmconfig.Unmarshal(group.ConfigJSON)
	if err != nil {
		return Deployment{}, err
	}
	if config.Manifest == nil {
		return Deployment{}, fmt.Errorf("deployment %s missing manifest", group.Name)
	}

	vmRepo := e.store.Queries().VirtualMachines()
	vms, err := vmRepo.ListByGroupID(ctx, group.ID)
	if err != nil {
		return Deployment{}, err
	}

	current := len(vms)
	desired := group.Replicas

	if current > desired {
		sort.Slice(vms, func(i, j int) bool {
			iIdx, _ := parseReplicaIndex(group.Name, vms[i].Name)
			jIdx, _ := parseReplicaIndex(group.Name, vms[j].Name)
			return iIdx > jIdx
		})
		for i := desired; i < current; i++ {
			if _, err := e.destroyVM(ctx, vms[i].Name, false); err != nil {
				e.logger.Error("scale down deployment", "deployment", group.Name, "vm", vms[i].Name, "error", err)
			}
		}
		vms, err = vmRepo.ListByGroupID(ctx, group.ID)
		if err != nil {
			return Deployment{}, err
		}
	}

	if desired > len(vms) {
		existing := make(map[int]bool, len(vms))
		for _, vm := range vms {
			if idx, ok := parseReplicaIndex(group.Name, vm.Name); ok {
				existing[idx] = true
			}
		}
		groupID := group.ID
		for i := 1; len(existing) < desired; i++ {
			if existing[i] {
				continue
			}
			vmName := replicaName(group.Name, i)
			manifestCopy := *config.Manifest
			manifestCopy.Normalize()
			cfgClone := config.Clone()
			cfgClone.Normalize()
			request := CreateVMRequest{
				Name:              vmName,
				Plugin:            cfgClone.Plugin,
				Runtime:           cfgClone.Runtime,
				CPUCores:          cfgClone.Resources.CPUCores,
				MemoryMB:          cfgClone.Resources.MemoryMB,
				KernelCmdlineHint: cfgClone.KernelCmdline,
				Manifest:          &manifestCopy,
				APIHost:           cfgClone.API.Host,
				APIPort:           cfgClone.API.Port,
				Config:            &cfgClone,
			}
			request.GroupID = &groupID
			if _, err := e.CreateVM(ctx, request); err != nil {
				e.logger.Error("scale up deployment", "deployment", group.Name, "vm", vmName, "error", err)
				break
			} else {
				existing[i] = true
			}
		}
		vms, err = vmRepo.ListByGroupID(ctx, group.ID)
		if err != nil {
			return Deployment{}, err
		}
	}

	deployment, err := e.buildDeployment(ctx, group)
	if err != nil {
		return Deployment{}, err
	}
	return deployment, nil
}

func (e *engine) buildDeployment(ctx context.Context, group db.VMGroup) (Deployment, error) {
	config, err := vmconfig.Unmarshal(group.ConfigJSON)
	if err != nil {
		return Deployment{}, err
	}
	vms, err := e.store.Queries().VirtualMachines().ListByGroupID(ctx, group.ID)
	if err != nil {
		return Deployment{}, err
	}
	ready := 0
	for _, vm := range vms {
		if vm.Status == db.VMStatusRunning {
			ready++
		}
	}
	return Deployment{
		Name:            group.Name,
		DesiredReplicas: group.Replicas,
		ReadyReplicas:   ready,
		Config:          config,
		CreatedAt:       group.CreatedAt,
		UpdatedAt:       group.UpdatedAt,
	}, nil
}

func normalizeDeploymentConfig(cfg vmconfig.Config) (vmconfig.Config, error) {
	clone := cfg.Clone()
	clone.Normalize()
	if strings.TrimSpace(clone.Plugin) == "" && clone.Manifest != nil {
		clone.Plugin = strings.TrimSpace(clone.Manifest.Name)
	}
	if err := clone.Validate(); err != nil {
		return vmconfig.Config{}, err
	}
	return clone, nil
}

func (e *engine) prepareCloudInitSeed(ctx context.Context, vm *db.VM, manifest *pluginspec.Manifest, override *pluginspec.CloudInit) (*pluginspec.CloudInit, *db.VMCloudInit, *runtime.Disk, error) {
	if vm == nil {
		return nil, nil, nil, fmt.Errorf("prepare cloud-init: vm required")
	}

	base := (*pluginspec.CloudInit)(nil)
	if manifest != nil && manifest.CloudInit != nil {
		copy := *manifest.CloudInit
		copy.Normalize()
		base = &copy
	}
	merged := mergeCloudInit(base, override)
	if merged == nil {
		if vm.ID != 0 {
			queries := e.store.Queries()
			if existing, err := queries.VMCloudInit().Get(ctx, vm.ID); err == nil {
				if existing != nil {
					if path := strings.TrimSpace(existing.SeedPath); path != "" {
						_ = os.Remove(path)
					}
				}
			} else {
				return nil, nil, nil, err
			}
		}
		return nil, nil, nil, nil
	}
	merged.Normalize()
	if err := merged.Validate(); err != nil {
		return nil, nil, nil, fmt.Errorf("cloud-init validate: %w", err)
	}

	queries := e.store.Queries()
	var previous *db.VMCloudInit
	if vm.ID != 0 {
		record, err := queries.VMCloudInit().Get(ctx, vm.ID)
		if err != nil {
			return nil, nil, nil, err
		}
		previous = record
	}

	seedsDir := filepath.Join(e.runtimeDir, "cloudinit")
	if err := os.MkdirAll(seedsDir, 0o755); err != nil {
		return nil, nil, nil, fmt.Errorf("prepare cloud-init: ensure seeds dir: %w", err)
	}
	seedPath := filepath.Join(seedsDir, fmt.Sprintf("%s-seed.img", vm.Name))

	input := cloudinit.SeedInput{
		InstanceID:    fmt.Sprintf("volant-%d", vm.ID),
		Hostname:      vm.Name,
		UserData:      strings.TrimSpace(merged.UserData.Content),
		MetaData:      strings.TrimSpace(merged.MetaData.Content),
		NetworkConfig: strings.TrimSpace(merged.NetworkCfg.Content),
	}
	if err := cloudinit.Build(ctx, input, seedPath); err != nil {
		return nil, nil, nil, fmt.Errorf("cloud-init build: %w", err)
	}

	if previous != nil {
		oldPath := strings.TrimSpace(previous.SeedPath)
		if oldPath != "" && oldPath != seedPath {
			_ = os.Remove(oldPath)
		}
	}

	record := &db.VMCloudInit{
		VMID:          vm.ID,
		UserData:      input.UserData,
		MetaData:      input.MetaData,
		NetworkConfig: input.NetworkConfig,
		SeedPath:      seedPath,
	}

	seedDisk := &runtime.Disk{
		Name:     "seed",
		Path:     seedPath,
		Readonly: true,
	}

	return merged, record, seedDisk, nil
}

func mergeCloudInit(base, override *pluginspec.CloudInit) *pluginspec.CloudInit {
	if base == nil && override == nil {
		return nil
	}
	result := pluginspec.CloudInit{}
	if base != nil {
		result = *base
	}
	if override == nil {
		return &result
	}
	if strings.TrimSpace(override.Datasource) != "" {
		result.Datasource = override.Datasource
	}
	if strings.TrimSpace(override.SeedMode) != "" {
		result.SeedMode = override.SeedMode
	}
	result.UserData = mergeCloudInitDoc(result.UserData, override.UserData)
	result.MetaData = mergeCloudInitDoc(result.MetaData, override.MetaData)
	result.NetworkCfg = mergeCloudInitDoc(result.NetworkCfg, override.NetworkCfg)
	return &result
}

func mergeCloudInitDoc(base, override pluginspec.CloudInitDoc) pluginspec.CloudInitDoc {
	if strings.TrimSpace(override.Content) != "" || strings.TrimSpace(override.Path) != "" || override.Inline {
		return override
	}
	return base
}

func buildAdditionalDisks(manifest *pluginspec.Manifest) []runtime.Disk {
	if manifest == nil {
		return nil
	}
	if len(manifest.Disks) == 0 {
		return nil
	}
	disks := make([]runtime.Disk, 0, len(manifest.Disks))
	for _, disk := range manifest.Disks {
		path := strings.TrimSpace(disk.Source)
		if path == "" {
			continue
		}
		disks = append(disks, runtime.Disk{
			Name:     strings.TrimSpace(disk.Name),
			Path:     path,
			Checksum: strings.TrimSpace(disk.Checksum),
			Readonly: disk.Readonly,
		})
	}
	if len(disks) == 0 {
		return nil
	}
	return disks
}

func replicaName(base string, index int) string {
	return fmt.Sprintf("%s-%d", base, index)
}

func parseReplicaIndex(base, name string) (int, bool) {
	if !strings.HasPrefix(name, base) {
		return 0, false
	}
	suffix := strings.TrimPrefix(name, base)
	if suffix == "" {
		return 0, false
	}
	if !strings.HasPrefix(suffix, "-") {
		return 0, false
	}
	idx, err := strconv.Atoi(suffix[1:])
	if err != nil || idx <= 0 {
		return 0, false
	}
	return idx, true
}

func validateCreateRequest(req CreateVMRequest) error {
	if req.Name == "" {
		return fmt.Errorf("orchestrator: vm name required")
	}
	if req.CPUCores <= 0 {
		return fmt.Errorf("orchestrator: cpu cores must be > 0")
	}
	if req.MemoryMB <= 0 {
		return fmt.Errorf("orchestrator: memory must be > 0")
	}
	return nil
}

func deriveMAC(name, ip string) string {
	h := sha1.Sum([]byte(name + "|" + ip))
	return fmt.Sprintf("02:%02x:%02x:%02x:%02x:%02x", h[0], h[1], h[2], h[3], h[4])
}

func sanitizeHostname(name string) string {
	cleaned := make([]rune, 0, len(name))
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			cleaned = append(cleaned, r)
		}
	}
	if len(cleaned) == 0 {
		return "vm"
	}
	return string(cleaned)
}

func buildKernelCmdline(ip, gateway, netmask, hostname, extra string) string {
	base := fmt.Sprintf("console=ttyS0 reboot=k panic=1 ip=%s::%s:%s:%s:eth0:off", ip, gateway, netmask, hostname)
	extra = strings.TrimSpace(extra)
	if extra == "" {
		return base
	}
	return base + " " + extra
}

func appendKernelArgs(cmdline string, args map[string]string) string {
	if len(args) == 0 {
		return cmdline
	}
	baseParts := strings.Fields(cmdline)
	extra := make([]string, 0, len(args))
	for key, value := range args {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		trimmedValue := strings.TrimSpace(value)
		if trimmedValue == "" {
			extra = append(extra, trimmedKey)
			continue
		}
		extra = append(extra, fmt.Sprintf("%s=%s", trimmedKey, trimmedValue))
	}
	if len(extra) == 0 {
		return strings.Join(baseParts, " ")
	}
	sort.Strings(extra)
	parts := append(baseParts, extra...)
	return strings.Join(parts, " ")
}

func cloneArgs(args map[string]string) map[string]string {
	if len(args) == 0 {
		return nil
	}
	dup := make(map[string]string, len(args))
	for key, value := range args {
		dup[key] = value
	}
	return dup
}

func deriveIPPool(subnet *net.IPNet, hostIP net.IP) ([]string, error) {
	ipv4 := subnet.IP.To4()
	if ipv4 == nil {
		return nil, fmt.Errorf("ipv6 subnets are not supported: %s", subnet)
	}

	ones, bits := subnet.Mask.Size()
	hostBits := bits - ones
	if hostBits <= 0 {
		return nil, fmt.Errorf("invalid subnet mask: %s", subnet.Mask)
	}

	total := 1 << hostBits
	base := binary.BigEndian.Uint32(ipv4.Mask(subnet.Mask))
	host := binary.BigEndian.Uint32(hostIP.To4())

	pool := make([]string, 0, total-2)
	for i := 0; i < total; i++ {
		addr := base + uint32(i)
		ip := make(net.IP, net.IPv4len)
		binary.BigEndian.PutUint32(ip, addr)

		if !subnet.Contains(ip) {
			continue
		}
		if addr == base { // network address
			continue
		}
		if addr == base+uint32(total-1) { // broadcast
			continue
		}
		if addr == host {
			continue
		}
		pool = append(pool, ip.String())
	}

	if len(pool) == 0 {
		return nil, fmt.Errorf("no assignable IPs in subnet %s", subnet)
	}
	return pool, nil
}

func formatNetmask(mask net.IPMask) string {
	if len(mask) != 4 {
		return "255.255.255.0"
	}
	parts := make([]string, len(mask))
	for i, b := range mask {
		parts[i] = fmt.Sprintf("%d", int(b))
	}
	return strings.Join(parts, ".")
}

func (e *engine) launchContext() context.Context {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.procCtx != nil {
		return e.procCtx
	}
	return context.Background()
}

func (e *engine) apiEndpoints() (string, string) {
	defaultHost := e.hostIP.String()
	defaultPort := e.controlPort
	advAddr := strings.TrimSpace(e.controlAdvertiseAddr)
	if advAddr != "" {
		if advHost, advPort, err := net.SplitHostPort(advAddr); err == nil {
			advHost = strings.TrimSpace(advHost)
			advPort = strings.TrimSpace(advPort)
			if advPort != "" && advPort != "0" {
				defaultPort = advPort
			}
			if isUsableAdvertiseHost(advHost) {
				defaultHost = advHost
			}
		} else if isUsableAdvertiseHost(advAddr) {
			defaultHost = advAddr
		}
	}
	if strings.TrimSpace(defaultPort) == "" || defaultPort == "0" {
		defaultPort = e.controlPort
	}
	if strings.TrimSpace(defaultHost) == "" {
		defaultHost = e.hostIP.String()
	}
	return defaultHost, defaultPort
}

func isUsableAdvertiseHost(host string) bool {
	trimmed := strings.TrimSpace(host)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	switch lower {
	case "localhost", "0.0.0.0", "::", "[::]", "[::1]", "::1":
		return false
	}
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		trimmed = strings.Trim(trimmed, "[]")
	}
	ip := net.ParseIP(trimmed)
	if ip != nil {
		if ip.IsLoopback() || ip.IsUnspecified() {
			return false
		}
	}
	return true
}

// resolveNetworkConfig determines the effective network configuration for a VM.
// VM-level config overrides plugin-level defaults.
func resolveNetworkConfig(manifest *pluginspec.Manifest, vmConfig *vmconfig.Config) *pluginspec.NetworkConfig {
	// VM config takes precedence
	if vmConfig != nil && vmConfig.Network != nil {
		return vmConfig.Network
	}
	// Fall back to plugin manifest
	if manifest != nil && manifest.Network != nil {
		return manifest.Network
	}
	// No network config specified - use default (bridged with IP)
	return nil
}

// needsIPAllocation returns true if the network mode requires host-managed IP allocation.
func needsIPAllocation(netCfg *pluginspec.NetworkConfig) bool {
	if netCfg == nil {
		return true // Default behavior: allocate IP
	}
	mode := pluginspec.NetworkMode(strings.ToLower(strings.TrimSpace(string(netCfg.Mode))))
	// Only bridged mode with host-managed IPs needs allocation
	// vsock and dhcp modes do not need host IP allocation
	return mode == pluginspec.NetworkModeBridged || mode == ""
}

// needsTapDevice returns true if the network mode requires a tap device.
func needsTapDevice(netCfg *pluginspec.NetworkConfig) bool {
	if netCfg == nil {
		return true // Default behavior: create tap
	}
	mode := pluginspec.NetworkMode(strings.ToLower(strings.TrimSpace(string(netCfg.Mode))))
	// vsock mode doesn't need a tap device
	return mode != pluginspec.NetworkModeVsock
}

func (e *engine) setVMState(ctx context.Context, vmID int64, status db.VMStatus, pid *int64) {
	if err := e.store.WithTx(ctx, func(q db.Queries) error {
		return q.VirtualMachines().UpdateRuntimeState(ctx, vmID, status, pid)
	}); err != nil {
		e.logger.Error("update vm state", "vm_id", vmID, "status", status, "error", err)
	}
}

// allocateNextCID finds the next available vsock CID starting from 3.
// CIDs 0-2 are reserved: 0=hypervisor, 1=local, 2=host.
func (e *engine) allocateNextCID(ctx context.Context, vmRepo db.VMRepository) (uint32, error) {
	vms, err := vmRepo.List(ctx)
	if err != nil {
		return 0, err
	}

	// Collect all used CIDs
	used := make(map[uint32]bool)
	for _, vm := range vms {
		if vm.VsockCID > 0 {
			used[vm.VsockCID] = true
		}
	}

	// Find first available CID starting from 3
	// CIDs 0-2 are reserved
	for cid := uint32(3); cid < 1<<32-1; cid++ {
		if !used[cid] {
			return cid, nil
		}
	}

	return 0, fmt.Errorf("no available vsock CIDs")
}
