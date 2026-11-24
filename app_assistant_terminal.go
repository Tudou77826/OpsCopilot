package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"opscopilot/pkg/agenttransport/acp"
	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/assistanttools"
	"opscopilot/pkg/terminalcontrol"
)

// StartAssistantTurn accepts a message and an optional resource candidate.
// Conversation never requires SSH; only terminal tools bind and check it.
func (a *App) StartAssistantTurn(question, sessionID, connectionID string) (assistantcontrol.State, error) {
	return a.startAssistantTurn(question, sessionID, connectionID, "")
}
func (a *App) StartAssistantTurnOnPage(question, sessionID, connectionID, client string) (assistantcontrol.State, error) {
	if client == "" {
		return assistantcontrol.State{}, errors.New("消息缺少发起页面，请刷新后重试")
	}
	return a.startAssistantTurn(question, sessionID, connectionID, client)
}
func (a *App) StartAssistantTurnWithReferencesOnPage(question, client string, refs []assistantcontrol.PageReference) (assistantcontrol.State, error) {
	if client == "" {
		return assistantcontrol.State{}, errors.New("消息缺少发起页面，请刷新后重试")
	}
	if err := assistantcontrol.ValidatePageReferences(refs); err != nil {
		return assistantcontrol.State{}, err
	}
	return a.startAssistantTurn(question, "", "", client, refs...)
}
func (a *App) startAssistantTurn(question, sessionID, connectionID, client string, refs ...assistantcontrol.PageReference) (assistantcontrol.State, error) {
	question = strings.TrimSpace(question)
	if question == "" || len(question) > 16384 || !utf8.ValidString(question) {
		return assistantcontrol.State{}, errors.New("请输入不超过 16 KB 的问题")
	}
	target := terminalcontrol.Target{SessionID: sessionID, ConnectionID: connectionID}
	executable, err := a.assistantExecutable()
	if err != nil {
		return assistantcontrol.State{}, err
	}
	a.assistantConversationMu.Lock()
	defer a.assistantConversationMu.Unlock()
	if a.assistantRunning {
		return assistantcontrol.State{}, assistantcontrol.ErrBusy
	}
	recovering := a.recoverUnavailableAssistantConversationLocked()
	managed, err := a.assistantModelConfigLocked()
	if err != nil {
		return assistantcontrol.State{}, err
	}
	managed.Executable = executable
	managed.ApprovalMode = a.assistantApprovalMode
	managed.SkillsDirectory = filepath.Join(a.configMgr.Directory(), "assistant-skills")
	managed.AuditDirectory = filepath.Join(a.configMgr.Directory(), "audit", "assistant")
	if !managed.IsStandardACP() {
		managed.ProjectSkillsEnabled, managed.ProjectConfigEnabled, err = acp.LocalProjectTrust(acp.LocalModelDirectory())
		if err != nil {
			return assistantcontrol.State{}, err
		}
	}
	managed.WorkspaceJSON, err = a.assistantProjectWorkspaceJSON()
	if err != nil {
		return assistantcontrol.State{}, fmt.Errorf("项目工作目录不可用: %w", err)
	}
	if a.assistantConversation != nil {
		currentKind := a.assistantConversation.AgentKind()
		desiredKind := managed.AgentKind
		if desiredKind == "" {
			desiredKind = "codeagent"
		}
		if currentKind != "" && currentKind != desiredKind {
			a.assistantConversation.Close()
			a.assistantConversation = nil
			a.assistantRecovered = true
			a.assistantRecoveryNote = "Agent 底座已切换，将参考近期可见对话继续。"
			recovering = true
		}
	}
	if a.assistantConversation != nil {
		// CustomRuntimeChanged only reports restartable custom/codeagent changes.
		if a.assistantConversation.CustomRuntimeChanged(managed) {
			a.assistantConversation.Close()
			a.assistantConversation = nil
			a.assistantRecovered = true
			a.assistantRecoveryNote = "Agent 配置已变更，将参考近期可见对话继续。"
			recovering = true
		}
	}
	if a.assistantConversation != nil {
		workspace, started := a.assistantConversation.WorkspaceConfig()
		oldSkills, oldConfig := a.assistantConversation.ProjectTrust()
		if started && (oldSkills != managed.ProjectSkillsEnabled || oldConfig != managed.ProjectConfigEnabled) {
			a.assistantConversation.Close()
			a.assistantConversation = nil
			a.assistantRecovered = true
			a.assistantRecoveryNote = "iCode 项目信任设置已变更，将参考近期可见对话继续。"
			recovering = true
			started = false
		}
		if started && workspace != managed.WorkspaceJSON {
			switchCtx, stopSwitch := context.WithTimeout(a.ctx, 30*time.Second)
			switched, switchErr := a.assistantConversation.SwitchWorkspace(switchCtx, managed.WorkspaceJSON)
			stopSwitch()
			if switchErr != nil {
				return assistantcontrol.State{}, switchErr
			}
			if !switched {
				a.assistantConversation.Close()
				a.assistantConversation = nil
				a.assistantRecovered = true
				a.assistantRecoveryNote = "项目工作区已变更，将参考近期可见对话继续；请核实旧项目中的结论。"
				recovering = true
			}
		}
	}
	if client != "" {
		state := a.assistantCoordinator().Snapshot()
		if state.Phase == "running" || state.Phase == "waiting" {
			return assistantcontrol.State{}, assistantcontrol.ErrBusy
		}
		if err := a.workspaceService().Activate(client); err != nil {
			return assistantcontrol.State{}, err
		}
	}
	refs, err = a.resolveAssistantPageReferences(refs)
	if err != nil {
		return assistantcontrol.State{}, err
	}
	for _, ref := range refs {
		if ref.Kind == "terminal" {
			target = terminalcontrol.Target{SessionID: ref.TabID, ConnectionID: ref.ConnectionID}
			break
		}
	}
	// Questions wait for an explicit answer/stop; resource leases and individual
	// operations keep their own bounded deadlines.
	ctx, cancel := context.WithCancel(a.ctx)
	turn, err := a.assistantCoordinator().Begin(ctx, assistantcontrol.Target{}, nil)
	if err != nil {
		cancel()
		return assistantcontrol.State{}, err
	}
	turn.SetSource("assistant")
	turn.SetApprovalMode(managed.ApprovalMode)
	if recovering {
		turn.Record("notice", a.assistantRecoveryNote)
	}
	a.assistantHistory.Begin(turn.ID(), question, refs...)
	a.assistantSelectedModel = managed.LocalModel
	if err = a.saveAssistantHistoryLocked(true, nil); err != nil {
		turn.Finish("failed", "尚未执行操作："+a.assistantSaveWarning)
		a.assistantHistory.Finish(turn.Snapshot())
		cancel()
		return assistantcontrol.State{}, err
	}
	if a.assistantConversation == nil {
		if a.assistantRecovered {
			// Capture history BEFORE this turn's accepted user message so the new
			// instruction is supplied once, at the end of the prompt.
			history := a.assistantHistory.Snapshot()
			history.Messages = history.Messages[:len(history.Messages)-1]
			a.assistantConversation = acp.NewRecoveredConversation(assistantcontrol.RecoveryContext(history), a.assistantResumeSession)
			a.assistantResumeSession = ""
		} else {
			a.assistantConversation = &acp.Conversation{}
		}
	}
	conversation := a.assistantConversation
	var standardReviewer assistantcontrol.Reviewer
	if managed.IsStandardACP() {
		baseReviewer := a.assistantAgentReviewer()
		prior := assistantReviewContext(a.assistantHistory.Snapshot(), turn.ID())
		standardReviewer = func(ctx context.Context, input assistantcontrol.ReviewInput) assistantcontrol.ReviewVerdict {
			input.UserRequest = question
			input.TaskContext = prior
			return baseReviewer(ctx, input)
		}
		conversation.SetStandardReviewer(standardReviewer)
	}
	a.assistantRunning = true
	done := make(chan struct{})
	a.assistantDone = done
	pageContext := ""
	if len(refs) == 0 {
		pageContext = a.assistantPageContext(target)
	} else {
		pageContext = a.assistantReferencedPageContext(refs)
	}
	reviewer := assistantRiskFloor
	if standardReviewer != nil {
		reviewer = standardReviewer
	}
	tools := a.assistantWorkspaceTools(turn, target, managed.ApprovalMode == "bypass", reviewer)
	tools.InitialPageContext = pageContext
	checkpointStop := make(chan struct{})
	go a.checkpointAssistant(turn, checkpointStop)
	go func() {
		defer close(checkpointStop)
		defer cancel()
		defer tools.Close()
		err := conversation.Run(a.ctx, managed, turn, tools, question)
		// Revoke application authority before publishing final completion.
		tools.Close()
		a.assistantConversationMu.Lock()
		defer a.assistantConversationMu.Unlock()
		defer close(done)
		a.assistantRunning = false
		defer func() { a.assistantHistory.Finish(turn.Snapshot()); _ = a.saveAssistantHistoryLocked(false, nil) }()
		if turn.Context().Err() != nil {
			turn.Finish("stopped", "已停止")
			return
		}
		if err != nil {
			turn.Finish("failed", err.Error())
			return
		}
		turn.Finish("completed", "本轮已完成")
	}()
	return turn.Snapshot(), nil
}

