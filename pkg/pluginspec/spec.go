package pluginspec

import internal "github.com/volantvm/volant/internal/pluginspec"

const (
	CmdlineKey        = internal.CmdlineKey
	RuntimeKey        = internal.RuntimeKey
	PluginKey         = internal.PluginKey
	APIHostKey        = internal.APIHostKey
	APIPortKey        = internal.APIPortKey
	RootFSKey         = internal.RootFSKey
	RootFSChecksumKey = internal.RootFSChecksumKey
)

type (
	Manifest     = internal.Manifest
	RootFS       = internal.RootFS
	Disk         = internal.Disk
	CloudInit    = internal.CloudInit
	CloudInitDoc = internal.CloudInitDoc
	ResourceSpec = internal.ResourceSpec
	Action       = internal.Action
	HealthCheck  = internal.HealthCheck
	Workload     = internal.Workload
)

var (
	Encode = internal.Encode
	Decode = internal.Decode
)
