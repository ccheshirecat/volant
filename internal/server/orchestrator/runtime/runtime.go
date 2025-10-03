// Copyright (c) 2025 HYPR. PTE. LTD.
//
// Business Source License 1.1
// See LICENSE file in the project root for details.

package runtime

import "context"

// LaunchSpec contains the information required to boot a microVM.
type LaunchSpec struct {
	Name          string
	CPUCores      int
	MemoryMB      int
	KernelCmdline string
	// KernelOverride allows per-VM kernel selection; when empty, the launcher chooses
	// a default based on the presence of Initramfs (vmlinux) or RootFS (bzImage).
	KernelOverride string
	TapDevice      string
	MACAddress     string
	IPAddress      string
	Gateway        string
	Netmask        string
	VsockCID       uint32 // Vsock Context ID for guest communication
	Args           map[string]string
	RootFS         string
	RootFSChecksum string
	// Initramfs, when set, is fetched and used as the initramfs image for the VM.
	// If provided, the launcher will prefer a vmlinux kernel (unless KernelOverride is set).
	Initramfs         string
	InitramfsChecksum string
	SerialSocket      string
	Disks             []Disk
	SeedDisk          *Disk
}

type Disk struct {
	Name     string
	Path     string
	Checksum string
	Readonly bool
}

// Instance represents a running hypervisor process.
type Instance interface {
	Name() string
	PID() int
	APISocketPath() string
	Stop(ctx context.Context) error
	Wait() <-chan error
}

// Launcher is responsible for launching microVMs using a specific hypervisor implementation.
type Launcher interface {
	Launch(ctx context.Context, spec LaunchSpec) (Instance, error)
}
