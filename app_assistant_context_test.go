package main

import (
	"context"
	"strings"
	"testing"

	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/terminalcontrol"
	"opscopilot/pkg/workspace"
)

func TestAssistantPageContextUsesSelectedRemoteMetadataWithoutSecrets(t *testing.T) {
	a := newLifecycleApp()
	client := a.workspaceService().Attach()
	a.activeConfigs = map[string]ConnectConfig{}
	a.activeConfigs["selected"] = ConnectConfig{Host: "39.108.107.148", Port: 22, User: "ops", Password: "secret-login", RootPassword: "secret-root"}
	target := terminalcontrol.Target{SessionID: "selected", ConnectionID: "physical"}
	_, _ = a.workspaceService().Pull(client, workspace.Snapshot{Tabs: []workspace.Tab{{Target: workspace.Target{TabID: target.SessionID, ConnectionID: target.ConnectionID}, Title: "remote node", Status: "connected"}}})
	encoded := a.assistantPageContext(target)
	if !strings.Contains(encoded, "remote-terminal") || !strings.Contains(encoded, "39.108.107.148") {
		t.Fatal("missing selected node", encoded)
	}
	if strings.Contains(encoded, "secret-") || strings.Contains(strings.ToLower(encoded), "password") {
		t.Fatal("context contains credentials")
	}
	if next := a.assistantPageContext(terminalcontrol.Target{}); strings.Contains(next, "39.108.107.148") {
		t.Fatal("later chat retained previous target")
	}
}

func TestAssistantDisconnectedSelectedPageDoesNotFallBackToLocal(t *testing.T) {
	a := newLifecycleApp()
	client := a.workspaceService().Attach()
	target := workspace.Target{TabID: "old-tab", ConnectionID: "old-connection"}
	_, err := a.workspaceService().Pull(client, workspace.Snapshot{Tabs: []workspace.Tab{{Target: target, Title: "业务节点", Status: "disconnected"}}, Active: target})
	if err != nil {
		t.Fatal(err)
	}
	context := a.assistantPageContext(terminalcontrol.Target{})
	if !strings.Contains(context, "remote-unavailable") || !strings.Contains(context, "不要改为检查本机") {
		t.Fatal("lost disconnected remote context", context)
	}
}

func TestAssistantOtherPageRegistrationAndHistoryRestoreDoNotStopSender(t *testing.T) {
	a := newLifecycleApp()
	sender, err := a.WorkspaceRegisterPage("sender")
	if err != nil {
		t.Fatal(err)
	}
	if err = a.workspaceService().Activate(sender); err != nil {
		t.Fatal(err)
	}
	turn, _ := a.assistantCoordinator().Begin(context.Background(), assistantcontrol.Target{}, nil)
	turn.SetSource("assistant")
	defer turn.Finish("completed", "")
	observer, err := a.WorkspaceRegisterPage("observer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.RestoreAssistantConversationOnPage(observer); err != nil {
		t.Fatal(err)
	}
	if turn.Context().Err() != nil {
		t.Fatal("opening observer cancelled sender")
	}
	a.WorkspaceDetach(observer)
	if turn.Context().Err() != nil {
		t.Fatal("closing observer cancelled sender")
	}
	if _, err = a.WorkspaceRegisterPage("sender"); err != nil {
		t.Fatal(err)
	}
	if turn.Context().Err() == nil {
		t.Fatal("same-page reload failed to revoke old turn")
	}
}
