package acp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"opscopilot/pkg/assistantcontrol"
)

func TestPermissionSelfReviewOnlyAutoAllowsReadOnlyActions(t *testing.T) {
	for _, kind := range []string{"execute", "edit"} {
		t.Run(kind, func(t *testing.T) {
			coordinator := &assistantcontrol.Coordinator{}
			turn, _ := coordinator.Begin(context.Background(), assistantcontrol.Target{}, nil)
			defer turn.Finish("completed", "")
			bridge := &Interactions{Turn: turn, SessionID: "s", Review: func(context.Context, assistantcontrol.ReviewInput) assistantcontrol.ReviewVerdict {
				return assistantcontrol.ReviewVerdict{ReadOnly: true, InScope: true, Reason: "仅查看普通目录"}
			}}
			payload, _ := json.Marshal(map[string]any{"sessionId": "s", "toolCall": map[string]any{"kind": kind, "title": "list files", "rawInput": map[string]string{"command": "Get-ChildItem D:/logs"}}, "options": []any{map[string]string{"optionId": "yes", "kind": "allow_once"}, map[string]string{"optionId": "no", "kind": "reject_once"}}})
			done := make(chan error, 1)
			go func() {
				_, err := bridge.Request(context.Background(), "session/request_permission", payload)
				done <- err
			}()
			if kind == "edit" {
				deadline := time.Now().Add(time.Second)
				for coordinator.Snapshot().Prompt == nil && time.Now().Before(deadline) {
					time.Sleep(time.Millisecond)
				}
				p := coordinator.Snapshot().Prompt
				if p == nil {
					t.Fatal("file edit bypassed human confirmation")
				}
				if err := coordinator.Respond(turn.ID(), p.ID, assistantcontrol.Answer{Selected: []string{"reject"}}); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("permission did not resolve")
			}
			if kind == "execute" && coordinator.Snapshot().Prompt != nil {
				t.Fatal("readonly self review still asked human")
			}
		})
	}
}

func TestLateSelfReviewCannotApproveAfterStop(t *testing.T) {
	c := &assistantcontrol.Coordinator{}
	turn, _ := c.Begin(context.Background(), assistantcontrol.Target{}, nil)
	started := make(chan struct{})
	finish := make(chan struct{})
	b := &Interactions{Turn: turn, SessionID: "s", Review: func(context.Context, assistantcontrol.ReviewInput) assistantcontrol.ReviewVerdict {
		close(started)
		<-finish
		return assistantcontrol.ReviewVerdict{ReadOnly: true, InScope: true, Reason: "late"}
	}}
	done := make(chan error, 1)
	go func() {
		_, err := b.Request(context.Background(), "session/request_permission", json.RawMessage(`{"sessionId":"s","toolCall":{"kind":"execute","title":"ls","rawInput":{"command":"ls /tmp"}},"options":[{"optionId":"yes","kind":"allow_once"},{"optionId":"no","kind":"reject_once"}]}`))
		done <- err
	}()
	<-started
	turn.Finish("stopped", "用户停止")
	close(finish)
	if err := <-done; !errors.Is(err, assistantcontrol.ErrStale) {
		t.Fatal(err)
	}
}
