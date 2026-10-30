package acp

import (
	"context"
	"encoding/json"
	"testing"
)

func TestSessionFeaturesTrackOptionsAndCommands(t *testing.T) {
	store := &sessionFeatureStore{}
	store.update(json.RawMessage(`{"configOptions":[{"id":"model","name":"Model","category":"model","type":"select","currentValue":"p/a","options":[{"value":"p/a","name":"First"},{"value":"p/b","name":"Second"}]}]}`))
	store.update(json.RawMessage(`{"update":{"sessionUpdate":"available_commands_update","availableCommands":[{"name":"review","description":"Review"},{"name":"review","description":"Duplicate"},{"name":"bad name"}]}}`))
	got := store.snapshot()
	if len(got.Options) != 1 || got.Options[0].Current != "p/a" || len(got.Options[0].Choices) != 2 || len(got.Commands) != 1 || got.Commands[0].Name != "review" {
		t.Fatalf("features = %+v", got)
	}
	got.Options[0].Choices[0].Value = "modified"
	got.Commands[0].Name = "modified"
	if s := store.snapshot(); s.Options[0].Choices[0].Value != "p/a" || s.Commands[0].Name != "review" {
		t.Fatal("snapshot aliases internal state")
	}
	store.update(json.RawMessage(`{"update":{"sessionUpdate":"available_commands_update","availableCommands":[]}}`))
	if len(store.snapshot().Commands) != 0 {
		t.Fatal("empty update must clear old commands")
	}
	store.reset()
	if len(store.snapshot().Options) != 0 {
		t.Fatal("session options survived reset")
	}
}

func TestIdleSessionFeaturesAreSessionScopedAndCannotAuthorize(t *testing.T) {
	store := &sessionFeatureStore{}
	router := &managedRouter{observeFeatures: store.update}
	idle := router.Capture()
	idle.Notify(context.Background(), "session/update", json.RawMessage(`{"sessionId":"other","update":{"sessionUpdate":"available_commands_update","availableCommands":[{"name":"foreign"}]}}`))
	idle.Notify(context.Background(), "session/update", json.RawMessage(`{"sessionId":"right","update":{"sessionUpdate":"available_commands_update","availableCommands":[{"name":"review"}]}}`))
	router.setSession("right")
	if got := store.snapshot(); len(got.Commands) != 1 || got.Commands[0].Name != "review" {
		t.Fatalf("commands = %+v", got.Commands)
	}
	if _, err := idle.Request(context.Background(), "session/request_permission", json.RawMessage(`{}`)); err == nil {
		t.Fatal("idle handler granted access")
	}
	idle.Notify(context.Background(), "session/update", json.RawMessage(`{"sessionId":"other","update":{"sessionUpdate":"available_commands_update","availableCommands":[{"name":"foreign"}]}}`))
	if store.snapshot().Commands[0].Name != "review" {
		t.Fatal("foreign session overwrote commands")
	}
}
