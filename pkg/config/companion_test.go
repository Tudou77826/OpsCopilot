package config

import (
	"os"
	"testing"
)

func TestLegacyGardenFlagDoesNotEnableCompanion(t *testing.T) {
	dir := t.TempDir()
	manager := NewManagerWithDir(dir)
	if err := os.WriteFile(manager.configPath, []byte(`{"experimental":{"garden_enabled":true}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := manager.Load(); err != nil {
		t.Fatal(err)
	}
	if manager.Config.Experimental.CompanionEnabled {
		t.Fatal("retired flag enabled new feature")
	}
	manager.Config.Experimental.CompanionEnabled = true
	if err := manager.Save(); err != nil {
		t.Fatal(err)
	}
	reread := NewManagerWithDir(dir)
	if err := reread.Load(); err != nil {
		t.Fatal(err)
	}
	if !reread.Config.Experimental.CompanionEnabled {
		t.Fatal("new flag not persisted")
	}
}