// A stopped or failed iCode process cannot be reused. On the next explicit
// message, rebuild only from the bounded visible history; never replay a turn.
// Caller holds assistantConversationMu and has verified that no turn is running.
func (a *App) recoverUnavailableAssistantConversationLocked() bool {
	if a.assistantConversation == nil || !a.assistantConversation.Unusable() {
		return false
	}
	// A standard agent that declares loadSession lets the rebuilt conversation
	// continue its native session; prose recovery remains the fallback.
	a.assistantResumeSession = a.assistantConversation.ResumeTarget()
	a.assistantConversation.Close()
	a.assistantConversation = nil
	a.assistantApprovalMode = ""
	a.assistantRecovered = true
	a.assistantRecoveryNote = "原助手会话已结束，后续消息将参考近期可见对话；此前未完成的操作不会自动重试，当前状态需重新核实。"
	return true
}

func (a *App) RestoreAssistantConversationOnPage(client string) (assistantcontrol.ConversationSnapshot, error) {
	if client == "" {
		return assistantcontrol.ConversationSnapshot{}, errors.New("页面标识无效")
	}
	return a.restoreAssistantConversation(client)
}

// NewAssistantConversation resets model context as well as the visible chat.
// No active execution may be discarded through a clear-history action.
func (a *App) NewAssistantConversation() error {
	a.assistantConversationMu.Lock()
	defer a.assistantConversationMu.Unlock()
	if a.assistantRunning {
		return assistantcontrol.ErrBusy
	}
	state := a.assistantCoordinator().Snapshot()
	if state.Phase == "running" || state.Phase == "waiting" {
		return assistantcontrol.ErrBusy
	}
	_ = a.ensureAssistantHistoryLocked()
	if err := a.resetAssistantHistoryLocked(); err != nil {
		return err
	}
	if a.assistantConversation != nil {
		a.assistantConversation.Close()
		a.assistantConversation = nil
	}
	// Keep the user's selection, but reload its credentials/settings for the
	// next conversation instead of retaining a stale model snapshot forever.
	a.assistantSelectedModel = acp.LocalModel{ID: a.assistantSelectedModel.ID}
	a.assistantApprovalMode = ""
	a.assistantCoordinator().ClearFinished(state.TurnID)
	a.assistantRecoveryNote = ""
	a.assistantRecovered = false
	a.assistantResumeSession = ""
	return nil
}

