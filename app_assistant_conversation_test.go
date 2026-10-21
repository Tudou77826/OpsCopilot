package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"opscopilot/pkg/agenttransport/acp"
	"opscopilot/pkg/assistantcontrol"
)

func TestAssistantNewConversationClearsBackendOnlyWhenIdle(t *testing.T) {
	a := newLifecycleApp()
	conversation := &acp.Conversation{}
	a.assistantConversation = conversation
	a.assistantHistory.Begin("old", "previous question")
	a.assistantHistory.Finish(assistantcontrol.State{TurnID: "old", Phase: "completed", Output: "previous answer"})
	turn, _ := a.assistantCoordinator().Begin(context.Background(), assistantcontrol.Target{}, nil)
	if err := a.NewAssistantConversation(); !errors.Is(err, assistantcontrol.ErrBusy) {
		t.Fatal(err)
	}
	if a.assistantConversation != conversation || turn.Context().Err() != nil {
		t.Fatal("new chat discarded live work")
	}
	turn.Finish("stopped", "")
	a.assistantRunning = true // cancellation cleanup still draining
	if err := a.NewAssistantConversation(); !errors.Is(err, assistantcontrol.ErrBusy) {
		t.Fatal(err)
	}
	a.assistantRunning = false
	if err := a.NewAssistantConversation(); err != nil {
		t.Fatal(err)
	}
	if a.assistantConversation != nil || a.AssistantControlState().Phase != "idle" {
		t.Fatal("backend conversation not reset")
	}
	if len(a.assistantHistory.Snapshot().Messages) != 0 {
		t.Fatal("reset retained visible history")
	}
	if err := a.NewAssistantConversation(); err != nil {
		t.Fatal("empty reset should be idempotent", err)
	}
}

func TestAssistantRestoreCancelsPendingApprovalAndDrainsBeforeHistory(t *testing.T) {
	a := newLifecycleApp()
	turn, _ := a.assistantCoordinator().Begin(context.Background(), assistantcontrol.Target{}, nil)
	turn.SetSource("assistant")
	a.assistantHistory.Begin(turn.ID(), "accepted question")
	turn.AppendText("partial answer")
	a.assistantRunning = true
	a.assistantDone = make(chan struct{})
	go func() {
		_, _ = turn.Request(turn.Context(), assistantcontrol.Prompt{Kind: "approval", Title: "old approval", Expires: time.Now().Add(time.Minute), Choices: []assistantcontrol.Choice{{ID: "allow", Label: "允许本次"}, {ID: "reject", Label: "拒绝"}}})
	}()
	deadline := time.Now().Add(time.Second)
	var promptID string
	for time.Now().Before(deadline) {
		if s := turn.Snapshot(); s.Prompt != nil {
			promptID = s.Prompt.ID
			break
		}
		time.Sleep(time.Millisecond)
	}
	if promptID == "" {
		t.Fatal("approval did not become pending")
	}
	go func() {
		<-turn.Context().Done()
		a.assistantConversationMu.Lock()
		defer a.assistantConversationMu.Unlock()
		turn.Finish("stopped", "已停止")
		a.assistantHistory.Finish(turn.Snapshot())
		a.assistantRunning = false
		close(a.assistantDone)
	}()
	s, err := a.RestoreAssistantConversation()
	if err != nil || len(s.Messages) != 2 || s.Messages[1].Phase != "stopped" || s.Messages[1].Content != "partial answer" {
		t.Fatal(s, err)
	}
	if err := a.RespondAssistantInteraction(turn.ID(), promptID, assistantcontrol.Answer{Selected: []string{"allow"}}); !errors.Is(err, assistantcontrol.ErrStale) {
		t.Fatal("restored an old approval", err)
	}
	if repeat, err := a.RestoreAssistantConversation(); err != nil || len(repeat.Messages) != 2 || repeat.ID != s.ID {
		t.Fatal("restore must not duplicate history", repeat, err)
	}
}

func TestAssistantRestoreDoesNotCancelIndependentOperation(t *testing.T) {
	a := newLifecycleApp()
	a.assistantHistory.Begin("earlier", "earlier chat")
	a.assistantHistory.Finish(assistantcontrol.State{TurnID: "earlier", Phase: "completed", Output: "answer"})
	turn, _ := a.assistantCoordinator().Begin(context.Background(), assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	s, err := a.RestoreAssistantConversation()
	if err != nil || len(s.Messages) != 2 || turn.Context().Err() != nil || turn.Snapshot().Phase != "running" {
		t.Fatal("unrelated operation interfered with history restore", s, err)
	}
}
