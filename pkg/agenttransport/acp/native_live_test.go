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
	"opscopilot/pkg/assistantskills"
)

// The model is deterministic; Chrys, its native Python runner, permission RPC,
// filesystem snapshots and settings reload are real and remain unmodified.
func TestLiveChrysNativeSkillRunsAndRefreshesWithoutLosingConversation(t *testing.T) {
	executable := os.Getenv("OPSCOPILOT_CHRYS_TEST_EXE")
	if executable == "" {
		t.Skip("opt-in local Chrys")
	}
	root := t.TempDir()
	source := filepath.Join(root, "source")
	os.MkdirAll(filepath.Join(source, "scripts"), 0700)
	body := "---\nname: native-check\ndescription: Native runner verification.\n---\nVERSION_ONE\n"
	os.WriteFile(filepath.Join(source, "SKILL.md"), []byte(body), 0600)
	marker := filepath.Join(root, "executed.txt")
	os.WriteFile(filepath.Join(source, "scripts", "lookup.py"), []byte("from pathlib import Path\nimport sys\nPath(sys.argv[1]).write_text('EXECUTED')\nprint('NATIVE_PYTHON_RESULT')\n"), 0600)
	library := assistantskills.Library{Directory: filepath.Join(root, "skills")}
	if err := library.Import(source, false); err != nil {
		t.Fatal(err)
	}
	var calls, approvals atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if serveReviewFixture(w, request) {
			return
		}
		step := calls.Add(1)
		tool := ""
		args := map[string]any{}
		switch step {
		case 1, 4:
			tool = "load_skill"
			args["skill_name"] = "native-check"
		case 2:
			tool = "run_skill_script"
			args = map[string]any{"skill_name": "native-check", "script_name": "scripts/lookup.py", "arguments": []string{marker}}
		case 3:
			data, _ := json.Marshal(request.Messages)
			if !strings.Contains(string(data), "NATIVE_PYTHON_RESULT") {
				t.Error("native script result missing")
			}
		case 5:
			data, _ := json.Marshal(request.Messages)
			if !strings.Contains(string(data), "FIRST_TURN_REFERENCE") {
				t.Error("conversation lost during refresh")
			}
			last := ""
			for _, m := range request.Messages {
				if m.Role == "tool" {
					last = string(m.Content)
				}
			}
			if !strings.Contains(last, "VERSION_TWO") {
				t.Error("updated Skill was not loaded", last)
			}
		default:
			t.Error("unexpected model request", step)
		}
		message := map[string]any{"role": "assistant", "content": "NATIVE_OK"}
		reason := "stop"
		if tool != "" {
			b, _ := json.Marshal(args)
			message = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": tool, "type": "function", "function": map[string]string{"name": tool, "arguments": string(b)}}}}
			reason = "tool_calls"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "created": 1, "model": "fixture", "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": reason}}, "usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20}})
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	coordinator := &assistantcontrol.Coordinator{}
	go func() {
		for ctx.Err() == nil {
			s := coordinator.Snapshot()
			if s.Prompt != nil {
				if err := coordinator.Respond(s.TurnID, s.Prompt.ID, assistantcontrol.Answer{Selected: []string{"allow"}}); err == nil {
					approvals.Add(1)
				}
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	conversation := &Conversation{}
	defer conversation.Close()
	config := ManagedConfig{Executable: executable, BaseURL: server.URL + "/v1", APIKey: "fixture", Model: "fixture", ParentDirectory: root, SkillsDirectory: library.Directory, AuditDirectory: filepath.Join(root, "audit")}
	first, _ := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
	if err := conversation.Run(ctx, config, first, &managedFixtureTools{}, "FIRST_TURN_REFERENCE run native-check"); err != nil {
		t.Fatal(err)
	}
	first.Finish("completed", "")
	visibleExecution := false
	for _, message := range first.Snapshot().Messages {
		if message.Operation.Kind == "local" && strings.Contains(message.Operation.Result, "NATIVE_PYTHON_RESULT") {
			visibleExecution = true
		}
	}
	if !visibleExecution {
		t.Fatal("native script output missing from display feed", first.Snapshot().Messages)
	}
	data, err := os.ReadFile(marker)
	if err != nil || string(data) != "EXECUTED" || approvals.Load() != 1 {
		t.Fatal("native execution/approval missing", string(data), err, approvals.Load())
	}
	client, session := conversation.client, conversation.sessionID
	os.WriteFile(filepath.Join(source, "SKILL.md"), []byte(strings.ReplaceAll(body, "VERSION_ONE", "VERSION_TWO")), 0600)
	if err = library.ImportAs(source, true, "native-check"); err != nil {
		t.Fatal(err)
	}
	second, _ := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
	defer second.Finish("completed", "")
	if err = conversation.Run(ctx, config, second, &managedFixtureTools{}, "Refresh native-check and recall FIRST_TURN_REFERENCE"); err != nil {
		t.Fatal(err)
	}
	if conversation.client != client || conversation.sessionID != session || calls.Load() != 5 || second.Snapshot().Output != "NATIVE_OK" {
		t.Fatal("session was not preserved", calls.Load(), second.Snapshot())
	}
	audit, err := os.ReadFile(filepath.Join(config.AuditDirectory, "native.jsonl"))
	if err != nil || !strings.Contains(string(audit), `"status":"allowed"`) || strings.Contains(string(audit), marker) {
		t.Fatal("native audit missing or leaked arguments", err)
	}
}
