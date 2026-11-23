package acp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"opscopilot/pkg/assistantcontrol"
)

func TestHostPermissionDeferredAndBoundToCall(t *testing.T) {
	c := &assistantcontrol.Coordinator{}
	turn, _ := c.Begin(context.Background(), assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	b := &managedBinding{ctx: turn.Context(), turn: turn, pendingPermissions: map[string]int{}}
	args := json.RawMessage(`{"command":"rm /tmp/x","purpose":"test"}`)
	if b.deferHostPermission("other_terminal_execute", args) || !b.deferHostPermission("terminal_execute", args) {
		t.Fatal("invalid tool identity")
	}
	if _, err := b.authorizeCall(turn.Context(), "terminal_execute", json.RawMessage(`{"command":"rm /tmp/y"}`)); err == nil {
		t.Fatal("mismatched call was accepted")
	}
	done := make(chan error, 1)
	go func() {
		ctx, err := b.authorizeCall(turn.Context(), "terminal_execute", args)
		if err == nil {
			var v assistantcontrol.ReviewVerdict
			v, err = turn.ReviewAction(ctx, nil, assistantcontrol.ReviewInput{Tool: "remote_shell", Action: "rm /tmp/x"}, "高风险")
			if err == nil && !v.Allowed() {
				err = errors.New("second confirmation would be required")
			}
		}
		done <- err
	}()
	deadline := time.Now().Add(time.Second)
	for c.Snapshot().Prompt == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	p := c.Snapshot().Prompt
	if p == nil {
		t.Fatal("operation was not confirmed")
	}
	if err := c.Respond(turn.ID(), p.ID, assistantcontrol.Answer{Selected: []string{"allow"}}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	ctx, err := b.authorizeCall(turn.Context(), "terminal_execute", args)
	if err != nil || assistantcontrol.CallApproved(ctx) {
		t.Fatal("approval reused", err)
	}
}

func TestStandardAgentHostPermissionUsesOneConfirmation(t *testing.T) {
	c := &assistantcontrol.Coordinator{}
	turn, _ := c.Begin(context.Background(), assistantcontrol.Target{}, nil)
	turn.SetApprovalMode("manual")
	defer turn.Finish("completed", "")
	b := &managedBinding{ctx: turn.Context(), turn: turn, pendingPermissions: map[string]int{}}
	bridge := &Interactions{Turn: turn, SessionID: "s", AgentName: "ACP Agent", DeferHostPermission: b.deferHostPermission}
	data := json.RawMessage(`{"sessionId":"s","toolCall":{"name":"opscopilot_terminal_execute","rawInput":{"command":"pwd"}},"options":[{"optionId":"yes","kind":"allow_once"},{"optionId":"no","kind":"reject_once"}]}`)
	result, err := bridge.Request(turn.Context(), "session/request_permission", data)
	if err != nil || result == nil || c.Snapshot().Prompt != nil {
		t.Fatal("host request should be deferred", result, err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := b.authorizeCall(turn.Context(), "terminal_execute", json.RawMessage(`{"command":"pwd"}`))
		done <- err
	}()
	deadline := time.Now().Add(time.Second)
	for c.Snapshot().Prompt == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	prompt := c.Snapshot().Prompt
	if prompt == nil {
		t.Fatal("missing host confirmation")
	}
	if err := c.Respond(turn.ID(), prompt.ID, assistantcontrol.Answer{Selected: []string{"allow"}}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestManualExecutionRequiresConfirmationAndBypassDoesNot(t *testing.T) {
	for _, mode := range []string{"manual", "bypass"} {
		t.Run(mode, func(t *testing.T) {
			c := &assistantcontrol.Coordinator{}
			turn, _ := c.Begin(context.Background(), assistantcontrol.Target{}, nil)
			turn.SetApprovalMode(mode)
			defer turn.Finish("completed", "")
			b := &managedBinding{ctx: turn.Context(), turn: turn, pendingPermissions: map[string]int{}}
			done := make(chan error, 1)
			go func() {
				_, err := b.authorizeCall(turn.Context(), "terminal_execute", json.RawMessage(`{"command":"echo hi"}`))
				done <- err
			}()
			if mode == "bypass" {
				select {
				case err := <-done:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(time.Second):
					t.Fatal("unexpected prompt")
				}
				return
			}
			deadline := time.Now().Add(time.Second)
			for c.Snapshot().Prompt == nil && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			p := c.Snapshot().Prompt
			if p == nil {
				t.Fatal("manual mode did not ask")
			}
			if err := c.Respond(turn.ID(), p.ID, assistantcontrol.Answer{Selected: []string{"reject"}}); err != nil {
				t.Fatal(err)
			}
			if err := <-done; !errors.Is(err, assistantcontrol.ErrActionDenied) {
				t.Fatal(err)
			}
			if _, err := b.authorizeCall(turn.Context(), "terminal_execute", json.RawMessage(`{"command":"echo hi"}`)); !errors.Is(err, assistantcontrol.ErrActionDenied) {
				t.Fatal("rejected action was retried", err)
			}
		})
	}
}
