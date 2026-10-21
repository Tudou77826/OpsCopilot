package assistanttools

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/terminalcontrol"
)

func waitPrompt(t *testing.T, c *assistantcontrol.Coordinator) assistantcontrol.State {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s := c.Snapshot()
		if s.Prompt != nil {
			return s
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("no prompt")
	return assistantcontrol.State{}
}
func TestNoTerminalAccessBeforeConsentAndRejectDoesNotOpen(t *testing.T) {
	c := &assistantcontrol.Coordinator{}
	turn, _ := c.Begin(context.Background(), assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	var opened atomic.Int32
	b := &TerminalTools{Turn: turn, Target: terminalcontrol.Target{SessionID: "tab", ConnectionID: "connection"}, Open: func(context.Context) (*terminalcontrol.Access, error) {
		opened.Add(1)
		return nil, errors.New("unexpected")
	}}
	done := make(chan error, 1)
	go func() { _, err := b.Call(turn.Context(), "terminal_context", ""); done <- err }()
	state := waitPrompt(t, c)
	if opened.Load() != 0 {
		t.Fatal("opened without consent")
	}
	if _, err := b.Call(turn.Context(), "terminal_execute", "pwd"); err == nil {
		t.Fatal("concurrent call queued")
	}
	if err := c.Respond(state.TurnID, state.Prompt.ID, assistantcontrol.Answer{Selected: []string{"reject"}}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, terminalcontrol.ErrDenied) || opened.Load() != 0 {
		t.Fatal(err, opened.Load())
	}
	if _, err := b.Call(turn.Context(), "terminal_context", ""); !errors.Is(err, terminalcontrol.ErrDenied) || c.Snapshot().Prompt != nil {
		t.Fatal("rejected scope asked for permission again", err)
	}
}
func TestStopInvalidatesConsentAndUnknownToolsDoNotAsk(t *testing.T) {
	c := &assistantcontrol.Coordinator{}
	turn, _ := c.Begin(context.Background(), assistantcontrol.Target{}, nil)
	b := &TerminalTools{Turn: turn, Target: terminalcontrol.Target{SessionID: "tab", ConnectionID: "connection"}}
	if _, err := b.Call(turn.Context(), "shell", ""); err == nil || c.Snapshot().Prompt != nil {
		t.Fatal("unknown tool affected UI")
	}
	done := make(chan error, 1)
	go func() { _, err := b.Call(turn.Context(), "terminal_context", ""); done <- err }()
	state := waitPrompt(t, c)
	_ = c.Stop(turn.ID())
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("stopped consent succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("stop stuck")
	}
	if err := c.Respond(state.TurnID, state.Prompt.ID, assistantcontrol.Answer{Selected: []string{"allow"}}); !errors.Is(err, assistantcontrol.ErrStale) {
		t.Fatal(err)
	}
	b.Close()
}

func TestPassiveContextNeverOpensInputAccessAndExecuteNeedsTake(t *testing.T) {
	c := &assistantcontrol.Coordinator{}
	turn, _ := c.Begin(context.Background(), assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	var opened, observed atomic.Int32
	b := &TerminalTools{Turn: turn, Target: terminalcontrol.Target{SessionID: "tab", ConnectionID: "connection"}, Observe: func(context.Context) (terminalcontrol.TerminalContext, error) {
		observed.Add(1)
		return terminalcontrol.TerminalContext{Ready: false, Owned: false}, nil
	}, Open: func(context.Context) (*terminalcontrol.Access, error) {
		opened.Add(1)
		return nil, errors.New("must not open")
	}}
	if _, err := b.Call(turn.Context(), "terminal_execute", "pwd"); err == nil || c.Snapshot().Prompt != nil {
		t.Fatal("execute implicitly acquired")
	}
	done := make(chan error, 1)
	go func() { _, err := b.Call(turn.Context(), "terminal_context", ""); done <- err }()
	state := waitPrompt(t, c)
	if state.Prompt.Title != "允许助手查看当前终端？" {
		t.Fatal(state)
	}
	if err := c.Respond(state.TurnID, state.Prompt.ID, assistantcontrol.Answer{Selected: []string{"allow"}}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := b.Call(turn.Context(), "terminal_context", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Call(turn.Context(), "terminal_release", ""); err != nil {
		t.Fatal(err)
	}
	if observed.Load() != 2 || opened.Load() != 0 {
		t.Fatal(observed.Load(), opened.Load())
	}
}
