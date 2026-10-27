package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"opscopilot/pkg/assistantcontrol"
)

// Uses the actual iCode process and ACP/MCP transport. Only the model and
// remote tool are simulated; the risk command is never sent to a server.
func TestLiveICodeAutoReviewsOpsToolAndForwardsRejection(t *testing.T) {
	executable := os.Getenv("OPSCOPILOT_CHRYS_TEST_EXE")
	if executable == "" {
		t.Skip("opt-in local iCode")
	}
	var phase, calls, judgments atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []json.RawMessage `json:"messages"`
			Tools    []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
		}
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			w.WriteHeader(400)
			return
		}
		body, _ := json.Marshal(req.Messages)
		w.Header().Set("Content-Type", "application/json")
		if bytes.Contains(bytes.ToLower(body), []byte("auto-approval assistant")) {
			judgments.Add(1)
			if phase.Load() == 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			v, _ := json.Marshal(map[string]any{"approved": phase.Load() == 1, "reason": "fixture judge decision"})
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "judge", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "message": map[string]string{"role": "assistant", "content": string(v)}, "finish_reason": "stop"}}})
			return
		}
		step := calls.Add(1)
		message := map[string]any{"role": "assistant", "content": "DONE"}
		finish := "stop"
		if step == 1 || step == 3 || step == 5 {
			name := ""
			for _, tool := range req.Tools {
				if tool.Function.Name == "terminal_execute" || strings.HasSuffix(tool.Function.Name, "_terminal_execute") {
					name = tool.Function.Name
					break
				}
			}
			if name == "" {
				t.Error("terminal tool missing")
				w.WriteHeader(500)
				return
			}
			command := "ls /tmp"
			if phase.Load() == 2 {
				command = "rm /tmp/fixture-only"
			}
			args, _ := json.Marshal(map[string]string{"command": command, "purpose": "isolated test"})
			message = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "fixture", "type": "function", "function": map[string]string{"name": name, "arguments": string(args)}}}}
			finish = "tool_calls"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "model", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}}})
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	coordinator := &assistantcontrol.Coordinator{}
	conversation := &Conversation{}
	defer conversation.Close()
	tools := &managedFixtureTools{}
	config := ManagedConfig{Executable: executable, BaseURL: server.URL + "/v1", APIKey: "fixture", Model: "fixture", ParentDirectory: t.TempDir()}
	phase.Store(1)
	first, _ := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
	if err := conversation.Run(ctx, config, first, tools, "列出目录"); err != nil {
		t.Fatal(err)
	}
	if tools.calls.Load() != 1 || judgments.Load() != 1 || coordinator.Snapshot().Prompt != nil {
		t.Fatal("ordinary call was not automatically reviewed and executed")
	}
	first.Finish("completed", "")
	phase.Store(2)
	second, _ := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
	defer second.Finish("completed", "")
	done := make(chan error, 1)
	go func() { done <- conversation.Run(ctx, config, second, tools, "仅查看目录") }()
	deadline := time.Now().Add(30 * time.Second)
	for coordinator.Snapshot().Prompt == nil && time.Now().Before(deadline) {
		select {
		case err := <-done:
			t.Fatalf("request ended without human confirmation: %v", err)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	p := coordinator.Snapshot().Prompt
	if p == nil || tools.calls.Load() != 1 {
		t.Fatal("risk call was dispatched before confirmation")
	}
	if err := coordinator.Respond(second.ID(), p.ID, assistantcontrol.Answer{Selected: []string{"reject"}}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if judgments.Load() != 2 || tools.calls.Load() != 1 {
		t.Fatal("rejected call reached the host", judgments.Load(), tools.calls.Load())
	}
	second.Finish("completed", "")
	phase.Store(3)
	third, _ := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
	defer third.Finish("completed", "")
	done = make(chan error, 1)
	go func() { done <- conversation.Run(ctx, config, third, tools, "查看目录") }()
	deadline = time.Now().Add(30 * time.Second)
	for coordinator.Snapshot().Prompt == nil && time.Now().Before(deadline) {
		select {
		case err := <-done:
			t.Fatalf("judge failure ended without human confirmation: %v", err)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	p = coordinator.Snapshot().Prompt
	if p == nil || tools.calls.Load() != 1 {
		t.Fatal("judge failure dispatched without human confirmation")
	}
	if err := coordinator.Respond(third.ID(), p.ID, assistantcontrol.Answer{Selected: []string{"reject"}}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if judgments.Load() < 3 || tools.calls.Load() != 1 {
		t.Fatal("judge failure bypassed review", judgments.Load(), tools.calls.Load())
	}
}
