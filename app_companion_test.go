package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"opscopilot/pkg/companion"
	"opscopilot/pkg/config"
)

func TestCompanionBindingsAndDisabledEventIsolation(t *testing.T) {
	app := &App{}
	if _, err := app.CompanionSnapshot(); !errors.Is(err, companion.ErrDisabled) {
		t.Fatal(err)
	}
	app.recordCompanion(companion.EventSessionEstablished, "disabled")
	app.companion = companion.NewDormant(filepath.Join(t.TempDir(), "companion.json"))
	app.companion.SetEnabled(true)
	if _, err := app.CompanionHatch(0); err != nil {
		t.Fatal(err)
	}
	app.recordCompanion(companion.EventSessionEstablished, "connection")
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		snap, err := app.CompanionSnapshot()
		if err != nil {
			t.Fatal(err)
		}
		if snap.Pet.XP == 2 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("business event not settled")
}

func TestSettingsRuleFailureCannotEnableCompanion(t *testing.T) {
	dir := t.TempDir()
	manager := config.NewManagerWithDir(dir)
	if err := manager.Load(); err != nil {
		t.Fatal(err)
	}
	manager.Config.Experimental.CompanionEnabled = false
	if err := manager.Save(); err != nil {
		t.Fatal(err)
	}
	app := &App{configMgr: manager, companion: companion.NewDormant(filepath.Join(dir, "companion.json"))}
	rulesPath := filepath.Join(dir, "highlight_rules.json")
	if err := os.Remove(rulesPath); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.Mkdir(rulesPath, 0700); err != nil {
		t.Fatal(err)
	}
	next := *manager.Config
	next.Experimental.CompanionEnabled = true
	next.HighlightRules = []config.HighlightRule{{ID: "failure", Name: "failure", Pattern: "ERROR", IsEnabled: true}}
	if message := app.SaveSettings(next); message == "" {
		t.Fatal("expected rule write failure")
	}
	if app.companion.Enabled() || manager.Config.Experimental.CompanionEnabled {
		t.Fatal("failed save enabled runtime or memory")
	}
	// Inspect only the main config: the intentionally broken rules file stays.
	raw, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"companion_enabled": true`) {
		t.Fatal("failed save persisted opt-in")
	}
}
