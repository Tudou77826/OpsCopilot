package acp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"opscopilot/pkg/assistantcontrol"
)

// Only the spawned test executable enters this minimal ACP peer. No model,
// network service, user profile or business SSH endpoint is involved.
func TestMain(m *testing.M) {
	if os.Getenv("ACP_CONVERSATION_TEST") == "1" && len(os.Args) > 1 && os.Args[1] == "acp" {
		conversationPeer()
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func conversationPeer() {
	decoder, encoder := json.NewDecoder(os.Stdin), json.NewEncoder(os.Stdout)
	var history []string
	var pending json.RawMessage
	ignoreCancel := false
	reply := func(id json.RawMessage, result any) {
		_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	}
	for {
		var message envelope
		if decoder.Decode(&message) != nil {
			return
		}
		switch message.Method {
		case "initialize":
			reply(message.ID, map[string]any{"protocolVersion": 1})
		case "_profiles/models/write", "_profiles/agents/write":
			reply(message.ID, map[string]any{})
		case "session/new":
			reply(message.ID, map[string]any{"sessionId": "conversation-one"})
		case "session/prompt":
			var params struct {
				SessionID string `json:"sessionId"`
				Prompt    []struct {
					Text string `json:"text"`
				} `json:"prompt"`
			}
			_ = json.Unmarshal(message.Params, &params)
			text := params.Prompt[0].Text
			history = append(history, text)
			if strings.HasPrefix(text, "wait") {
				pending = message.ID
				ignoreCancel = text == "wait-ignore"
				_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": params.SessionID, "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": "WAITING"}}}})
				continue
			}
			_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": params.SessionID, "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]string{"type": "text", "text": strings.Join(history, "|")}}}})
			reply(message.ID, map[string]string{"stopReason": "end_turn"})
		case "session/cancel":
			if pending != nil && !ignoreCancel {
				reply(pending, map[string]string{"stopReason": "cancelled"})
				pending = nil
			}
		}
	}
}
func testConversationConfig(t *testing.T) ManagedConfig {
	t.Helper()
	t.Setenv("ACP_CONVERSATION_TEST", "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return ManagedConfig{Executable: executable, BaseURL: "http://127.0.0.1:1/v1", Model: "fixture", ParentDirectory: t.TempDir()}
}
func TestConversationRetainsSessionAcrossTurnsAndRevokesOldBindings(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	config := testConversationConfig(t)
	c := &Conversation{}
	defer c.Close()
	coordinator := &assistantcontrol.Coordinator{}
	tools := &managedFixtureTools{}
	first, _ := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
	if err := c.Run(ctx, config, first, tools, "random-reference"); err != nil {
		t.Fatal(err)
	}
	client, session, directory := c.client, c.sessionID, c.directory
	if _, err := c.router.Call(ctx, "terminal_execute", "pwd"); !errors.Is(err, assistantcontrol.ErrStale) {
		t.Fatal("idle conversation granted authority", err)
	}
	first.Finish("completed", "")
	second, _ := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
	if err := c.Run(ctx, config, second, tools, "recall"); err != nil {
		t.Fatal(err)
	}
	second.Finish("completed", "")
	if c.client != client || c.sessionID != session || c.directory != directory || second.Snapshot().Output != "random-reference|recall" {
		t.Fatal("session/context not reused", second.Snapshot())
	}
	changed := config
	changed.Model = "other"
	third, _ := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
	if err := c.Run(ctx, changed, third, tools, "ignored"); err == nil {
		t.Fatal("silently changed model/context")
	}
	third.Finish("failed", "")
	c.Close()
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatal("conversation directory retained", err)
	}
	fourth, _ := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
	defer fourth.Finish("completed", "")
	if err := c.Run(ctx, config, fourth, tools, "ignored"); err == nil {
		t.Fatal("closed conversation silently restarted")
	}
}
func TestConversationCancellationKeepsContextAndKillsUnresponsiveChild(t *testing.T) {
	for _, scenario := range []string{"wait", "wait-ignore"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			config := testConversationConfig(t)
			c := &Conversation{}
			defer c.Close()
			coordinator := &assistantcontrol.Coordinator{}
			tools := &managedFixtureTools{}
			turn, _ := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
			done := make(chan error, 1)
			go func() { done <- c.Run(ctx, config, turn, tools, scenario) }()
			deadline := time.After(5 * time.Second)
			for turn.Snapshot().Output != "WAITING" {
				select {
				case err := <-done:
					t.Fatal(err)
				case <-deadline:
					t.Fatal("prompt never started")
				default:
					time.Sleep(time.Millisecond)
				}
			}
			_ = coordinator.Stop(turn.ID())
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal("cancel failed to drain")
			}
			if _, err := c.router.Call(ctx, "terminal_context", ""); !errors.Is(err, assistantcontrol.ErrStale) {
				t.Fatal("stopped tool authority survived", err)
			}
			next, _ := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
			defer next.Finish("completed", "")
			err := c.Run(ctx, config, next, tools, "after-stop")
			if scenario == "wait-ignore" {
				if err == nil || !c.closed {
					t.Fatal("unresponsive child reused", err)
				}
				return
			}
			if err != nil || next.Snapshot().Output != "wait|after-stop" {
				t.Fatal("stop lost conversation", next.Snapshot(), err)
			}
		})
	}
}
func TestManagedBindingCannotForwardLateUpdatesRequestsOrTools(t *testing.T) {
	var router managedRouter
	coordinator := &assistantcontrol.Coordinator{}
	ctx := context.Background()
	first, _ := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
	tools := &managedFixtureTools{}
	if err := router.bind(first, tools, "one"); err != nil {
		t.Fatal(err)
	}
	oldHandler, oldTools := router.Capture(), router.CaptureCaller()
	router.clear()
	first.Finish("stopped", "")
	second, _ := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
	defer second.Finish("completed", "")
	if err := router.bind(second, tools, "one"); err != nil {
		t.Fatal(err)
	}
	defer router.clear()
	oldHandler.Notify(ctx, "session/update", json.RawMessage(`{"sessionId":"one","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"STALE"}}}`))
	if _, err := oldHandler.Request(ctx, "chrys/request_input", json.RawMessage(`{"sessionId":"one","question":"stale"}`)); !errors.Is(err, assistantcontrol.ErrStale) {
		t.Fatal(err)
	}
	if _, err := oldTools.Call(ctx, "terminal_execute", "pwd"); !errors.Is(err, assistantcontrol.ErrStale) {
		t.Fatal(err)
	}
	if second.Snapshot().Output != "" || second.Snapshot().Prompt != nil || tools.calls.Load() != 0 {
		t.Fatal("old binding contaminated next turn")
	}
}
