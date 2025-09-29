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
	"strings"
	"sync"
	"time"

	"github.com/ccheshirecat/volant/internal/pluginspec"
	"github.com/ccheshirecat/volant/internal/server/db"
	"github.com/ccheshirecat/volant/internal/server/eventbus"
	orchestratorevents "github.com/ccheshirecat/volant/internal/server/orchestrator/events"
	"github.com/ccheshirecat/volant/internal/server/orchestrator/network"
	"github.com/ccheshirecat/volant/internal/server/orchestrator/runtime"
)

// Engine represents the VM orchestration core.
type Engine interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error

	CreateVM(ctx context.Context, req CreateVMRequest) (*db.VM, error)
	DestroyVM(ctx context.Context, name string) error
	ListVMs(ctx context.Context) ([]db.VM, error)
	GetVM(ctx context.Context, name string) (*db.VM, error)
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
}

var (
	// ErrVMExists indicates a VM with the same name already exists.
	ErrVMExists = errors.New("orchestrator: vm already exists")
	// ErrVMNotFound indicates the requested VM does not exist.
	ErrVMNotFound = errors.New("orchestrator: vm not found")
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
	if req.Manifest != nil {
		manifestRuntime = strings.TrimSpace(req.Manifest.Runtime)
	}

	pluginName := ""
	if req.Manifest != nil {
		pluginName = strings.TrimSpace(req.Manifest.Name)
	}

	req.Runtime = strings.TrimSpace(req.Runtime)
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

	err := e.store.WithTx(ctx, func(q db.Queries) error {
		vmRepo := q.VirtualMachines()
		existing, err := vmRepo.GetByName(ctx, req.Name)
		if err != nil {
			return err
		}
		if existing != nil {
			return fmt.Errorf("%w: %s", ErrVMExists, req.Name)
		}

		allocation, err := q.IPAllocations().LeaseNextAvailable(ctx)
		if err != nil {
			return err
		}

		mac := deriveMAC(req.Name, allocation.IPAddress)
		baseCmdline := buildKernelCmdline(allocation.IPAddress, e.hostIP.String(), netmask, hostname, req.KernelCmdlineHint)
		fullCmdline := appendKernelArgs(baseCmdline, map[string]string{})

		vm := &db.VM{
			Name:          req.Name,
			Status:        db.VMStatusStarting,
			Runtime:       req.Runtime,
			IPAddress:     allocation.IPAddress,
			MACAddress:    mac,
			CPUCores:      req.CPUCores,
			MemoryMB:      req.MemoryMB,
			KernelCmdline: fullCmdline,
		}

		id, err := vmRepo.Create(ctx, vm)
		if err != nil {
			return err
		}
		if err := q.IPAllocations().Assign(ctx, allocation.IPAddress, id); err != nil {
			return err
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

	tapName, err := e.network.PrepareTap(ctx, vmRecord.Name, vmRecord.MACAddress)
	if err != nil {
		e.rollbackCreate(ctx, vmRecord)
		return nil, err
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
		SerialSocket:  serialPath,
	}

	cmdArgs := map[string]string{
		pluginspec.RuntimeKey: req.Runtime,
	}
	if req.APIHost != "" {
		cmdArgs[pluginspec.APIHostKey] = req.APIHost
	}
	if req.APIPort != "" {
		cmdArgs[pluginspec.APIPortKey] = req.APIPort
	}
	if pluginName != "" {
		cmdArgs[pluginspec.PluginKey] = pluginName
	}
	spec.KernelCmdline = appendKernelArgs(spec.KernelCmdline, cmdArgs)

	if req.Manifest != nil {
		spec.RootFS = strings.TrimSpace(req.Manifest.RootFS.URL)
		spec.RootFSChecksum = strings.TrimSpace(req.Manifest.RootFS.Checksum)
	}

	launchCtx := e.launchContext()

	instance, err := e.launcher.Launch(launchCtx, spec)
	if err != nil {
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
		return nil, err
	}

	e.mu.Lock()
	handle := processHandle{instance: instance, tapName: tapName, serial: spec.SerialSocket}
	e.instances[vmRecord.Name] = handle
	e.mu.Unlock()

	e.monitorInstance(vmRecord.Name, handle)

	vmRecord.Status = db.VMStatusRunning
	vmRecord.PID = &pid
	e.publishEvent(ctx, orchestratorevents.TypeVMRunning, orchestratorevents.VMStatusRunning, vmRecord, "vm running")
	return vmRecord, nil
}

func (e *engine) DestroyVM(ctx context.Context, name string) error {
	var vmRecord *db.VM
	err := e.store.WithTx(ctx, func(q db.Queries) error {
		vmRepo := q.VirtualMachines()
		vm, err := vmRepo.GetByName(ctx, name)
		if err != nil {
			return err
		}
		if vm == nil {
			return fmt.Errorf("%w: %s", ErrVMNotFound, name)
		}
		vmRecord = &db.VM{
			ID:            vm.ID,
			Name:          vm.Name,
			Status:        vm.Status,
			IPAddress:     vm.IPAddress,
			MACAddress:    vm.MACAddress,
			CPUCores:      vm.CPUCores,
			MemoryMB:      vm.MemoryMB,
			KernelCmdline: vm.KernelCmdline,
		}
		if err := vmRepo.Delete(ctx, vm.ID); err != nil {
			return err
		}
		if err := q.IPAllocations().Release(ctx, vm.IPAddress); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
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
		if err := e.network.CleanupTap(ctx, handle.tapName); err != nil {
			e.logger.Error("cleanup tap", "tap", handle.tapName, "error", err)
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

	e.publishEvent(ctx, orchestratorevents.TypeVMDeleted, orchestratorevents.VMStatusStopped, vmRecord, "vm deleted")

	return nil
}

func (e *engine) ListVMs(ctx context.Context) ([]db.VM, error) {
	return e.store.Queries().VirtualMachines().List(ctx)
}

func (e *engine) GetVM(ctx context.Context, name string) (*db.VM, error) {
	return e.store.Queries().VirtualMachines().GetByName(ctx, name)
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

		if exitErr != nil {
			e.logger.Warn("vm exited unexpectedly", "vm", name, "error", exitErr)
			if vmRecord != nil {
				vmRecord.Status = db.VMStatusCrashed
				vmRecord.PID = nil
			}
			e.publishEvent(ctx, orchestratorevents.TypeVMCrashed, orchestratorevents.VMStatusCrashed, vmRecord, exitErr.Error())
		} else {
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
