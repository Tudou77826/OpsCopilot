package acp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"opscopilot/pkg/assistantcontrol"
)

// Real Chrys owns history and cancellation. Only model responses are simulated;
// checking actual model requests proves this is not a frontend history replay.
func TestLiveChrysConversationContinuity(t *testing.T) {
	executable := os.Getenv("OPSCOPILOT_CHRYS_TEST_EXE")
	if executable == "" {
		t.Skip("opt-in real Chrys + local model fixture")
	}
	waiting := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			w.WriteHeader(400)
			return
		}
		last := ""
		hasNonce := false
		for _, message := range request.Messages {
			if message.Role == "user" && !strings.Contains(string(message.Content), "<system-reminder>") {
				last = string(message.Content)
				hasNonce = hasNonce || strings.Contains(last, "nonce-8c70a4")
			}
		}
		answer := "NO_PRIOR_CONTEXT"
		switch {
		case strings.Contains(last, "WAIT_FOR_CANCEL"):
			select {
			case waiting <- struct{}{}:
			default:
			}
			select {
			case <-r.Context().Done():
				return
			case <-time.After(15 * time.Second):
				answer = "LATE"
			}
		case strings.Contains(last, "AFTER_CANCEL") && hasNonce:
			answer = "NATIVE_AFTER_CANCEL_OK"
		case strings.Contains(last, "RECALL") && hasNonce:
			answer = "NATIVE_CONTEXT_OK"
		case strings.Contains(last, "nonce-8c70a4"):
			answer = "REMEMBERED"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "created": 1, "model": "fixture", "choices": []any{map[string]any{"index": 0, "message": map[string]string{"role": "assistant", "content": answer}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20}})
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	config := ManagedConfig{Executable: executable, BaseURL: server.URL + "/v1", APIKey: "fixture-only", Model: "fixture", ParentDirectory: t.TempDir()}
	c := &Conversation{}
	defer c.Close()
	coordinator := &assistantcontrol.Coordinator{}
	ask := func(c *Conversation, question string) string {
		t.Helper()
		turn, err := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer turn.Finish("completed", "")
		if err := c.Run(ctx, config, turn, &managedFixtureTools{}, question); err != nil {
			var rpc *RPCError
			if errors.As(err, &rpc) {
				t.Logf("fixture-only %s: code=%d data=%s", question, rpc.Code, rpc.Data)
			}
			t.Fatal(err)
		}
		return turn.Snapshot().Output
	}
	if answer := ask(c, "Remember nonce-8c70a4"); answer != "REMEMBERED" {
		t.Fatal(answer)
	}
	session, client := c.sessionID, c.client
	if answer := ask(c, "RECALL"); answer != "NATIVE_CONTEXT_OK" {
		t.Fatal(answer)
	}
	turn, _ := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx, config, turn, &managedFixtureTools{}, "WAIT_FOR_CANCEL") }()
	select {
	case <-waiting:
	case <-ctx.Done():
		t.Fatal("cancel fixture never reached model")
	}
	_ = coordinator.Stop(turn.ID())
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("cancel did not return")
	}
	if answer := ask(c, "AFTER_CANCEL"); answer != "NATIVE_AFTER_CANCEL_OK" {
		t.Fatal(answer)
	}
	if c.sessionID != session || c.client != client {
		t.Fatal("native session replaced between prompts")
	}
	c.Close()
	fresh := &Conversation{}
	defer fresh.Close()
	if answer := ask(fresh, "RECALL"); answer != "NO_PRIOR_CONTEXT" {
		t.Fatal("new chat leaked old context", answer)
	}
}
