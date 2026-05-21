package updater

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteReadManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")

	original := &Manifest{
		ExtractedDir: "/tmp/extracted",
		AppDir:       "C:\\app",
		ExePath:      "C:\\app\\opscopilot.exe",
		ParentPid:    12345,
		Version:      "v1.5.4",
		LogPath:      "C:\\app\\update.log",
	}

	if err := WriteManifest(original, path); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	got, err := ReadManifest(path)
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}

	if got.ExtractedDir != original.ExtractedDir {
		t.Errorf("ExtractedDir = %q, want %q", got.ExtractedDir, original.ExtractedDir)
	}
	if got.AppDir != original.AppDir {
		t.Errorf("AppDir = %q, want %q", got.AppDir, original.AppDir)
	}
	if got.ParentPid != original.ParentPid {
		t.Errorf("ParentPid = %d, want %d", got.ParentPid, original.ParentPid)
	}
	if got.Version != original.Version {
		t.Errorf("Version = %q, want %q", got.Version, original.Version)
	}
}

func TestReadManifestCorrupt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	os.WriteFile(path, []byte("{invalid json"), 0644)

	_, err := ReadManifest(path)
	if err == nil {
		t.Error("expected error for corrupt JSON")
	}
}

func TestReadManifestMissing(t *testing.T) {
	_, err := ReadManifest("/nonexistent/path/manifest.json")
	if err == nil {
		t.Error("expected error for missing file")
	}
}
