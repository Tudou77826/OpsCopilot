package main

import (
	"os"
	"path/filepath"
	"testing"

	"opscopilot/pkg/config"
)

func TestLoadCLIExecTimeoutSecFromConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"cli":{"exec_timeout_sec":300}}`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got := loadCLIExecTimeoutSec(cliEnv{binDir: dir})
	if got != 300 {
		t.Fatalf("loadCLIExecTimeoutSec = %d, want 300", got)
	}
}

func TestLoadCLIExecTimeoutSecFallback(t *testing.T) {
	got := loadCLIExecTimeoutSec(cliEnv{binDir: t.TempDir()})
	if got != config.DefaultCLIExecTimeoutSec {
		t.Fatalf("loadCLIExecTimeoutSec = %d, want %d", got, config.DefaultCLIExecTimeoutSec)
	}
}
