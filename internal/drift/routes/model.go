package routes

// BackendType represents a routing backend.
type BackendType string

const (
	BackendBridge BackendType = "bridge"
	BackendVsock  BackendType = "vsock"
)

// Backend describes the routing destination for a host port.
type Backend struct {
	Type BackendType `json:"type"`
	IP   string      `json:"ip,omitempty"`
	Port uint16      `json:"port"`
	CID  uint32      `json:"cid,omitempty"`
}

// Route binds an exposed host port to a backend target.
type Route struct {
	HostPort uint16  `json:"host_port"`
	Protocol string  `json:"protocol"`
	Backend  Backend `json:"backend"`
}
