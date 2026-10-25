package acp

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"opscopilot/pkg/assistantcontrol"
)

func awaitInteraction(t *testing.T, c *assistantcontrol.Coordinator) assistantcontrol.State {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		state := c.Snapshot()
		if state.Prompt != nil {
			return state
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("interaction missing")
	return assistantcontrol.State{}
}
func TestClientReverseQuestionsPermissionsAndUpdatesWhilePromptPending(t *testing.T) {
	left, right := net.Pipe()
	defer right.Close()
	coordinator := &assistantcontrol.Coordinator{}
	turn, _ := coordinator.Begin(context.Background(), assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	updates := make(chan struct{}, 1)
	bridge := &Interactions{Turn: turn, SessionID: "s1", OnUpdate: func(_ string, _ json.RawMessage) { updates <- struct{}{} }}
	client := Connect(context.Background(), left, left, bridge, nil)
	defer client.Close()
	bridge.Bind(client)
	serverDone := make(chan error, 1)
	go func() {
		decoder, encoder := json.NewDecoder(right), json.NewEncoder(right)
		var initialize envelope
		if err := decoder.Decode(&initialize); err != nil {
			serverDone <- err
			return
		}
		_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(initialize.ID), "result": map[string]any{"protocolVersion": 1, "agentInfo": map[string]string{"name": "fixture", "version": "1"}}})
		var prompt envelope
		if err := decoder.Decode(&prompt); err != nil {
			serverDone <- err
			return
		}
		_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": "input-1", "method": "_chrys/request_input", "params": map[string]any{"sessionId": "s1", "questions": []any{map[string]any{"question": "哪些节点？", "multiSelect": true, "options": []any{map[string]string{"label": "A"}, map[string]string{"label": "B"}}}}}})
		_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "s1", "update": map[string]string{"sessionUpdate": "agent_message_chunk"}}})
		var input envelope
		if err := decoder.Decode(&input); err != nil {
			serverDone <- err
			return
		}
		if !strings.Contains(string(input.Result), `"values":["A","B"]`) {
			serverDone <- errors.New("structured answer was lost")
			return
		}
		_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "session/request_permission", "params": map[string]any{"sessionId": "s1", "toolCall": map[string]any{"title": "查看日志", "rawInput": map[string]string{"path": "/var/log/app.log"}}, "options": []any{map[string]string{"optionId": "one", "kind": "allow_once"}, map[string]string{"optionId": "no", "kind": "reject_once"}}}})
		var permission envelope
		if err := decoder.Decode(&permission); err != nil {
			serverDone <- err
			return
		}
		if !strings.Contains(string(permission.Result), `"optionId":"one"`) {
			serverDone <- errors.New("wrong permission option")
			return
		}
		_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(prompt.ID), "result": map[string]string{"stopReason": "end_turn"}})
		serverDone <- nil
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if result, err := client.Initialize(ctx); err != nil || result.AgentInfo.Name != "fixture" {
		t.Fatal(result, err)
	}
	done := make(chan error, 1)
	go func() {
		var result any
		done <- client.Call(ctx, "session/prompt", map[string]any{"sessionId": "s1", "prompt": []any{}}, &result)
	}()
	state := awaitInteraction(t, coordinator)
	select {
	case <-updates:
	case <-ctx.Done():
		t.Fatal("reader blocked behind question")
	}
	if err := coordinator.Respond(state.TurnID, state.Prompt.ID, assistantcontrol.Answer{Selected: []string{"0", "1"}, Text: "今天"}); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(time.Second); (state.Prompt == nil || state.Prompt.Kind != "approval") && time.Now().Before(deadline); {
		time.Sleep(time.Millisecond)
		state = coordinator.Snapshot()
		if state.Prompt == nil {
			continue
		}
	}
	if state.Prompt == nil || state.Prompt.Kind != "approval" {
		t.Fatal(state)
	}
	if err := coordinator.Respond(state.TurnID, state.Prompt.ID, assistantcontrol.Answer{Selected: []string{"allow"}}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
}
func TestClientCancellationClosesBlockedWrite(t *testing.T) {
	left, right := net.Pipe()
	defer right.Close()
	client := Connect(context.Background(), left, left, nil, nil)
	defer client.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- client.Call(ctx, "initialize", map[string]int{"protocolVersion": 1}, nil) }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked write hid cancellation")
	}
}
func TestClientCloseWithdrawsPendingInteraction(t *testing.T) {
	left, right := net.Pipe()
	defer right.Close()
	c := &assistantcontrol.Coordinator{}
	turn, _ := c.Begin(context.Background(), assistantcontrol.Target{}, nil)
	bridge := &Interactions{Turn: turn, SessionID: "s1"}
	client := Connect(context.Background(), left, left, bridge, nil)
	defer client.Close()
	bridge.Bind(client)
	go func() {
		_ = json.NewEncoder(right).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "chrys/request_input", "params": map[string]any{"sessionId": "s1", "question": "旧问题", "options": []string{"A"}}})
	}()
	state := awaitInteraction(t, c)
	client.Close()
	deadline := time.Now().Add(time.Second)
	for c.Snapshot().Prompt != nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if c.Snapshot().Prompt != nil {
		t.Fatal("dead agent left a prompt")
	}
	if err := c.Respond(state.TurnID, state.Prompt.ID, assistantcontrol.Answer{Text: "late"}); !errors.Is(err, assistantcontrol.ErrStale) {
		t.Fatal(err)
	}
}
func TestClientRejectsMalformedProtocol(t *testing.T) {
	for _, payload := range []string{"not json\n", `{"jsonrpc":"1.0","id":1,"result":{}}` + "\n", strings.Repeat("x", (1<<20)+1) + "\n"} {
		left, right := net.Pipe()
		client := Connect(context.Background(), left, left, nil, nil)
		go func() { _, _ = right.Write([]byte(payload)); _ = right.Close() }()
		select {
		case <-client.Done():
		case <-time.After(time.Second):
			client.Close()
			t.Fatal("malformed stream remained active")
		}
	}
}
func TestInteractionsRejectForeignSessionAndUnknownTools(t *testing.T) {
	c := &assistantcontrol.Coordinator{}
	turn, _ := c.Begin(context.Background(), assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	b := &Interactions{Turn: turn, SessionID: "bound"}
	if _, err := b.Request(context.Background(), "chrys/request_input", json.RawMessage(`{"sessionId":"foreign","question":"Q"}`)); err == nil {
		t.Fatal("foreign session asked a question")
	}
	if _, err := b.Request(context.Background(), "terminal/create", json.RawMessage(`{"sessionId":"bound"}`)); err == nil {
		t.Fatal("unexpected terminal capability")
	}
	if c.Snapshot().Prompt != nil {
		t.Fatal("unknown request changed interaction")
	}
}
func TestLiveChrysInitialize(t *testing.T) {
	executable := os.Getenv("OPSCOPILOT_CHRYS_TEST_EXE")
	if executable == "" {
		t.Skip("explicit local Chrys handshake test disabled")
	}
	directory := t.TempDir()
	home := filepath.Join(directory, "private-home")
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	env := managedEnvironment(directory, home, "")
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	client, err := Start(ctx, ProcessConfig{Executable: executable, Args: []string{"acp"}, Directory: directory, Environment: env}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	result, err := client.Initialize(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("LIVE_CHRYS_INITIALIZE name=%s version=%s protocol=%d; no session/new or model prompt sent", result.AgentInfo.Name, result.AgentInfo.Version, result.ProtocolVersion)
	if err := result.requireManagedCapabilities(); err != nil {
		t.Fatal(err)
	}
}
