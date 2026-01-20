package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTerminalConfigDefaultAndSave(t *testing.T) {
	dir := t.TempDir()
	wd, _ := os.Getwd()
	_ = os.Chdir(dir)
	t.Cleanup(func() { _ = os.Chdir(wd) })

	m := NewManager()
	m.configPath = filepath.Join(dir, "config.json")
	m.promptsPath = filepath.Join(dir, "prompts.json")
	m.quickCommandsPath = filepath.Join(dir, "quick_commands.json")
	m.highlightRulesPath = filepath.Join(dir, "highlight_rules.json")

	if err := m.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if m.Config.Terminal.Scrollback != 5000 {
		t.Fatalf("expected default scrollback 5000, got %d", m.Config.Terminal.Scrollback)
	}

	m.Config.Terminal.Scrollback = 8000
	m.Config.Terminal.SearchEnabled = false
	m.Config.Terminal.HighlightEnabled = false
	if err := m.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	m2 := NewManager()
	m2.configPath = m.configPath
	m2.promptsPath = m.promptsPath
	m2.quickCommandsPath = m.quickCommandsPath
	m2.highlightRulesPath = m.highlightRulesPath
	if err := m2.Load(); err != nil {
		t.Fatalf("load2: %v", err)
	}
	if m2.Config.Terminal.Scrollback != 8000 || m2.Config.Terminal.SearchEnabled || m2.Config.Terminal.HighlightEnabled {
		t.Fatalf("unexpected terminal config: %+v", m2.Config.Terminal)
	}
}
