package acp

import (
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

// The iCode runtime and input validation are real; only model responses are fixtures.
func TestLiveICodeFreeTextQuestion(t *testing.T) {
	executable := os.Getenv("OPSCOPILOT_CHRYS_TEST_EXE")
	if executable == "" {
		t.Skip("opt-in local iCode")
	}
	var calls atomic.Int32
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
		message := map[string]any{"role": "assistant", "content": "FREE_TEXT_ACCEPTED"}
		finish := "stop"
		if step == 1 {
			message = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "question", "type": "function", "function": map[string]string{"name": "ask_user", "arguments": `{"questions":[{"question":"Which service?","header":"Service","options":[]}]}`}}}}
			finish = "tool_calls"
		} else {
			lastTool := ""
			for _, m := range request.Messages {
				if m.Role == "tool" {
					lastTool = string(m.Content)
				}
			}
			if step != 2 || !strings.Contains(lastTool, "orders-api-accepted") || strings.Contains(lastTool, "ask_user_no_response") {
				t.Errorf("free text was not accepted: %s", lastTool)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "created": 1, "model": "fixture", "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}}, "usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20}})
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	coordinator := &assistantcontrol.Coordinator{}
	turn, _ := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	done := make(chan error, 1)
	go func() {
		done <- RunManaged(ctx, ManagedConfig{Executable: executable, BaseURL: server.URL + "/v1", APIKey: "fixture", Model: "fixture", ParentDirectory: t.TempDir()}, turn, &managedFixtureTools{}, "Ask which service.")
	}()
	deadline := time.NewTimer(45 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	answered := false
	for !answered {
		select {
		case err := <-done:
			t.Fatal("turn ended before question", err)
		case <-deadline.C:
			t.Fatal("iCode question did not reach host")
		case <-ticker.C:
			if prompt := coordinator.Snapshot().Prompt; prompt != nil {
				if prompt.Kind != "input" || len(prompt.Choices) != 0 {
					t.Fatal(prompt)
				}
				if err := coordinator.Respond(turn.ID(), prompt.ID, assistantcontrol.Answer{Text: "orders-api-accepted"}); err != nil {
					t.Fatal(err)
				}
				answered = true
			}
		}
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if calls.Load() != 2 || turn.Snapshot().Output != "FREE_TEXT_ACCEPTED" {
		t.Fatal(calls.Load(), turn.Snapshot())
	}
}
