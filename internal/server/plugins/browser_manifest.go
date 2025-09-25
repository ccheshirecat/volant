package plugins

var BuiltInBrowserManifest = Manifest{
	Name:    "browser",
	Version: "1.0.0",
	Runtime: "browser",
	Image:   "browser-runtime-image",
	Resources: ResourceSpec{
		CPUCores: 2,
		MemoryMB: 2048,
	},
	Actions: map[string]Action{
		"navigate": {
			Description: "Navigate to a URL",
			TimeoutMs:   60000,
		},
		"screenshot": {
			Description: "Capture a screenshot",
			TimeoutMs:   60000,
		},
		"evaluate": {
			Description: "Evaluate JavaScript expression",
			TimeoutMs:   60000,
		},
		"graphql": {
			Description: "Execute GraphQL request",
			TimeoutMs:   60000,
		},
	},
	HealthCheck: HealthCheck{
		Endpoint: "/healthz",
		Timeout:  10000,
	},
}
