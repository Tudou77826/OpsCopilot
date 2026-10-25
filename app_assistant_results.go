package main

import (
	"context"
	"errors"
	"time"

	"opscopilot/pkg/assistantcontrol"
)

// RevealAssistantResult accepts a display message ID, never model-supplied
// paths or executable actions. Navigation cannot restore lost authority.
func (a *App) RevealAssistantResult(client, messageID string) error {
	if err := a.workspaceService().Registered(client); err != nil {
		return err
	}
	a.assistantConversationMu.Lock()
	state := a.assistantCoordinator().Snapshot()
	if a.assistantRunning || state.Phase == "running" || state.Phase == "waiting" {
		// An observer page cannot steal an executing turn's presentation receipts.
		if err := a.workspaceService().Check(client); err != nil {
			a.assistantConversationMu.Unlock()
			return err
		}
	} else if err := a.workspaceService().Activate(client); err != nil {
		a.assistantConversationMu.Unlock()
		return err
	}
	if err := a.ensureAssistantHistoryLocked(); err != nil {
		a.assistantConversationMu.Unlock()
		return err
	}
	messages := a.assistantHistory.Snapshot().Messages
	a.assistantConversationMu.Unlock()
	messages = append(messages, a.assistantCoordinator().Snapshot().Messages...)
	var op assistantcontrol.Operation
	for _, message := range messages {
		if message.ID == messageID && message.Role == "ai" && message.Kind == "operation" {
			op = message.Operation
			break
		}
	}
	if op.WorkspaceID == "" {
		return errors.New("原操作结果已不存在，请查看保留的对话记录")
	}
	ctx, cancel := context.WithTimeout(a.ctx, 12*time.Second)
	defer cancel()
	switch op.Kind {
	case "database":
		return a.databaseControlService().Reveal(ctx, op.WorkspaceID, op.ItemID)
	case "file":
		return a.fileControlService().Reveal(ctx, op.WorkspaceID, op.ItemID)
	case "terminal":
		_, view, err := a.workspaceService().Current()
		if err != nil {
			return err
		}
		for _, tab := range view.Tabs {
			if tab.TabID == op.WorkspaceID && tab.ConnectionID == op.ConnectionID {
				return a.workspaceService().Present(ctx, client, "select", tab)
			}
		}
		return errors.New("原终端已关闭或连接已更换；不会重新连接或执行命令")
	default:
		return errors.New("此记录没有可定位的操作结果")
	}
}
