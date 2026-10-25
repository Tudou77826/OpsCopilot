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
	"opscopilot/pkg/assistanttools"
	"opscopilot/pkg/knowledge"
	"opscopilot/pkg/terminalcontrol"
)

// Real iCode and MCP transport with a deterministic local model; no credentials,
// user knowledge, server connections or native shell execution are involved.
func TestLiveICodeApplicationReferences(t *testing.T) {
	executable := os.Getenv("OPSCOPILOT_CHRYS_TEST_EXE")
	if executable == "" {
		t.Skip("opt-in local iCode + deterministic model")
	}
	dir := t.TempDir()
	text := "---\nservice: payments\nmodule: api\n---\n\n## 场景：超时\n- **关键词**: E215\n\nREFERENCE_BODY_MARKER\n"
	if err := os.WriteFile(filepath.Join(dir, "experience.md"), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	id := knowledge.ScenarioID("payments", "api", "超时")
	names := []string{"knowledge_list", "knowledge_search", "knowledge_read", "quick_command_list", "quick_command_read"}
	arguments := []string{`{}`, `{"query":"E215"}`, `{"id":"` + id + `"}`, `{}`, `{"kind":"assistant","id":"inspect"}`}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Tools []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
			Messages []json.RawMessage `json:"messages"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			t.Error("invalid model request")
			w.WriteHeader(400)
			return
		}
		if serveReviewFixture(w, request) {
			return
		}
		step := int(calls.Add(1)) - 1
		if step > len(names) {
			t.Error("unexpected model retry")
			w.WriteHeader(400)
			return
		}
		all, _ := json.Marshal(request.Messages)
		if step >= 3 && !strings.Contains(string(all), "REFERENCE_BODY_MARKER") {
			t.Error("knowledge body did not reach model")
		}
		message := map[string]any{"role": "assistant", "content": "REFERENCES_CHAIN_OK"}
		reason := "stop"
		if step < len(names) {
			tool := ""
			for _, item := range request.Tools {
				if item.Function.Name == names[step] || strings.HasSuffix(item.Function.Name, "_"+names[step]) {
					tool = item.Function.Name
					break
				}
			}
			if tool == "" {
				t.Error("reference tool missing from real iCode model tools", names[step])
				w.WriteHeader(400)
				return
			}
			message = map[string]any{"role": "assistant", "content": nil, "tool_calls": []any{map[string]any{"id": "reference-" + names[step], "type": "function", "function": map[string]string{"name": tool, "arguments": arguments[step]}}}}
			reason = "tool_calls"
		} else if !strings.Contains(string(all), "COMMAND_BODY_MARKER") {
			t.Error("saved command body did not reach model")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "reference-fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": reason}}, "usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20}})
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()
	c := &assistantcontrol.Coordinator{}
	turn, _ := c.Begin(ctx, assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	tools := assistanttools.NewWorkspaceTools(turn, terminalcontrol.Target{}, nil, nil, nil, func(target terminalcontrol.Target) *assistanttools.TerminalTools {
		return &assistanttools.TerminalTools{Turn: turn}
	})
	defer tools.Close()
	tools.References = &assistanttools.ReferenceTools{Turn: turn, KnowledgeDirectory: func() string { return dir }, Commands: func() ([]assistanttools.ReferenceCommand, error) {
		return []assistanttools.ReferenceCommand{{Kind: "assistant", ID: "inspect", Name: "inspect", Enabled: true, Content: "COMMAND_BODY_MARKER"}}, nil
	}}
	conversation := &Conversation{}
	defer conversation.Close()
	if err := conversation.Run(ctx, ManagedConfig{Executable: executable, BaseURL: server.URL + "/v1", APIKey: "fixture-key", Model: "fixture", ParentDirectory: t.TempDir()}, turn, tools, "Read existing knowledge and command definitions only."); err != nil {
		t.Fatal(err)
	}
	if usage := conversation.ContextUsage(); !usage.Known || usage.Used <= 0 || usage.Size <= 0 {
		t.Fatalf("real iCode did not report session context usage: %+v", usage)
	}
	if calls.Load() != 6 || turn.Snapshot().Output != "REFERENCES_CHAIN_OK" || turn.Snapshot().Prompt != nil {
		t.Fatalf("reference chain failed: modelCalls=%d output=%s", calls.Load(), turn.Snapshot().Output)
	}
	var receipts int
	for _, message := range turn.Snapshot().Messages {
		if op := message.Operation; message.Kind == "operation" {
			receipts++
			if op.Status != "completed" || !op.Measured {
				t.Fatalf("reference call missing completed measured receipt: %+v", op)
			}
		}
	}
	if receipts != len(names) {
		t.Fatalf("want one receipt per reference call, got %d", receipts)
	}
}
