package main

import (
	"context"
	"strings"
	"testing"

	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/workspace"
)

func TestResultNavigationRejectsLostWorkspacesAndReplacedTerminal(t *testing.T) {
	a := newLifecycleApp()
	client := a.workspaceService().Attach()
	_, err := a.workspaceService().Pull(client, workspace.Snapshot{Tabs: []workspace.Tab{{Target: workspace.Target{TabID: "tab", ConnectionID: "replacement"}}}})
	if err != nil {
		t.Fatal(err)
	}
	turn, _ := a.assistantCoordinator().Begin(context.Background(), assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	for _, kind := range []string{"database", "file", "terminal"} {
		turn.ObserveOperation(kind, "已保留的旧结果", assistantcontrol.Operation{Kind: kind, WorkspaceID: "tab", ConnectionID: "original", Status: "completed"})
		message := turn.Snapshot().Messages[len(turn.Snapshot().Messages)-1]
		if a.RevealAssistantResult(client, message.ID) == nil {
			t.Fatal("lost/replaced result was accepted", kind)
		}
	}
	if a.RevealAssistantResult(client, "model-invented-id") == nil {
		t.Fatal("unrecorded result accepted")
	}
	if a.RevealAssistantResult("unregistered", turn.Snapshot().Messages[0].ID) == nil {
		t.Fatal("unregistered page accepted")
	}
}

func TestResultNavigationCanActivateIdlePageButCannotStealRunningPage(t *testing.T) {
	a := newLifecycleApp()
	sender, _ := a.WorkspaceRegisterPage("sender")
	observer, _ := a.WorkspaceRegisterPage("observer")
	turn, _ := a.assistantCoordinator().Begin(context.Background(), assistantcontrol.Target{}, nil)
	turn.ObserveOperation("file", "传输完成", assistantcontrol.Operation{Kind: "file", WorkspaceID: "lost", Status: "done"})
	id := turn.Snapshot().Messages[0].ID
	if a.RevealAssistantResult(observer, id) == nil {
		t.Fatal("observer stole active page")
	}
	if a.workspaceService().Check(sender) != nil || turn.Context().Err() != nil {
		t.Fatal("navigation interrupted sender")
	}
	turn.Finish("completed", "")
	err := a.RevealAssistantResult(observer, id)
	if err == nil || !strings.Contains(err.Error(), "未恢复") {
		t.Fatal("idle observer could not reach retained result check", err)
	}
	if a.workspaceService().Check(observer) != nil {
		t.Fatal("idle click did not activate page")
	}
}
