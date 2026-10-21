package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/terminalcontrol"
)

func (a *App) assistantCoordinator() *assistantcontrol.Coordinator {
	a.assistantControlOnce.Do(func() { a.assistantControl = &assistantcontrol.Coordinator{} })
	return a.assistantControl
}
func (a *App) cancelAssistantTarget(sessionID, connectionID string) {
	state := a.assistantCoordinator().Snapshot()
	if state.Target.TabID == sessionID && (connectionID == "" || state.Target.ConnectionID == connectionID) {
		_ = a.assistantCoordinator().Stop(state.TurnID)
	}
}
func (a *App) AssistantControlState() assistantcontrol.State {
	return a.assistantCoordinator().Snapshot()
}
func (a *App) RespondAssistantInteraction(turnID, requestID string, answer assistantcontrol.Answer) error {
	return a.assistantCoordinator().Respond(turnID, requestID, answer)
}
func (a *App) StopAssistantTurn(turnID string) error {
	a.assistantConversationMu.Lock()
	state := a.assistantCoordinator().Snapshot()
	if state.TurnID != turnID {
		a.assistantConversationMu.Unlock()
		return assistantcontrol.ErrStale
	}
	done := a.assistantDone
	a.assistantConversationMu.Unlock()
	terminals := a.terminalControlService().States()
	if err := a.assistantCoordinator().Stop(turnID); err != nil {
		return err
	}
	for _, terminal := range terminals {
		if terminal.SessionID == state.Target.TabID && terminal.ConnectionID == state.Target.ConnectionID && terminal.Owned {
			if err := a.terminalControlService().Stop(terminal.SessionID, terminal.RunID); err != nil {
				return err
			}
		}
	}
	if done != nil {
		timer := time.NewTimer(8 * time.Second)
		defer timer.Stop()
		select {
		case <-done:
		case <-timer.C:
			return errors.New("助手仍在停止，请稍后重试")
		}
	}
	return nil
}

func (a *App) StartTerminalApprovalTrial(sessionID, connectionID string, confirmed bool) (assistantcontrol.State, error) {
	if !confirmed || connectionID == "" {
		return assistantcontrol.State{}, terminalcontrol.ErrNeedConfirmation
	}
	var initial terminalcontrol.State
	for _, state := range a.terminalControlService().States() {
		if state.SessionID == sessionID && state.ConnectionID == connectionID {
			initial = state
			break
		}
	}
	if initial.ConnectionID == "" {
		return assistantcontrol.State{}, terminalcontrol.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)
	turn, err := a.assistantCoordinator().Begin(ctx, assistantcontrol.Target{TabID: sessionID, ConnectionID: connectionID}, nil)
	if err != nil {
		cancel()
		return assistantcontrol.State{}, err
	}
	go func() {
		defer cancel()
		a.runTerminalApprovalTrial(turn, terminalcontrol.Target{SessionID: sessionID, ConnectionID: connectionID})
	}()
	return turn.Snapshot(), nil
}

// This is a deterministic, harmless trial of the real authority boundary. It
// accepts no command text, scope, UID or approval credential from the browser.
func (a *App) runTerminalApprovalTrial(turn *assistantcontrol.Turn, target terminalcontrol.Target) {
	ctx := turn.Context()
	fail := func(err error) {
		if ctx.Err() != nil {
			turn.Finish("stopped", "已停止，旧确认已撤销")
			return
		}
		if errors.Is(err, terminalcontrol.ErrStaleApproval) {
			turn.Finish("failed", "终端现场已变化，请重新开始体验")
			return
		}
		turn.Finish("failed", fmt.Sprintf("授权体验未完成：%v", err))
	}
	setup, err := a.terminalControlService().Take(ctx, terminalcontrol.TakeRequest{Target: target, ConfirmedOrdinaryShell: true})
	if err != nil {
		fail(err)
		return
	}
	view, err := setup.Inspect(terminalcontrol.ReadOptions{Limit: 1})
	_ = setup.Release()
	if err != nil || view.LastPrompt == nil {
		if err == nil {
			err = terminalcontrol.ErrNeedConfirmation
		}
		fail(err)
		return
	}
	access, err := a.newTerminalAccess(ctx, terminalcontrol.AccessScope{Actor: "desktop-approval-trial", Expires: time.Now().Add(2 * time.Minute), Targets: []terminalcontrol.AccessTarget{{Target: target, UID: view.LastPrompt.UID}}})
	if err != nil {
		fail(err)
		return
	}
	defer access.Cancel()
	run, err := access.Operations().Take(ctx, terminalcontrol.TakeRequest{Target: target, ConfirmedOrdinaryShell: true})
	if err != nil {
		fail(err)
		return
	}
	if _, err := run.Execute(ctx, terminalcontrol.CommandRequest{Command: "command pwd"}); err != nil {
		_ = run.Release()
		fail(err)
		return
	}
	_, err = run.Execute(ctx, terminalcontrol.CommandRequest{Command: `command printf '\n[授权体验] 本次操作已获明确批准，仍在原 Shell 中。\n'`})
	var required *terminalcontrol.ApprovalRequired
	if !errors.As(err, &required) {
		_ = run.Release()
		if err == nil {
			err = errors.New("未产生预期审批")
		}
		fail(err)
		return
	}
	review, err := access.Review(required.ID)
	if err != nil {
		fail(err)
		return
	}
	answer, err := turn.Request(ctx, assistantcontrol.Prompt{Kind: "approval", Title: "确认这一次终端操作", Description: "这条体验命令只在当前终端打印一段文字，不修改服务器文件。等待期间，你可以继续手动使用终端。", Command: review.Command, Expires: review.Expires, Choices: []assistantcontrol.Choice{{ID: "reject", Label: "拒绝"}, {ID: "allow", Label: "允许本次"}}, Details: []assistantcontrol.Detail{{Label: "执行身份", Value: fmt.Sprintf("UID %d · Shell %d", review.Prompt.UID, review.Prompt.PID)}, {Label: "当前目录", Value: review.Prompt.Directory}}})
	if err != nil {
		fail(err)
		return
	}
	allow := len(answer.Selected) == 1 && answer.Selected[0] == "allow"
	approved, err := access.Approve(required.ID, allow)
	if !allow {
		if err != nil && !errors.Is(err, terminalcontrol.ErrDenied) {
			fail(err)
			return
		}
		turn.Finish("completed", "已拒绝，体验命令没有执行")
		return
	}
	if err != nil {
		fail(err)
		return
	}
	result, err := approved.Execute(ctx)
	if err != nil {
		fail(err)
		return
	}
	if result.Completion != terminalcontrol.Completed {
		fail(errors.New("命令结果尚未确认"))
		return
	}
	if result.ExitCode == nil || *result.ExitCode != 0 {
		fail(errors.New("体验命令未成功返回，请核对终端结果"))
		return
	}
	turn.Finish("completed", "授权体验完成，终端已交还")
}
