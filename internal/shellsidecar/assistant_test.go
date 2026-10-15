package shellsidecar

import (
	"context"
	"encoding/json"
	"opscopilot/pkg/assistantsettings"
	"opscopilot/pkg/companion"
	"path/filepath"
	"testing"
)

func TestAssistantPreferencesRPC(t *testing.T) {
	api := &ControlAPI{AssistantSettings: assistantsettings.New(filepath.Join(t.TempDir(), "assistant.json"))}
	raw, err := api.dispatch(context.Background(), &rpcRequest{Method: "shell.assistant.getSettings"})
	if err != nil {
		t.Fatal(err)
	}
	before := raw.(assistantsettings.Snapshot)
	params, _ := json.Marshal(map[string]string{"personaPrompt": "耐心、简洁", "revision": before.Revision})
	raw, err = api.dispatch(context.Background(), &rpcRequest{Method: "shell.assistant.saveSettings", Params: params})
	if err != nil {
		t.Fatal(err)
	}
	if raw.(assistantsettings.Snapshot).PersonaPrompt != "耐心、简洁" {
		t.Fatal(raw)
	}
	if _, err = api.dispatch(context.Background(), &rpcRequest{Method: "shell.assistant.saveSettings", Params: params}); err == nil {
		t.Fatal("stale RPC update accepted")
	}
	if _, err = api.dispatch(context.Background(), &rpcRequest{Method: "shell.assistant.saveSettings", Params: json.RawMessage(`{"revision":"x"}`)}); err == nil {
		t.Fatal("missing prompt accepted as reset")
	}
}
func TestHatchedPersonaSurvivesRenameRPC(t *testing.T) {
	dir := t.TempDir()
	store := companion.NewDormant(filepath.Join(dir, "companion.json"))
	store.SetEnabled(true)
	first, err := store.Hatch(0)
	if err != nil {
		t.Fatal(err)
	}
	api := &ControlAPI{Companion: &CompanionService{inner: store}, AssistantSettings: assistantsettings.New(filepath.Join(dir, "assistant.json"))}
	params, _ := json.Marshal(map[string]any{"name": "新名字", "revision": first.Revision})
	if _, err := api.dispatch(context.Background(), &rpcRequest{Method: "shell.companion.rename", Params: params}); err != nil {
		t.Fatal(err)
	}
	saved, _ := store.ReadSnapshot()
	if saved.Pet.SpeciesID != first.Pet.SpeciesID || saved.Pet.ID != first.Pet.ID {
		t.Fatal("switch failed")
	}
	raw, rpcErr := api.dispatch(context.Background(), &rpcRequest{Method: "shell.assistant.getSettings"})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	settings := raw.(assistantsettings.Snapshot)
	if settings.DefaultPrompt != companion.PersonaFor(first.Pet.SpeciesID) {
		t.Fatal("wrong default persona")
	}
	reset, _ := json.Marshal(map[string]string{"personaPrompt": settings.DefaultPrompt, "revision": settings.Revision})
	raw, rpcErr = api.dispatch(context.Background(), &rpcRequest{Method: "shell.assistant.saveSettings", Params: reset})
	if rpcErr != nil || raw.(assistantsettings.Snapshot).PersonaPrompt != "" {
		t.Fatal("reset persisted default as custom", rpcErr)
	}
}
