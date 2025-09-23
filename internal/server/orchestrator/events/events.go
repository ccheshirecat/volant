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

// VMEvent describes a significant change in a VM lifecycle.
type VMEvent struct {
    Type      string    `json:"type"`
    Name      string    `json:"name"`
    Status    VMStatus  `json:"status"`
    IPAddress string    `json:"ip_address,omitempty"`
    MAC       string    `json:"mac_address,omitempty"`
    PID       *int64    `json:"pid,omitempty"`
    Timestamp time.Time `json:"timestamp"`
    Message   string    `json:"message,omitempty"`
}

const (
    TypeVMCreated  = "VM_CREATED"
    TypeVMRunning  = "VM_RUNNING"
    TypeVMStopped  = "VM_STOPPED"
    TypeVMCrashed  = "VM_CRASHED"
    TypeVMDeleted  = "VM_DELETED"
)

// TopicVMEvents is the default event bus topic for microVM lifecycle.
const TopicVMEvents = "orchestrator.vm.events"
