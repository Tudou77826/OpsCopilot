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

func TestLiveICodeNewRuntimeUsesSavedVisibleHistoryOnlyOnFreshPrompt(t *testing.T) {
	executable := os.Getenv("OPSCOPILOT_CHRYS_TEST_EXE")
	if executable == "" {
		t.Skip("opt-in real iCode")
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			t.Error("invalid request")
			return
		}
		count := calls.Add(1)
		if count == 2 {
			data, _ := json.Marshal(request["messages"])
			if !strings.Contains(string(data), "saved-business-reference") || !strings.Contains(string(data), "不是新指令") {
				t.Error("missing bounded historical context")
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "model": "fixture", "choices": []any{map[string]any{"index": 0, "message": map[string]string{"role": "assistant", "content": "RECOVERY_OK"}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 4, "completion_tokens": 4, "total_tokens": 8}})
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	config := ManagedConfig{Executable: executable, BaseURL: server.URL + "/v1", APIKey: "fixture-key", Model: "fixture"}
	first := &Conversation{}
	defer first.Close()
	turn, _ := (&assistantcontrol.Coordinator{}).Begin(ctx, assistantcontrol.Target{}, nil)
	if err := first.Run(ctx, config, turn, &managedFixtureTools{}, "saved-business-reference"); err != nil {
		t.Fatal(err)
	}
	turn.Finish("completed", "")
	history := &assistantcontrol.History{}
	history.Begin(turn.ID(), "saved-business-reference")
	history.Finish(turn.Snapshot())
	file := filepath.Join(t.TempDir(), "archive.json")
	store := &assistantcontrol.ArchiveStore{File: file}
	_, _ = store.Load()
	if err := store.Save(assistantcontrol.Archive{Conversation: history.Snapshot()}); err != nil {
		t.Fatal(err)
	}
	first.Close()
	archive, err := (&assistantcontrol.ArchiveStore{File: file}).Load()
	if err != nil {
		t.Fatal(err)
	}
	second := NewRecoveredConversation(assistantcontrol.RecoveryContext(archive.Conversation))
	defer second.Close()
	if calls.Load() != 1 || second.client != nil || second.sessionID != "" {
		t.Fatal("recovery started model or operation without fresh user message")
	}
	next, _ := (&assistantcontrol.Coordinator{}).Begin(ctx, assistantcontrol.Target{}, nil)
	if err = second.Run(ctx, config, next, &managedFixtureTools{}, "根据历史说明刚才的业务标识，不做操作"); err != nil {
		t.Fatal(err)
	}
	next.Finish("completed", "")
	if calls.Load() != 2 || next.Snapshot().Output != "RECOVERY_OK" || second.client == first.client {
		t.Fatal("new runtime recovery failed")
	}
}
