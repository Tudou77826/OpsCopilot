package acp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"opscopilot/pkg/assistantcontrol"
)

func TestLiveChrysSkillResourceTraversalDeniedAndScriptRequiresApproval(t *testing.T) {
	executable := os.Getenv("OPSCOPILOT_CHRYS_TEST_EXE")
	if executable == "" {
		t.Skip("opt-in local Chrys + simulated model")
	}
	root := t.TempDir()
	skills := filepath.Join(root, "skills")
	if err := os.MkdirAll(filepath.Join(skills, "order-logs", "scripts"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(skills, "order-logs", "references"), 0700); err != nil {
		t.Fatal(err)
	}
	write := func(filename, content string) {
		t.Helper()
		if err := os.WriteFile(filename, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(skills, "order-logs", "SKILL.md"), "---\nname: order-logs\ndescription: Order log troubleshooting.\n---\nSKILL_BODY_MARKER Read references/deployment.md.\n")
	write(filepath.Join(skills, "order-logs", "references", "deployment.md"), "RESOURCE_BODY_MARKER service=orders-reader")
	outside := filepath.Join(root, "private.txt")
	write(outside, "OUTSIDE_SECRET_MARKER")
	marker := filepath.Join(root, "script-ran")
	write(filepath.Join(skills, "order-logs", "scripts", "escape.py"), "from pathlib import Path\nPath("+pythonQuotedPath(marker)+").touch()\n")
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []json.RawMessage `json:"messages"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			t.Error("invalid model request")
			w.WriteHeader(400)
			return
		}
		all, _ := json.Marshal(request.Messages)
		if serveReviewFixture(w, request) {
			return
		}
		step := calls.Add(1)
		if strings.Contains(string(all), "OUTSIDE_SECRET_MARKER") {
			t.Error("arbitrary local file exposed")
		}
		if step == 1 && (!strings.Contains(string(all), "order-logs") || strings.Contains(string(all), "SKILL_BODY_MARKER")) {
			t.Error("catalog did not progressively disclose skill")
		}
		if step == 2 && !strings.Contains(string(all), "SKILL_BODY_MARKER") {
			t.Error("native load_skill body missing")
		}
		if step == 2 && strings.Contains(string(all), "RESOURCE_BODY_MARKER") {
			t.Error("resource eagerly injected")
		}
		if step >= 3 && !strings.Contains(string(all), "RESOURCE_BODY_MARKER") {
			t.Error("native resource missing")
		}
		var tool string
		var args any
		switch step {
		case 1:
			tool, args = "load_skill", map[string]string{"skill_name": "order-logs"}
		case 2:
			tool, args = "read_skill_resource", map[string]string{"skill_name": "order-logs", "resource_name": "references/deployment.md"}
		case 3:
			tool, args = "read_skill_resource", map[string]string{"skill_name": "order-logs", "resource_name": outside}
		case 4:
			tool, args = "run_skill_script", map[string]string{"skill_name": "order-logs", "script_name": "scripts/escape.py"}
		}
		message := map[string]any{"role": "assistant", "content": "SKILLS_BOUNDARY_OK"}
		reason := "stop"
		if tool != "" {
			data, _ := json.Marshal(args)
			message = map[string]any{"role": "assistant", "content": nil, "tool_calls": []any{map[string]any{"id": tool, "type": "function", "function": map[string]string{"name": tool, "arguments": string(data)}}}}
			reason = "tool_calls"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "created": 1, "model": "fixture", "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": reason}}, "usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20}})
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	coordinator := &assistantcontrol.Coordinator{}
	turn, _ := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	go func() {
		for ctx.Err() == nil {
			state := coordinator.Snapshot()
			if state.Prompt != nil {
				_ = coordinator.Respond(state.TurnID, state.Prompt.ID, assistantcontrol.Answer{Selected: []string{"reject"}})
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	tools := &managedFixtureTools{}
	err := RunManaged(ctx, ManagedConfig{Executable: executable, BaseURL: server.URL + "/v1", APIKey: "fixture", Model: "fixture", SkillsDirectory: skills, ParentDirectory: root}, turn, tools, "Inspect the order log Skill.")
	if err != nil {
		t.Fatal(err)
	}
	if turn.Snapshot().Output != "SKILLS_BOUNDARY_OK" || turn.Snapshot().Prompt != nil || tools.calls.Load() != 0 || calls.Load() != 5 {
		t.Fatal("skill tool boundary failed", turn.Snapshot(), calls.Load(), tools.calls.Load())
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("skill script was executed")
	}
}
func pythonQuotedPath(path string) string { data, _ := json.Marshal(path); return string(data) }
