package main

import (
	"context"
	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/terminalcontrol"
	"testing"
)

func TestAssistantChatDoesNotDependOnUnusedTerminal(t *testing.T) {
	a := newLifecycleApp()
	turn, err := a.assistantCoordinator().Begin(context.Background(), assistantcontrol.Target{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer turn.Finish("completed", "")
	tools := a.assistantTerminalTools(turn, terminalcontrol.Target{})
	defer tools.Close()
	// No config/audit directory or SSH resource is needed to construct chat tools.
	if turn.Snapshot().Target != (assistantcontrol.Target{}) {
		t.Fatal("chat bound a terminal")
	}
	a.cancelAssistantTarget("unused-tab", "unused-connection")
	if turn.Context().Err() != nil {
		t.Fatal("unused terminal closure canceled chat")
	}
	for _, tool := range []string{"terminal_context", "terminal_take", "terminal_execute"} {
		result, err := tools.Call(turn.Context(), tool, "pwd")
		if status, ok := result.(map[string]string); err != nil || !ok || status["code"] != "ssh_required" {
			t.Fatal("missing prerequisite not returned to model", tool, result, err)
		}
		if state := turn.Snapshot(); state.Phase != "running" || state.Prompt != nil || state.Target != (assistantcontrol.Target{}) {
			t.Fatal(state)
		}
	}
	stale := a.assistantTerminalTools(turn, terminalcontrol.Target{SessionID: "closed", ConnectionID: "old"})
	defer stale.Close()
	result, err := stale.Call(turn.Context(), "terminal_context", "")
	if status, ok := result.(map[string]string); err != nil || !ok || status["code"] != "ssh_unavailable" {
		t.Fatal("closed target did not explain requirement", result, err)
	}
	if state := turn.Snapshot(); state.Target != (assistantcontrol.Target{}) || state.Phase != "running" || state.Prompt != nil {
		t.Fatal("unavailable tool broke chat", state)
	}
	if _, err := tools.Call(turn.Context(), "terminal_release", ""); err != nil {
		t.Fatal(err)
	}
}

func TestAssistantTerminalCandidateBindsOnlyWhenToolIsUsed(t *testing.T) {
	a := newLifecycleApp()
	life, err := a.terminalLifecycles.Begin("tool-tab")
	if err != nil {
		t.Fatal(err)
	}
	installed, err := a.installTerminal(life, "tool-tab", ConnectConfig{}, &lifecycleConn{}, &handoffTestWriter{})
	if err != nil {
		t.Fatal(err)
	}
	turn, _ := a.assistantCoordinator().Begin(context.Background(), assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	target := terminalcontrol.Target{SessionID: "tool-tab", ConnectionID: installed.connectionID}
	tools := a.assistantTerminalTools(turn, target)
	defer tools.Close()
	if turn.Snapshot().Target != (assistantcontrol.Target{}) {
		t.Fatal("candidate eagerly bound")
	}
	if err := tools.Validate(turn.Context()); err != nil {
		t.Fatal(err)
	}
	if turn.Snapshot().Target != (assistantcontrol.Target{TabID: target.SessionID, ConnectionID: target.ConnectionID}) {
		t.Fatal(turn.Snapshot())
	}
	a.cancelAssistantTarget(target.SessionID, target.ConnectionID)
	if turn.Context().Err() == nil {
		t.Fatal("active tool target closure did not cancel its work")
	}
}
