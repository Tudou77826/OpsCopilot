package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"opscopilot/pkg/agenttransport/acp"
	"opscopilot/pkg/assistantcontrol"
)

func TestAssistantModelsSelectOnlyICodeProfilesWhenIdleAndPreserveHistory(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("CHRYS_MODEL_PROFILE", "")
	dir := acp.LocalModelDirectory()
	if err := os.MkdirAll(filepath.Join(dir, "models"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{
		"settings.yaml":      "model.profile.active: first\n",
		"models/first.yaml":  "id: first\nname: First\nmodel_id: model-one\napi_key: model-secret\nbase_url: https://private.invalid\n",
		"models/second.yaml": "id: second\nname: Second\nmodel_id: model-two\napi_key: other-secret\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	a := newLifecycleApp()
	a.assistantHistory.Begin("old", "keep-this-question")
	options, err := a.AssistantModelOptions()
	if err != nil || len(options.Models) != 2 || options.Current.ID != "first" {
		t.Fatal("wrong default", err)
	}
	a.assistantRunning = true
	if _, err = a.SelectAssistantModel("second"); !errors.Is(err, assistantcontrol.ErrBusy) {
		t.Fatal("switch admitted while running", err)
	}
	a.assistantRunning = false
	turn, _ := a.assistantCoordinator().Begin(context.Background(), assistantcontrol.Target{}, nil)
	if _, err = a.SelectAssistantModel("second"); !errors.Is(err, assistantcontrol.ErrBusy) {
		t.Fatal("switch admitted during coordinator operation", err)
	}
	turn.Finish("completed", "")
	if _, err = a.SelectAssistantModel("Second"); err == nil {
		t.Fatal("accepted non-ID")
	}
	options, err = a.SelectAssistantModel("second")
	if err != nil || options.Current.ID != "second" || options.Current.ModelID != "model-two" {
		t.Fatal("switch failed", err)
	}
	encoded, _ := json.Marshal(options)
	if strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "private.invalid") {
		t.Fatal("private model config exposed")
	}
	if len(a.assistantHistory.Snapshot().Messages) != 1 {
		t.Fatal("switch cleared visible history")
	}
	if err = os.WriteFile(filepath.Join(dir, "models/second.yaml"), []byte("id: second\nname: Second\nmodel_id: model-two-updated\napi_key: refreshed-secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	pinned, err := a.AssistantModelOptions()
	if err != nil || pinned.Current.ModelID != "model-two" {
		t.Fatal("external edits changed the effective snapshot", err)
	}
	if err = a.NewAssistantConversation(); err != nil {
		t.Fatal(err)
	}
	config, err := a.assistantModelConfig("opscopilot")
	if err != nil || config.LocalModel.ID != "second" || config.LocalModel.ModelID != "model-two-updated" {
		t.Fatal("new conversation lost selected iCode model", err)
	}
	settings, _ := os.ReadFile(filepath.Join(dir, "settings.yaml"))
	if string(settings) != "model.profile.active: first\n" {
		t.Fatal("changed global iCode default")
	}
}
