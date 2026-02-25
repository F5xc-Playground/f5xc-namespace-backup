package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kevingstewart/xcbackup/internal/refs"
)

// Manifest describes a backup's metadata.
type Manifest struct {
	Version             string           `json:"version"`
	ToolVersion         string           `json:"tool_version"`
	Tenant              string           `json:"tenant"`
	Namespace           string           `json:"namespace"`
	Timestamp           string           `json:"timestamp"`
	ResourceCounts      map[string]int   `json:"resource_counts"`
	SkippedViewChildren []string         `json:"skipped_view_children"`
	SharedReferences    []refs.SharedRef `json:"shared_references"`
	Warnings            []string         `json:"warnings"`
	Errors              []string         `json:"errors"`
}

// Write serializes the manifest to manifest.json in the given directory.
func Write(dir string, m *Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}
	path := filepath.Join(dir, "manifest.json")
	return os.WriteFile(path, data, 0644)
}

// Read deserializes manifest.json from the given directory.
func Read(dir string) (*Manifest, error) {
	path := filepath.Join(dir, "manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}
	return &m, nil
}
