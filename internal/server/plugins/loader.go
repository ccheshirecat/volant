package plugins

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// Loader handles manifest discovery and parsing.
type Loader interface {
	Load(path string) (Manifest, error)
}

// FileLoader loads plugin manifests from disk.
type FileLoader struct{}

// Load reads and parses a manifest file.
func (FileLoader) Load(path string) (Manifest, error) {
	if strings.TrimSpace(path) == "" {
		return Manifest{}, fmt.Errorf("plugin loader: path required")
	}
	file, err := os.Open(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("plugin loader: open %s: %w", path, err)
	}
	defer file.Close()

	manifest, err := decodeManifest(file)
	if err != nil {
		return Manifest{}, fmt.Errorf("plugin loader: decode %s: %w", path, err)
	}
	return manifest, nil
}

func decodeManifest(reader io.Reader) (Manifest, error) {
	var manifest Manifest
	if err := json.NewDecoder(reader).Decode(&manifest); err != nil {
		return Manifest{}, err
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}
