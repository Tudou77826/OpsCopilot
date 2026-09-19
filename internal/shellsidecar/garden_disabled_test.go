package shellsidecar

import (
	"opscopilot/pkg/garden"
	"path/filepath"
	"testing"
)

func TestGardenSettingOnlyAppliesAfterSuccessfulSave(t *testing.T) {
	dir := t.TempDir()
	settings, err := NewSettingsService(dir)
	if err != nil {
		t.Fatal(err)
	}
	store := garden.NewDormant(filepath.Join(dir, "garden.json"))
	settings.onSaved = func(next ShellSettingsJSON) { store.SetEnabled(next.GardenEnabled) }
	next, err := settings.Get()
	if err != nil {
		t.Fatal(err)
	}
	if next.GardenEnabled || store.Enabled() {
		t.Fatal("must default off")
	}
	next.GardenEnabled = true
	if err := settings.Save(next); err != nil {
		t.Fatal(err)
	}
	if !store.Enabled() {
		t.Fatal("save did not enable runtime")
	}
	next.GardenEnabled = false
	if err := settings.Save(next); err != nil {
		t.Fatal(err)
	}
	if store.Enabled() {
		t.Fatal("save did not disable runtime")
	}
	reread, err := settings.Get()
	if err != nil {
		t.Fatal(err)
	}
	if reread.GardenEnabled {
		t.Fatal("disabled state not persisted")
	}
	settings.path = filepath.Join(dir, "missing", "settings.json")
	next.GardenEnabled = true
	if err := settings.Save(next); err == nil {
		t.Fatal("expected failed save")
	}
	if store.Enabled() {
		t.Fatal("failed save changed runtime")
	}
}
