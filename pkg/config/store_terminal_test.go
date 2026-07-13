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
	m.quickCommandsPath = filepath.Join(dir, "quick_commands.json")
	m.highlightRulesPath = filepath.Join(dir, "highlight_rules.json")

	if err := m.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if m.Config.Terminal.Scrollback != 5000 {
		t.Fatalf("expected default scrollback 5000, got %d", m.Config.Terminal.Scrollback)
	}
	if m.Config.Terminal.FontFamily != DefaultTerminalFontFamily || m.Config.Terminal.FontSize != DefaultTerminalFontSize {
		t.Fatalf("unexpected default terminal appearance: %+v", m.Config.Terminal)
	}

	m.Config.Terminal.Scrollback = 8000
	m.Config.Terminal.SearchEnabled = false
	m.Config.Terminal.HighlightEnabled = false
	m.Config.Terminal.FontFamily = "Fira Code"
	m.Config.Terminal.FontSize = 18
	if err := m.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	m2 := NewManager()
	m2.configPath = m.configPath
	m2.quickCommandsPath = m.quickCommandsPath
	m2.highlightRulesPath = m.highlightRulesPath
	if err := m2.Load(); err != nil {
		t.Fatalf("load2: %v", err)
	}
	if m2.Config.Terminal.Scrollback != 8000 || m2.Config.Terminal.SearchEnabled || m2.Config.Terminal.HighlightEnabled || m2.Config.Terminal.FontFamily != "Fira Code" || m2.Config.Terminal.FontSize != 18 {
		t.Fatalf("unexpected terminal config: %+v", m2.Config.Terminal)
	}
}

func TestNormalizeTerminalConfigAppearance(t *testing.T) {
	cfg := NormalizeTerminalConfig(TerminalConfig{Scrollback: 5000})
	if cfg.FontFamily != DefaultTerminalFontFamily || cfg.FontSize != DefaultTerminalFontSize {
		t.Fatalf("expected appearance defaults, got %+v", cfg)
	}

	cfg = NormalizeTerminalConfig(TerminalConfig{Scrollback: 5000, FontFamily: "  Fira Code  ", FontSize: 99})
	if cfg.FontFamily != "Fira Code" || cfg.FontSize != MaxTerminalFontSize {
		t.Fatalf("expected trimmed family and clamped size, got %+v", cfg)
	}

	cfg = NormalizeTerminalConfig(TerminalConfig{Scrollback: 5000, FontFamily: " Inconsolata ", FontSize: 16})
	if cfg.FontFamily != "Inconsolata" || cfg.FontSize != 16 {
		t.Fatalf("expected bundled Inconsolata font to be preserved, got %+v", cfg)
	}

	cfg = NormalizeTerminalConfig(TerminalConfig{Scrollback: 5000, FontFamily: "Consolas", FontSize: 14})
	if cfg.FontFamily != DefaultTerminalFontFamily {
		t.Fatalf("expected unsupported system font to migrate to default, got %+v", cfg)
	}
}
