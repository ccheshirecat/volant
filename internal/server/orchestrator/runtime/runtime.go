package runtime

import "context"

// LaunchSpec contains the information required to boot a microVM.
type LaunchSpec struct {
	Name           string
	CPUCores       int
	MemoryMB       int
	KernelCmdline  string
	TapDevice      string
	MACAddress     string
	IPAddress      string
	Gateway        string
	Netmask        string
	VsockCID       uint32 // Vsock Context ID for guest communication
	Args           map[string]string
	RootFS         string
	RootFSChecksum string
	SerialSocket   string
	Disks          []Disk
	SeedDisk       *Disk
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
