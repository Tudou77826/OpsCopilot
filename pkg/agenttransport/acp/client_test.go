package acp

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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
func TestClientIgnoresNonProtocolOutput(t *testing.T) {
	left, right := net.Pipe()
	defer right.Close()
	client := Connect(context.Background(), left, left, nil, nil)
	defer client.Close()
	serverDone := make(chan error, 1)
	go func() {
		// CodeAgent announces its service address on stdout before the handshake.
		_, err := right.Write([]byte("[ACP] Server started: http://127.0.0.1:7725/\nnot json\n{\"jsonrpc\":\"1.0\",\"id\":1,\"result\":{}}\n"))
		if err != nil {
			serverDone <- err
			return
		}
		decoder, encoder := json.NewDecoder(right), json.NewEncoder(right)
		var initialize envelope
		if err := decoder.Decode(&initialize); err != nil {
			serverDone <- err
			return
		}
		serverDone <- encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(initialize.ID), "result": map[string]any{"protocolVersion": 1, "agentInfo": map[string]string{"name": "codeagent", "version": "1"}}})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if result, err := client.Initialize(ctx); err != nil || result.AgentInfo.Name != "codeagent" {
		t.Fatal(result, err)
	}
	select {
	case <-client.Done():
		t.Fatal("non-protocol output ended the session")
	default:
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
}
func TestClientClosesOnOversizedLine(t *testing.T) {
	left, right := net.Pipe()
	client := Connect(context.Background(), left, left, nil, nil)
	defer client.Close()
	go func() { _, _ = right.Write([]byte(strings.Repeat("x", maxMessageBytes+1) + "\n")); _ = right.Close() }()
	select {
	case <-client.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("oversized line left the stream active")
	}
}

// slowHandler stalls on every call until released, like a consumer waiting on
// a synchronous audit write or a person.
type slowHandler struct {
	release  chan struct{}
	handled  atomic.Int32
	requests bool
}

func (h *slowHandler) Request(ctx context.Context, _ string, _ json.RawMessage) (any, error) {
	h.handled.Add(1)
	select {
	case <-h.release:
	case <-ctx.Done():
	}
	return nil, nil
}

func (h *slowHandler) Notify(ctx context.Context, _ string, _ json.RawMessage) {
	h.handled.Add(1)
	select {
	case <-h.release:
	case <-ctx.Done():
	}
}

// burstAgent handshakes around count agent messages sent while handler is
// stalled. It returns the client, the handshake result and the release signal.
func burstAgent(t *testing.T, handler *slowHandler, count int) (*Client, <-chan error, chan struct{}) {
	t.Helper()
	left, right := net.Pipe()
	t.Cleanup(func() { _ = right.Close() })
	client := Connect(context.Background(), left, left, handler, nil)
	t.Cleanup(client.Close)
	handler.release = make(chan struct{})
	done := make(chan error, 1)
	result := func(id json.RawMessage) {
		_ = json.NewEncoder(right).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"protocolVersion": 1, "agentInfo": map[string]string{"name": "burst", "version": "1"}}})
	}
	written := make(chan struct{})
	go func() {
		decoder, encoder := json.NewDecoder(right), json.NewEncoder(right)
		var initialize envelope
		if err := decoder.Decode(&initialize); err != nil {
			done <- err
			return
		}
		id := json.RawMessage(initialize.ID)
		if handler.requests {
			// A real agent keeps reading answers while it writes; the synchronous
			// test pipe needs a separate reader for them.
			go func() {
				for i := 0; i < count; i++ {
					var reply json.RawMessage
					if decoder.Decode(&reply) != nil {
						return
					}
				}
				<-written
				result(id)
			}()
		}
		for i := 0; i < count; i++ {
			message := map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "s1"}}
			if handler.requests {
				message = map[string]any{"jsonrpc": "2.0", "id": i + 1, "method": "session/request_permission", "params": map[string]any{"sessionId": "s1"}}
			}
			if err := encoder.Encode(message); err != nil {
				done <- err
				return
			}
		}
		close(written)
		if !handler.requests {
			result(id)
		}
	}()
	go func() {
		_, err := client.Initialize(context.Background())
		done <- err
	}()
	deadline := time.Now().Add(3 * time.Second)
	for handler.handled.Load() == 0 && time.Now().Before(deadline) {
		select {
		case err := <-done:
			t.Fatalf("handshake settled before the burst was released: %v", err)
		default:
		}
		time.Sleep(time.Millisecond)
	}
	if handler.handled.Load() == 0 {
		t.Fatal("no traffic reached the handler")
	}
	return client, done, handler.release
}

func TestClientBackpressuresNotificationBursts(t *testing.T) {
	handler := &slowHandler{}
	client, done, release := burstAgent(t, handler, 200)
	// The consumer is stalled well beyond the buffer size of 64.
	time.Sleep(100 * time.Millisecond)
	select {
	case <-client.Done():
		t.Fatal("notification burst ended the session")
	default:
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for handler.handled.Load() < 200 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := handler.handled.Load(); got != 200 {
		t.Fatalf("notifications delivered = %d, want 200", got)
	}
}

func TestClientBackpressuresConcurrentRequests(t *testing.T) {
	handler := &slowHandler{requests: true}
	client, done, release := burstAgent(t, handler, 12)
	// Nine requests are in flight well beyond the limit of eight.
	time.Sleep(100 * time.Millisecond)
	select {
	case <-client.Done():
		t.Fatal("concurrent requests ended the session")
	default:
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for handler.handled.Load() < 12 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := handler.handled.Load(); got != 12 {
		t.Fatalf("requests handled = %d, want 12", got)
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
