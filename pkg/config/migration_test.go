package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigMigration_OldLLMModelToFastAndComplex(t *testing.T) {
	dir := t.TempDir()

	mgr := NewManager()
	mgr.configPath = filepath.Join(dir, "config.json")
	mgr.quickCommandsPath = filepath.Join(dir, "quick_commands.json")

	oldCfg := map[string]any{
		"llm": map[string]any{
			"APIKey":  "k",
			"BaseURL": "https://example.com/v1",
			"Model":   "old-model",
		},
		"log": map[string]any{
			"dir": dir,
		},
		"docs": map[string]any{
			"dir": "",
		},
		"completion_delay": 1500,
	}
	b, err := json.MarshalIndent(oldCfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal old config: %v", err)
	}
	if err := os.WriteFile(mgr.configPath, b, 0644); err != nil {
		t.Fatalf("write old config: %v", err)
	}

	if err := mgr.Load(); err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if mgr.Config.LLM.FastModel != "old-model" {
		t.Fatalf("FastModel = %q, want %q", mgr.Config.LLM.FastModel, "old-model")
	}
	if mgr.Config.LLM.ComplexModel != "glm46" {
		t.Fatalf("ComplexModel = %q, want %q", mgr.Config.LLM.ComplexModel, "glm46")
	}
	if mgr.Config.LLM.Model != "" {
		t.Fatalf("Model = %q, want empty after migration", mgr.Config.LLM.Model)
	}
	if mgr.Config.CommandQueryShortcut != "Ctrl+K" {
		t.Fatalf("CommandQueryShortcut = %q, want %q", mgr.Config.CommandQueryShortcut, "Ctrl+K")
	}
	if mgr.Config.CLI.ExecTimeoutSec != DefaultCLIExecTimeoutSec {
		t.Fatalf("CLI.ExecTimeoutSec = %d, want %d", mgr.Config.CLI.ExecTimeoutSec, DefaultCLIExecTimeoutSec)
	}

	data, err := os.ReadFile(mgr.configPath)
	if err != nil {
		t.Fatalf("read migrated config: %v", err)
	}
	var saved map[string]any
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("unmarshal migrated config: %v", err)
	}
	cli, ok := saved["cli"].(map[string]any)
	if !ok {
		t.Fatalf("migrated config missing cli section: %#v", saved)
	}
	if got := int(cli["exec_timeout_sec"].(float64)); got != DefaultCLIExecTimeoutSec {
		t.Fatalf("saved cli.exec_timeout_sec = %d, want %d", got, DefaultCLIExecTimeoutSec)
	}
}
