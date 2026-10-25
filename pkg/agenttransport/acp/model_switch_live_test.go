package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"opscopilot/pkg/assistantcontrol"
)

// Real iCode/ACP and model rebuild; only the two HTTP model services are fixtures.
func TestLiveICodeModelSwitchKeepsSessionHistoryAndSeparateCredentials(t *testing.T) {
	executable := os.Getenv("OPSCOPILOT_CHRYS_TEST_EXE")
	if executable == "" {
		t.Skip("opt-in local iCode")
	}
	t.Setenv("CHRYS_MODEL_PROFILE", "")
	var mu sync.Mutex
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Model    string `json:"model"`
			Messages []struct {
				Content any `json:"content"`
			} `json:"messages"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			t.Error("invalid model request")
			return
		}
		key := "first-fixture-key"
		if request.Model == "two" {
			key = "second-fixture-key"
		}
		if r.Header.Get("Authorization") != "Bearer "+key {
			t.Error("model switch lost distinct credentials")
		}
		mu.Lock()
		seen = append(seen, request.Model)
		n := len(seen)
		mu.Unlock()
		if n > 1 {
			data, _ := json.Marshal(request.Messages)
			if !strings.Contains(string(data), "remember-switch-reference") {
				t.Error("model switch lost prior prompt context")
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "model": request.Model, "choices": []any{map[string]any{"index": 0, "message": map[string]string{"role": "assistant", "content": "MODEL_" + request.Model}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 5, "completion_tokens": 5, "total_tokens": 10}})
	}))
	defer server.Close()
	profile := func(id, model, key string) string {
		return fmt.Sprintf("id: %s\nname: %s\nprovider: openai\nmodel_id: %s\nstream: false\nbase_url: %s/v1\napi_key: '{{%s}}'\n", id, id, model, server.URL, key)
	}
	dir := localModelFixture(t, profile("first", "one", "MODEL_FIRST_TEST_KEY"), "model.profile.active: first\n", "MODEL_FIRST_TEST_KEY=first-fixture-key\nMODEL_SECOND_TEST_KEY=second-fixture-key\n")
	if err := os.WriteFile(filepath.Join(dir, "models/second.yaml"), []byte(profile("second", "two", "MODEL_SECOND_TEST_KEY")), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := LoadLocalModel(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadLocalModelByID(dir, "second")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
	defer cancel()
	c := &Conversation{}
	defer c.Close()
	coordinator := &assistantcontrol.Coordinator{}
	config := ManagedConfig{Executable: executable, LocalModel: first}
	run := func(question, want string) {
		t.Helper()
		turn, err := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer turn.Finish("completed", "")
		if err = c.Run(ctx, config, turn, &managedFixtureTools{}, question); err != nil {
			t.Fatal(err)
		}
		if turn.Snapshot().Output != want {
			t.Fatalf("unexpected response %q", turn.Snapshot().Output)
		}
	}
	run("remember-switch-reference", "MODEL_one")
	session, client := c.sessionID, c.client
	if err = c.SwitchLocalModel(ctx, second); err != nil {
		t.Fatal(err)
	}
	config.LocalModel = second
	run("recall reference", "MODEL_two")
	// Skill refresh also reloads settings; the per-session model must stay pinned.
	if err = c.client.callExtension(ctx, "settings/reload", map[string]string{"sessionId": session}, nil); err != nil {
		t.Fatal(err)
	}
	run("recall again", "MODEL_two")
	if c.sessionID != session || c.client != client {
		t.Fatal("ACP process or session replaced")
	}
	settings, _ := os.ReadFile(filepath.Join(dir, "settings.yaml"))
	if string(settings) != "model.profile.active: first\n" {
		t.Fatal("native default changed")
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(seen, ",") != "one,two,two" {
		t.Fatal("unexpected model requests", seen)
	}
}
