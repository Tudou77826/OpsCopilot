package shellsidecar

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"opscopilot/pkg/companion"
	"opscopilot/pkg/config"
)

func TestCompanionSettingsApplyOnlyAfterSuccessfulSave(t *testing.T) {
	dir := t.TempDir()
	settings, err := NewSettingsService(dir)
	if err != nil {
		t.Fatal(err)
	}
	store := companion.NewDormant(filepath.Join(dir, "companion.json"))
	settings.onSaved = func(next ShellSettingsJSON) { store.SetEnabled(next.CompanionEnabled) }
	next, err := settings.Get()
	if err != nil {
		t.Fatal(err)
	}
	if !next.CompanionEnabled || store.Enabled() {
		t.Fatal("settings must default on; dormant store awaits successful save")
	}
	next.CompanionEnabled = true
	if err := settings.Save(next); err != nil {
		t.Fatal(err)
	}
	if !store.Enabled() {
		t.Fatal("save did not enable")
	}
	next.CompanionEnabled = false
	if err := settings.Save(next); err != nil {
		t.Fatal(err)
	}
	settings.path = filepath.Join(dir, "missing", "settings.json")
	next.CompanionEnabled = true
	if err := settings.Save(next); err == nil {
		t.Fatal("expected failed save")
	}
	if store.Enabled() {
		t.Fatal("failed save enabled store")
	}
}

func TestCompanionSettingsDefaultAndExplicitOff(t *testing.T) {
	for _, data := range []string{`{}`, `{"companionEnabled":false}`} {
		t.Run(data, func(t *testing.T) {
			service, err := NewSettingsService(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(service.path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			settings, err := service.Get()
			if err != nil {
				t.Fatal(err)
			}
			want := data == `{}`
			if settings.CompanionEnabled != want {
				t.Fatal("unexpected companion state")
			}
			if err := service.Save(settings); err != nil {
				t.Fatal(err)
			}
			next, err := service.Get()
			if err != nil || next.CompanionEnabled != want {
				t.Fatal("saved companion choice not retained", err)
			}
		})
	}
}

func TestSettingsApplyMergedFeatureStateAndPreserveCorruption(t *testing.T) {
	dir := t.TempDir()
	manager := config.NewManagerWithDir(dir)
	if err := manager.Load(); err != nil {
		t.Fatal(err)
	}
	service := &SettingsService{desktop: &desktopSettings{root: dir, snapshots: map[string]*config.Manager{}}}
	var applied bool
	service.onSaved = func(next ShellSettingsJSON) { applied = next.CompanionEnabled }
	settings, err := service.Get()
	if err != nil {
		t.Fatal(err)
	}
	other := config.NewManagerWithDir(dir)
	if err = other.Load(); err != nil {
		t.Fatal(err)
	}
	other.Config.Experimental.CompanionEnabled = true
	if err = other.Save(); err != nil {
		t.Fatal(err)
	}
	settings.CompletionDelay = 222
	if err = service.Save(settings); err != nil {
		t.Fatal(err)
	}
	if !applied {
		t.Fatal("runtime did not apply the merged persisted feature state")
	}
	standalone, err := NewSettingsService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	damaged := []byte("{broken")
	if err = os.WriteFile(standalone.path, damaged, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = standalone.Get(); err == nil {
		t.Fatal("corruption silently reset")
	}
	if err = standalone.Save(defaultShellSettings()); err == nil {
		t.Fatal("corruption overwritten")
	}
	after, _ := os.ReadFile(standalone.path)
	if string(after) != string(damaged) {
		t.Fatal("damaged file changed")
	}
}

func TestRetiredGardenRPCsCannotTouchLegacyData(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "garden.json")
	os.WriteFile(legacy, []byte("saved legacy"), 0600)
	api := &ControlAPI{Companion: &CompanionService{inner: companion.NewDormant(filepath.Join(dir, "companion.json"))}}
	for _, method := range []string{"shell.garden.snapshot", "shell.garden.signal", "shell.garden.purchase", "shell.garden.place", "shell.garden.stow"} {
		if _, err := api.dispatch(context.Background(), &rpcRequest{Method: method}); err == nil {
			t.Fatalf("retired API allowed %s", method)
		}
	}
	after, _ := os.ReadFile(legacy)
	if string(after) != "saved legacy" {
		t.Fatal("legacy touched")
	}
}

func TestCompanionRPCsUseAuthoritativeLocalGrowth(t *testing.T) {
	store := companion.NewDormant(filepath.Join(t.TempDir(), "companion.json"))
	store.SetEnabled(true)
	api := &ControlAPI{Companion: &CompanionService{inner: store}}
	params := json.RawMessage(`{"revision":0}`)
	if _, err := api.dispatch(context.Background(), &rpcRequest{Method: "shell.companion.hatch", Params: params}); err != nil {
		t.Fatal(err)
	}
	if err := api.Companion.Record(companion.EventTransferCompleted, "task"); err != nil {
		t.Fatal(err)
	}
	raw, err := api.dispatch(context.Background(), &rpcRequest{Method: "shell.companion.snapshot"})
	if err != nil {
		t.Fatal(err)
	}
	snap := raw.(map[string]any)["companion"].(*companion.Snapshot)
	if snap.Pet.XP != 3 || snap.Level != 1 {
		t.Fatal(snap)
	}
}

func TestCompanionBusinessHooksOnlyRewardSuccessfulResults(t *testing.T) {
	dir := t.TempDir()
	store := companion.NewDormant(filepath.Join(dir, "companion.json"))
	store.SetEnabled(true)
	if _, err := store.Hatch(0); err != nil {
		t.Fatal(err)
	}
	ft := NewFTService(nil, dir)
	defer ft.Close()
	scripts := &StructuredScriptService{}
	api := &ControlAPI{Companion: &CompanionService{inner: store}, FT: ft, Scripts: scripts}
	wireCompanion(api)
	ft.notifyDone("failed", "terminal", false, false, "failed", 0)
	snap, err := store.ReadSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Pet.XP != 0 {
		t.Fatal("failure rewarded")
	}
	ft.onTransferDone("task")
	ft.onTransferDone("task")
	scripts.onReplayDispatched("run")
	scripts.onReplayDispatched("run")
	snap, err = store.ReadSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Pet.XP != 7 {
		t.Fatal(snap)
	}
	wireCompanion(&ControlAPI{})
	wireCompanion(&ControlAPI{Companion: api.Companion})
}
