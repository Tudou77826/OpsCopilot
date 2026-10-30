package main

import (
	"context"
	"errors"
	"time"

	"opscopilot/pkg/assistantcontrol"
)

// AssistantApprovalMode is intentionally session-only. In particular bypass
// is never restored after restarting the app or starting a new conversation.
func (a *App) AssistantApprovalMode() string {
	a.assistantConversationMu.Lock()
	defer a.assistantConversationMu.Unlock()
	if a.assistantApprovalMode == "" {
		return "auto"
	}
	return a.assistantApprovalMode
}

func (a *App) SelectAssistantApprovalMode(mode string) (string, error) {
	if mode != "auto" && mode != "manual" && mode != "bypass" {
		return "", errors.New("不支持的权限模式")
	}
	a.assistantConversationMu.Lock()
	defer a.assistantConversationMu.Unlock()
	state := a.assistantCoordinator().Snapshot()
	if a.assistantRunning || state.Phase == "running" || state.Phase == "waiting" {
		return "", assistantcontrol.ErrBusy
	}
	if a.assistantConversation != nil {
		previous := a.assistantApprovalMode
		if previous == "" {
			previous = "auto"
		}
		if a.assistantConversation.AgentKind() == "opencode" && (previous == "bypass") != (mode == "bypass") {
			// OpenCode loads its task and command capabilities at startup. Restart
			// on entering/leaving full access so a reviewed mode never inherits
			// unrestricted pre-execution from a prior full-access session.
			a.assistantConversation.Close()
			a.assistantConversation = nil
			a.assistantRecovered = true
			a.assistantRecoveryNote = "权限模式已切换，助手将参考近期可见对话继续。"
			a.assistantApprovalMode = mode
			return mode, nil
		}
		ctx := a.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		if err := a.assistantConversation.SwitchApprovalMode(ctx, mode); err != nil {
			return "", err
		}
	}
	a.assistantApprovalMode = mode
	return mode, nil
}
