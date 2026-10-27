package acp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"opscopilot/pkg/assistantcontrol"
)

func TestICodePermissionAlwaysWaitsForHuman(t *testing.T) {
	c := &assistantcontrol.Coordinator{}
	turn, _ := c.Begin(context.Background(), assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	b := &Interactions{Turn: turn, SessionID: "s"}
	payload := json.RawMessage(`{"sessionId":"s","toolCall":{"kind":"execute","title":"check","rawInput":{"command":"ls"},"rawOutput":"需要确认目标"},"options":[{"optionId":"yes","kind":"allow_once"},{"optionId":"no","kind":"reject_once"}]}`)
	done := make(chan any, 1)
	go func() {
		result, err := b.Request(context.Background(), "session/request_permission", payload)
		if err != nil {
			done <- err
		} else {
			done <- result
		}
	}()
	deadline := time.Now().Add(time.Second)
	for c.Snapshot().Prompt == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	p := c.Snapshot().Prompt
	if p == nil {
		t.Fatal("iCode request was automatically approved or not shown")
	}
	encoded, _ := json.Marshal(p)
	if !strings.Contains(string(encoded), "需要确认目标") {
		t.Fatal("missing iCode review reason", string(encoded))
	}
	if err := c.Respond(turn.ID(), p.ID, assistantcontrol.Answer{Selected: []string{"reject"}}); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-done:
		body, _ := json.Marshal(result)
		if !strings.Contains(string(body), `"optionId":"no"`) {
			t.Fatalf("refusal not forwarded: %s", body)
		}
	case <-time.After(time.Second):
		t.Fatal("permission did not resolve")
	}
}

func TestStopCancelsPendingICodePermission(t *testing.T) {
	c := &assistantcontrol.Coordinator{}
	turn, _ := c.Begin(context.Background(), assistantcontrol.Target{}, nil)
	b := &Interactions{Turn: turn, SessionID: "s"}
	done := make(chan error, 1)
	go func() {
		_, err := b.Request(context.Background(), "session/request_permission", json.RawMessage(`{"sessionId":"s","toolCall":{"kind":"execute","title":"ls","rawInput":{"command":"ls"}},"options":[{"optionId":"yes","kind":"allow_once"},{"optionId":"no","kind":"reject_once"}]}`))
		done <- err
	}()
	deadline := time.Now().Add(time.Second)
	for c.Snapshot().Prompt == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if c.Snapshot().Prompt == nil {
		t.Fatal("permission not presented")
	}
	turn.Finish("stopped", "用户停止")
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("stop resolved as approval")
		}
	case <-time.After(time.Second):
		t.Fatal("stop did not cancel permission")
	}
}
