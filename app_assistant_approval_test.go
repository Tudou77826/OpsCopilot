package main

import (
	"errors"
	"testing"

	"opscopilot/pkg/assistantcontrol"
)

func TestAssistantApprovalModeIsIdleOnlyAndSessionScoped(t *testing.T) {
	a := newLifecycleApp()
	if a.AssistantApprovalMode() != "auto" {
		t.Fatal("wrong default")
	}
	if _, err := a.SelectAssistantApprovalMode("unknown"); err == nil {
		t.Fatal("accepted invalid mode")
	}
	a.assistantRunning = true
	if _, err := a.SelectAssistantApprovalMode("bypass"); !errors.Is(err, assistantcontrol.ErrBusy) {
		t.Fatal("accepted mode change during a turn")
	}
	a.assistantRunning = false
	if mode, err := a.SelectAssistantApprovalMode("bypass"); err != nil || mode != "bypass" {
		t.Fatal(mode, err)
	}
	if err := a.NewAssistantConversation(); err != nil {
		t.Fatal(err)
	}
	if a.AssistantApprovalMode() != "auto" {
		t.Fatal("bypass survived new conversation")
	}
}
