package updater

import (
	"encoding/json"
	"fmt"
	"os"
)

// Manifest is the contract between the running app (Phase 1) and the
// self-update process (Phase 2).
type Manifest struct {
	ExtractedDir string `json:"extractedDir"`
	AppDir       string `json:"appDir"`
	ExePath      string `json:"exePath"`
	ParentPid    int    `json:"parentPid"`
	Version      string `json:"version"`
	LogPath      string `json:"logPath"`
}

// WriteManifest serializes the manifest to the given file path.
func WriteManifest(m *Manifest, path string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	return nil
}

// ReadManifest reads and deserializes a manifest from the given file path.
func ReadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return &m, nil
}