// RestoreAssistantConversation restores display state, never a pending action.
// A replacement page first revokes/drains the previous turn, including prompts.
func (a *App) RestoreAssistantConversation() (assistantcontrol.ConversationSnapshot, error) {
	return a.restoreAssistantConversation("")
}
func (a *App) restoreAssistantConversation(client string) (assistantcontrol.ConversationSnapshot, error) {
	a.assistantConversationMu.Lock()
	if err := a.ensureAssistantHistoryLocked(); err != nil {
		a.assistantConversationMu.Unlock()
		return assistantcontrol.ConversationSnapshot{}, err
	}
	if client != "" {
		if err := a.workspaceService().Registered(client); err != nil {
			a.assistantConversationMu.Unlock()
			return assistantcontrol.ConversationSnapshot{}, err
		}
		// A passive window may read history, but cannot stop the sender's turn.
		if a.workspaceService().Check(client) != nil {
			snapshot := a.assistantSnapshotLocked()
			a.assistantConversationMu.Unlock()
			return snapshot, nil
		}
	}
	state := a.assistantCoordinator().Snapshot()
	running := a.assistantRunning
	if !running && state.Source != "assistant" {
		// Independent legacy/trial operations share admission, not the Chrys
		// conversation. Reading its history must not cancel or wait on them.
		snapshot := a.assistantSnapshotLocked()
		a.assistantConversationMu.Unlock()
		return snapshot, nil
	}
	a.assistantConversationMu.Unlock()
	if running || state.Phase == "running" || state.Phase == "waiting" {
		if state.Source != "assistant" {
			return assistantcontrol.ConversationSnapshot{}, assistantcontrol.ErrBusy
		}
		if err := a.StopAssistantTurn(state.TurnID); err != nil {
			return assistantcontrol.ConversationSnapshot{}, err
		}
	}
	a.assistantConversationMu.Lock()
	defer a.assistantConversationMu.Unlock()
	state = a.assistantCoordinator().Snapshot()
	if a.assistantRunning || (state.Source == "assistant" && (state.Phase == "running" || state.Phase == "waiting")) {
		return assistantcontrol.ConversationSnapshot{}, assistantcontrol.ErrBusy
	}
	return a.assistantSnapshotLocked(), nil
}

