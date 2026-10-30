package acp

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"testing"
	"time"
)

func TestApprovalModeUsesSessionRPCAndRejectsInvalidMode(t *testing.T) {
	left, right := net.Pipe()
	defer right.Close()
	client := Connect(context.Background(), left, left, nil, nil)
	defer client.Close()
	conversation := &Conversation{client: client, sessionID: "session-1"}
	if err := conversation.SwitchApprovalMode(context.Background(), "invalid"); err == nil {
		t.Fatal("invalid mode accepted")
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		var request envelope
		if err := json.NewDecoder(right).Decode(&request); err != nil {
			return
		}
		var params map[string]string
		_ = json.Unmarshal(request.Params, &params)
		if request.Method != "session/set_mode" || params["sessionId"] != "session-1" || params["modeId"] != "bypass" {
			return
		}
		_ = json.NewEncoder(right).Encode(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(request.ID), "result": map[string]any{}})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := conversation.SwitchApprovalMode(ctx, "bypass"); err != nil {
		t.Fatal(err)
	}
	<-done
	if conversation.config.ApprovalMode != "bypass" {
		t.Fatal("effective mode not recorded")
	}
}

func TestLiveICodeApprovalModeSwitch(t *testing.T) {
	executable := os.Getenv("OPSCOPILOT_CHRYS_TEST_EXE")
	if executable == "" {
		t.Skip("opt-in local iCode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c := &Conversation{}
	defer c.Close()
	config := ManagedConfig{Executable: executable, BaseURL: "http://127.0.0.1:1/v1", Model: "fixture", ParentDirectory: t.TempDir()}
	if err := c.start(ctx, ctx, config); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"manual", "auto", "bypass", "auto"} {
		if err := c.SwitchApprovalMode(ctx, mode); err != nil {
			t.Fatalf("switch %s: %v", mode, err)
		}
		if c.config.ApprovalMode != mode {
			t.Fatalf("wrong effective mode: %s", c.config.ApprovalMode)
		}
	}
}
