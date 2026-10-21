package main

import (
	"context"
	"errors"
	"opscopilot/pkg/assistantcontrol"
	"testing"
	"time"
)

func TestAssistantTargetClosureCancelsWaitingAndFencesReplacements(t *testing.T) {
	a := newLifecycleApp()
	life, err := a.terminalLifecycles.Begin("waiting-tab")
	if err != nil {
		t.Fatal(err)
	}
	target, err := a.installTerminal(life, "waiting-tab", ConnectConfig{}, &lifecycleConn{}, &handoffTestWriter{})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := a.assistantCoordinator().Begin(context.Background(), assistantcontrol.Target{TabID: "waiting-tab", ConnectionID: target.connectionID}, nil)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := turn.Request(context.Background(), assistantcontrol.Prompt{Kind: "input", Title: "哪份日志？", Expires: time.Now().Add(time.Minute)})
		done <- err
	}()
	var state assistantcontrol.State
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		state = a.AssistantControlState()
		if state.Prompt != nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if state.Prompt == nil {
		t.Fatal("question missing")
	}
	a.cancelAssistantTarget("waiting-tab", "previous-connection")
	if a.AssistantControlState().Prompt == nil {
		t.Fatal("old connection canceled new work")
	}
	a.CloseSession("waiting-tab")
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("closed tab still accepted an answer")
		}
	case <-time.After(time.Second):
		t.Fatal("close did not release waiter")
	}
	if err := a.RespondAssistantInteraction(state.TurnID, state.Prompt.ID, assistantcontrol.Answer{Text: "late"}); !errors.Is(err, assistantcontrol.ErrStale) {
		t.Fatal(err)
	}
	fresh, err := a.assistantCoordinator().Begin(context.Background(), assistantcontrol.Target{TabID: "waiting-tab", ConnectionID: "fresh"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Finish("completed", "")
	if err := a.StopAssistantTurn(state.TurnID); !errors.Is(err, assistantcontrol.ErrStale) {
		t.Fatal(err)
	}
	if a.AssistantControlState().TurnID != fresh.ID() {
		t.Fatal("stale stop altered replacement")
	}
}