func (a *App) shutdownAssistant(ctx context.Context) {
	state := a.assistantCoordinator().Snapshot()
	if state.TurnID != "" {
		_ = a.StopAssistantTurn(state.TurnID)
	}
	a.assistantConversationMu.Lock()
	conversation := a.assistantConversation
	a.assistantConversation = nil
	a.assistantConversationMu.Unlock()
	if conversation != nil {
		conversation.Close()
	}
	a.shutdownDatabase(ctx)
}

func (a *App) assistantTerminalTools(turn *assistantcontrol.Turn, target terminalcontrol.Target, allowAssessed ...bool) *assistanttools.TerminalTools {
	// The tool adapter serializes calls, so observer creation is lazy and needs
	// no separate mutex. No terminal, audit file or identity probe is opened for chat.
	var observer *terminalcontrol.Observer
	open := func(ctx context.Context, expected *uint64) (*terminalcontrol.Access, error) {
		setup, err := a.terminalControlService().Take(ctx, terminalcontrol.TakeRequest{Target: target, ConfirmedOrdinaryShell: true, ExpectedRevision: expected})
		if err != nil {
			return nil, err
		}
		view, err := setup.Inspect(terminalcontrol.ReadOptions{Limit: 1})
		_ = setup.Release()
		if err != nil {
			return nil, err
		}
		if view.LastPrompt == nil {
			return nil, terminalcontrol.ErrNeedConfirmation
		}
		return a.newTerminalAccess(ctx, terminalcontrol.AccessScope{Actor: "desktop-chrys", RequireActionReview: true, Expires: time.Now().Add(10 * time.Minute), Targets: []terminalcontrol.AccessTarget{{Target: target, UID: view.LastPrompt.UID, RoutineDiagnostics: true}}})
	}
	tools := &assistanttools.TerminalTools{Turn: turn, Target: target, Validate: func(ctx context.Context) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if _, err := a.terminalControlService().Inspect(target, terminalcontrol.ReadOptions{Limit: 1}); err != nil {
			return fmt.Errorf("%w：所选终端已断开或连接已变化。聊天不受影响；操作终端前请重新连接并发送操作要求", assistanttools.ErrSSHUnavailable)
		}
		return turn.BindTarget(assistantcontrol.Target{TabID: target.SessionID, ConnectionID: target.ConnectionID})
	}, Observe: func(ctx context.Context) (terminalcontrol.TerminalContext, error) {
		if ctx.Err() != nil {
			return terminalcontrol.TerminalContext{}, ctx.Err()
		}
		if observer == nil {
			var err error
			observer, err = a.newTerminalObserver(turn.Context(), target)
			if err != nil {
				return terminalcontrol.TerminalContext{}, err
			}
		}
		view, err := observer.Inspect(terminalcontrol.ReadOptions{Limit: 16384})
		if ctx.Err() != nil {
			return terminalcontrol.TerminalContext{}, ctx.Err()
		}
		return view, err
	}, Open: func(ctx context.Context) (*terminalcontrol.Access, error) {
		return open(ctx, nil)
	}}
	if len(allowAssessed) > 0 && allowAssessed[0] {
		tools.OpenAssessed = func(ctx context.Context, revision uint64) (*terminalcontrol.Access, error) {
			return open(ctx, &revision)
		}
	}
	return tools
}
