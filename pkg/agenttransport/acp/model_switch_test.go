package acp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"opscopilot/pkg/assistantcontrol"
)

func TestLocalModelCatalogAndExplicitSelection(t *testing.T) {
	t.Setenv("CHRYS_MODEL_PROFILE", "chosen")
	dir := localModelFixture(t, "id: chosen\nname: First\nmodel_id: model-one\napi_key: private-key\nbase_url: https://private.invalid\n", "model.profile.active: chosen\n", "")
	_ = os.WriteFile(filepath.Join(dir, "models/second.yaml"), []byte("id: second\nname: Second\nmodel_id: model-two\napi_key: other-private-key\n"), 0600)
	options, err := ListLocalModels(dir)
	data, _ := json.Marshal(options)
	if err != nil || len(options) != 2 || strings.Contains(string(data), "private") {
		t.Fatal("catalog is missing metadata or leaks private configuration", err)
	}
	model, err := LoadLocalModelByID(dir, "second")
	if err != nil || model.ID != "second" || model.ModelID != "model-two" {
		t.Fatal("explicit choice did not override default selector", err)
	}
	if _, err = LoadLocalModelByID(dir, "Second"); err == nil {
		t.Fatal("name accepted in place of exact ID")
	}
	profile, err := switchModelProfile(model)
	if err != nil || profile["api_key"] != "other-private-key" || profile["http_max_retries"] != float64(0) {
		t.Fatal("switch lost credentials or replay policy", err)
	}
	_ = os.WriteFile(filepath.Join(dir, "models/duplicate.yaml"), []byte("id: second\nname: Duplicate\nmodel_id: model-three\n"), 0600)
	if _, err = ListLocalModels(dir); err == nil {
		t.Fatal("duplicate ID accepted")
	}
	if _, err = LoadLocalModelByID(dir, "second"); err == nil {
		t.Fatal("ambiguous selection accepted")
	}
}

func TestConversationModelSwitchPreservesHistoryAndCommitsOnlyOnSuccess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	config := testConversationConfig(t)
	model := LocalModel{Profile: `{"id":"opscopilot","name":"OpsCopilot","model_id":"one"}`, Environment: `{}`, ID: "first", Name: "First", ModelID: "one"}
	config.LocalModel = model
	c := &Conversation{}
	defer c.Close()
	coordinator := &assistantcontrol.Coordinator{}
	first, _ := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
	if err := c.Run(ctx, config, first, &managedFixtureTools{}, "remember-me"); err != nil {
		t.Fatal(err)
	}
	first.Finish("completed", "")
	client, session := c.client, c.sessionID
	model.ID, model.Name, model.ModelID, model.Profile = "second", "Second", "two", `{"id":"opscopilot","name":"OpsCopilot","model_id":"two"}`
	if err := c.SwitchLocalModel(ctx, model); err != nil {
		t.Fatal(err)
	}
	if c.client != client || c.sessionID != session || c.config.LocalModel != model {
		t.Fatal("switch replaced session or did not commit")
	}
	config.LocalModel = model
	second, _ := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
	if err := c.Run(ctx, config, second, &managedFixtureTools{}, "recall"); err != nil {
		t.Fatal(err)
	}
	second.Finish("completed", "")
	if second.Snapshot().Output != "remember-me|recall" {
		t.Fatal("history lost", second.Snapshot())
	}
	invalid := model
	invalid.Profile = `{"id":"opscopilot","name":"OpsCopilot","model_id":"reject-switch"}`
	if err := c.SwitchLocalModel(ctx, invalid); err == nil || c.config.LocalModel != model || c.closed {
		t.Fatal("rejected model committed or broke existing session", err)
	}
	c.mu.Lock()
	err := c.SwitchLocalModel(ctx, model)
	c.mu.Unlock()
	if err != assistantcontrol.ErrBusy {
		t.Fatal("switch admitted during execution", err)
	}
}

func TestRecoveredConversationSuppliesHistoryOnceAndNoToolAuthority(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	config := testConversationConfig(t)
	c := NewRecoveredConversation("saved-history-reference")
	defer c.Close()
	coordinator := &assistantcontrol.Coordinator{}
	for _, question := range []string{"new request", "follow-up"} {
		turn, _ := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
		if err := c.Run(ctx, config, turn, &managedFixtureTools{}, question); err != nil {
			t.Fatal(err)
		}
		turn.Finish("completed", "")
		if strings.Count(turn.Snapshot().Output, "saved-history-reference") != 1 {
			t.Fatal("history missing or replayed", turn.Snapshot())
		}
	}
	if _, err := c.router.Call(ctx, "terminal_execute", "pwd"); err != assistantcontrol.ErrStale {
		t.Fatal("idle restored context retained authority", err)
	}
}
