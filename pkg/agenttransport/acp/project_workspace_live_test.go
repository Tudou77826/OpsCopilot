package acp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"opscopilot/pkg/assistantcontrol"
)

// Real iCode and ACP, with a local deterministic model endpoint.
func TestLiveProjectWorkspaceAndSwitch(t *testing.T) {
	exe := os.Getenv("OPSCOPILOT_CHRYS_TEST_EXE")
	if exe == "" {
		t.Skip("opt-in iCode")
	}
	root := t.TempDir()
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	for _, d := range []string{a, b} {
		if err := os.MkdirAll(d, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(a, "AGENTS.md"), []byte("WORKSPACE_RULE_ALPHA"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b, "AGENTS.md"), []byte("WORKSPACE_RULE_BETA"), 0600); err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(a, ".agents", "skills", "workspace-sample")
	if err := os.MkdirAll(skillDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: workspace-sample\ndescription: WORKSPACE_SKILL_DESCRIPTION\n---\nInstructions\n"), 0600); err != nil {
		t.Fatal(err)
	}
	requests := make(chan string, 10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			w.WriteHeader(400)
			return
		}
		b, _ := json.Marshal(request)
		requests <- string(b)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "created": 1, "model": "fixture", "choices": []any{map[string]any{"index": 0, "message": map[string]string{"role": "assistant", "content": "OK"}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20}})
	}))
	defer srv.Close()
	jsonDirs := func(v ...string) string { b, _ := json.Marshal(v); return string(b) }
	cfg := ManagedConfig{Executable: exe, BaseURL: srv.URL + "/v1", APIKey: "fixture", Model: "fixture", ParentDirectory: root, WorkspaceJSON: jsonDirs(a), ProjectSkillsEnabled: true}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	co := &assistantcontrol.Coordinator{}
	run := func(c *Conversation, config ManagedConfig, question string) string {
		t.Helper()
		turn, err := co.Begin(ctx, assistantcontrol.Target{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		err = c.Run(ctx, config, turn, &managedFixtureTools{}, question)
		turn.Finish("completed", "")
		if err != nil {
			t.Fatal(err)
		}
		select {
		case req := <-requests:
			return req
		case <-ctx.Done():
			t.Fatal(ctx.Err())
			return ""
		}
	}
	c := &Conversation{}
	defer c.Close()
	first := run(c, cfg, "FIRST_MARKER")
	if !strings.Contains(first, "WORKSPACE_RULE_ALPHA") {
		t.Fatal("primary AGENTS.md missing from model request")
	}
	if !strings.Contains(first, "WORKSPACE_SKILL_DESCRIPTION") {
		t.Fatal("trusted project skill not made available")
	}
	sw, err := c.SwitchWorkspace(ctx, jsonDirs(b))
	if err != nil || !sw {
		t.Fatalf("workspace switch: %v %v", sw, err)
	}
	cfg.WorkspaceJSON = jsonDirs(b)
	second := run(c, cfg, "SECOND_MARKER")
	if !strings.Contains(second, "WORKSPACE_RULE_BETA") || !strings.Contains(second, "FIRST_MARKER") {
		t.Fatal("switch lost project rule or conversation history")
	}
	multi := &Conversation{}
	defer multi.Close()
	cfg.WorkspaceJSON = jsonDirs(a, b)
	third := run(multi, cfg, "MULTI_MARKER")
	if !strings.Contains(third, "WORKSPACE_RULE_ALPHA") {
		t.Fatal("multi workspace missing primary rule")
	}
	// The runtime's workspace snapshot must contain both roots; model formatting
	// can escape path separators, so compare JSON-encoded paths.
	escapedA, _ := json.Marshal(a)
	escapedB, _ := json.Marshal(b)
	if !strings.Contains(third, string(escapedA)[1:len(escapedA)-1]) || !strings.Contains(third, string(escapedB)[1:len(escapedB)-1]) {
		t.Fatal("model context missing workspace roots")
	}
	if switched, err := multi.SwitchWorkspace(ctx, jsonDirs(b)); err != nil || switched {
		t.Fatalf("multi-root change must request a new session, got %v %v", switched, err)
	}
}
