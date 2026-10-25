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
	"opscopilot/pkg/assistantreview"
)

// Real ACP permission -> host fast HTTP classifier -> human denial. The
// classifier is deterministic and no proposed file write is allowed to run.
func TestLiveICodePermissionUsesHostFastReviewer(t *testing.T) {
	executable := os.Getenv("OPSCOPILOT_CHRYS_TEST_EXE")
	if executable == "" {
		t.Skip("opt-in iCode")
	}
	var mainCalls, fastCalls, fallbackCalls atomic.Int32
	file := filepath.Join(t.TempDir(), "must-not-exist.txt")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		raw := json.NewDecoder(r.Body)
		_ = raw.Decode(&req)
		message := map[string]any{"role": "assistant", "content": "DENIED_AS_REQUESTED"}
		finish := "stop"
		if req.Model == "fast-model" {
			fastCalls.Add(1)
			message["content"] = "B"
		} else if mainCalls.Add(1) == 1 {
			args, _ := json.Marshal(map[string]string{"path": file, "content": "must not be written"})
			message = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "write-once", "type": "function", "function": map[string]string{"name": "write_file", "arguments": string(args)}}}}
			finish = "tool_calls"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}}})
	}))
	defer server.Close()
	reviewer := assistantreview.Service{Fast: assistantreview.NewFastClassifier(server.URL+"/v1", "fixture", "fast-model"), Fallback: func(context.Context, assistantcontrol.ReviewInput) assistantcontrol.ReviewVerdict {
		fallbackCalls.Add(1)
		return assistantcontrol.ReviewVerdict{Reason: "unexpected fallback"}
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	coordinator := &assistantcontrol.Coordinator{}
	turn, _ := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	conversation := &Conversation{}
	defer conversation.Close()
	done := make(chan error, 1)
	go func() {
		done <- conversation.RunWithReviewer(ctx, ManagedConfig{Executable: executable, BaseURL: server.URL + "/v1", APIKey: "fixture", Model: "main-model", ParentDirectory: t.TempDir()}, turn, &managedFixtureTools{}, "propose a file write", reviewer.Review)
	}()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	answered := false
	for !answered {
		select {
		case err := <-done:
			t.Fatal("ended before human confirmation", err)
		case <-ctx.Done():
			t.Fatal("permission did not arrive")
		case <-ticker.C:
			if prompt := coordinator.Snapshot().Prompt; prompt != nil {
				found := false
				for _, detail := range prompt.Details {
					if detail.Label == "AI 自审" && strings.Contains(detail.Value, "快速模型") {
						found = true
					}
				}
				if !found {
					t.Error("fast review reason was not shown")
				}
				if err := coordinator.Respond(turn.ID(), prompt.ID, assistantcontrol.Answer{Selected: []string{"reject"}}); err != nil {
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
	if fastCalls.Load() != 1 || mainCalls.Load() != 2 || fallbackCalls.Load() != 0 {
		t.Fatal(mainCalls.Load(), fastCalls.Load(), fallbackCalls.Load())
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("denied file write executed")
	}
}
