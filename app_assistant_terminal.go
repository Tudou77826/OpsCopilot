package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
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
	question = strings.TrimSpace(question)
	if question == "" || len(question) > 16384 || !utf8.ValidString(question) {
		return assistantcontrol.State{}, errors.New("请输入不超过 16 KB 的问题")
	}
	target := terminalcontrol.Target{SessionID: sessionID, ConnectionID: connectionID}
	executable, err := findChrysExecutable()
	if err != nil {
		return assistantcontrol.State{}, err
	}
	llm := a.configMgr.Config.LLM
	model := llm.ComplexModel
	if model == "" {
		model = llm.Model
	}
	if model == "" {
		model = llm.FastModel
	}
	if model == "" || llm.BaseURL == "" {
		return assistantcontrol.State{}, errors.New("请先在模型服务中配置模型和服务地址")
	}
	a.assistantConversationMu.Lock()
	defer a.assistantConversationMu.Unlock()
	if a.assistantRunning {
		return assistantcontrol.State{}, assistantcontrol.ErrBusy
	}
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Minute)
	turn, err := a.assistantCoordinator().Begin(ctx, assistantcontrol.Target{}, nil)
	if err != nil {
		cancel()
		return assistantcontrol.State{}, err
	}
	turn.SetSource("assistant")
	a.assistantHistory.Begin(turn.ID(), question)
	if a.assistantConversation == nil {
		a.assistantConversation = &acp.Conversation{}
	}
	conversation := a.assistantConversation
	a.assistantRunning = true
	done := make(chan struct{})
	a.assistantDone = done
	tools := a.assistantWorkspaceTools(turn, target)
	go func() {
		defer cancel()
		defer tools.Close()
		err := conversation.Run(a.ctx, acp.ManagedConfig{Executable: executable, BaseURL: llm.BaseURL, APIKey: llm.APIKey, Model: model, SkillsDirectory: filepath.Join(a.configMgr.Directory(), "assistant-skills"), AuditDirectory: filepath.Join(a.configMgr.Directory(), "audit", "assistant")}, turn, tools, question)
		// Revoke application authority before publishing final completion.
		tools.Close()
		a.assistantConversationMu.Lock()
		defer a.assistantConversationMu.Unlock()
		defer close(done)
		a.assistantRunning = false
		defer func() { a.assistantHistory.Finish(turn.Snapshot()) }()
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
	if a.assistantConversation != nil {
		a.assistantConversation.Close()
		a.assistantConversation = nil
	}
	a.assistantCoordinator().ClearFinished(state.TurnID)
	a.assistantHistory.Reset()
	return nil
}

// RestoreAssistantConversation restores display state, never a pending action.
// A replacement page first revokes/drains the previous turn, including prompts.
func (a *App) RestoreAssistantConversation() (assistantcontrol.ConversationSnapshot, error) {
	a.assistantConversationMu.Lock()
	state := a.assistantCoordinator().Snapshot()
	running := a.assistantRunning
	if !running && state.Source != "assistant" {
		// Independent legacy/trial operations share admission, not the Chrys
		// conversation. Reading its history must not cancel or wait on them.
		snapshot := a.assistantHistory.Snapshot()
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
	return a.assistantHistory.Snapshot(), nil
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

func (a *App) assistantTerminalTools(turn *assistantcontrol.Turn, target terminalcontrol.Target) *assistanttools.TerminalTools {
	// The tool adapter serializes calls, so observer creation is lazy and needs
	// no separate mutex. No terminal, audit file or identity probe is opened for chat.
	var observer *terminalcontrol.Observer
	return &assistanttools.TerminalTools{Turn: turn, Target: target, Validate: func(ctx context.Context) error {
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
		// Human consent has just confirmed an idle ordinary shell. Bootstrap
		// only that original PTY, then bind the observed identity, not SSH login UID.
		setup, err := a.terminalControlService().Take(ctx, terminalcontrol.TakeRequest{Target: target, ConfirmedOrdinaryShell: true})
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
		return a.newTerminalAccess(ctx, terminalcontrol.AccessScope{Actor: "desktop-chrys", Expires: time.Now().Add(10 * time.Minute), Targets: []terminalcontrol.AccessTarget{{Target: target, UID: view.LastPrompt.UID, RoutineDiagnostics: true}}})
	}}
}

func findChrysExecutable() (string, error) {
	if configured := os.Getenv("OPSCOPILOT_CHRYS_EXECUTABLE"); configured != "" {
		if !filepath.IsAbs(configured) {
			return "", errors.New("Chrys 程序路径必须是绝对路径")
		}
		info, err := os.Stat(configured)
		if err != nil || info.IsDir() {
			return "", errors.New("找不到指定的 Chrys 程序")
		}
		return configured, nil
	}
	if executable, err := os.Executable(); err == nil {
		for _, name := range []string{"chrys.exe", "chrys"} {
			candidate := filepath.Join(filepath.Dir(executable), "chrys", name)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
		}
	}
	if executable, err := exec.LookPath("chrys"); err == nil {
		return filepath.Abs(executable)
	}
	return "", errors.New("尚未安装 Chrys 运行组件。当前开发版可通过 OPSCOPILOT_CHRYS_EXECUTABLE 指定程序路径")
}
