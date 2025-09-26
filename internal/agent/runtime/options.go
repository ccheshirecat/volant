package runtime

// ConfigAccessor describes configuration sources (env, agent config, manifest).
type ConfigAccessor interface {
	Get(name string) string
}
