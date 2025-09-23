package events

import "time"

// VMStatus represents the lifecycle stage for event payloads.
type VMStatus string

const (
	VMStatusPending  VMStatus = "pending"
	VMStatusStarting VMStatus = "starting"
	VMStatusRunning  VMStatus = "running"
	VMStatusStopped  VMStatus = "stopped"
	VMStatusCrashed  VMStatus = "crashed"
)

// VMEvent describes a significant change in a VM lifecycle, or a log line emitted by
// one of its processes when Type is TypeVMLog.
type VMEvent struct {
	Type      string    `json:"type"`
	Name      string    `json:"name"`
	Status    VMStatus  `json:"status"`
	IPAddress string    `json:"ip_address,omitempty"`
	MAC       string    `json:"mac_address,omitempty"`
	PID       *int64    `json:"pid,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message,omitempty"`
	Stream    string    `json:"stream,omitempty"`
	Line      string    `json:"line,omitempty"`
}

const (
	TypeVMCreated = "VM_CREATED"
	TypeVMRunning = "VM_RUNNING"
	TypeVMStopped = "VM_STOPPED"
	TypeVMCrashed = "VM_CRASHED"
	TypeVMDeleted = "VM_DELETED"
	TypeVMLog     = "VM_LOG"
)

// Canonical stream identifiers used when VMEvent.Type is TypeVMLog.
const (
	LogStreamStdout = "stdout"
	LogStreamStderr = "stderr"
)

// TopicVMEvents is the default event bus topic for microVM lifecycle.
const TopicVMEvents = "orchestrator.vm.events"

// TopicVMLogs is the default event bus topic for VM log streaming.
const TopicVMLogs = "orchestrator.vm.logs"
