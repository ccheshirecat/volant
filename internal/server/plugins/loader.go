package plugins

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ccheshirecat/volant/internal/pluginspec"
)

// Loader handles manifest discovery and parsing.
type Loader interface {
	Load(path string) (pluginspec.Manifest, error)
}

// FileLoader loads plugin manifests from disk.
type FileLoader struct{}

// Load reads and parses a manifest file.
func (FileLoader) Load(path string) (pluginspec.Manifest, error) {
	if strings.TrimSpace(path) == "" {
		return pluginspec.Manifest{}, fmt.Errorf("plugin loader: path required")
	}
	file, err := os.Open(path)
	if err != nil {
		return pluginspec.Manifest{}, fmt.Errorf("plugin loader: open %s: %w", path, err)
	}
	defer file.Close()

	manifest, err := decodeManifest(file)
	if err != nil {
		return pluginspec.Manifest{}, fmt.Errorf("plugin loader: decode %s: %w", path, err)
	}
	return manifest, nil
}

func decodeManifest(reader io.Reader) (pluginspec.Manifest, error) {
	var manifest pluginspec.Manifest
	if err := json.NewDecoder(reader).Decode(&manifest); err != nil {
		return pluginspec.Manifest{}, err
	}
	if err := manifest.Validate(); err != nil {
		return pluginspec.Manifest{}, err
	}
	return manifest, nil
}
