package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHighlightRulesLoadSave(t *testing.T) {
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

	m.Config.HighlightRules = []HighlightRule{
		{ID: "1", Name: "err", Pattern: "error", IsEnabled: true, Priority: 10, Style: HighlightStyle{BackgroundColor: "#111"}},
	}
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
	if len(m2.Config.HighlightRules) != 1 || m2.Config.HighlightRules[0].ID != "1" {
		t.Fatalf("unexpected rules: %+v", m2.Config.HighlightRules)
	}
	if m2.Config.Terminal.Scrollback <= 0 {
		t.Fatalf("expected terminal scrollback default")
	}
}
