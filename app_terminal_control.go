package main

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"opscopilot/pkg/terminalcontrol"
)

func (a *App) terminalControlService() *terminalcontrol.Service {
	a.terminalControlOnce.Do(func() {
		a.terminalControl = terminalcontrol.NewService(func(event terminalcontrol.Audit) {
			// Lifecycle identifiers only: no command text, credentials or output.
			slog.Info("terminal control lifecycle", "session", event.SessionID, "run", event.RunID, "action", event.Action, "phase", event.Phase)
		})
	})
	return a.terminalControl
}

// A screen observer grants no shell initialization or input capability. Its
// audit may record an unknown identity until a real prompt has been observed.
func (a *App) newTerminalObserver(ctx context.Context, target terminalcontrol.Target) (*terminalcontrol.Observer, error) {
	if a.configMgr == nil {
		return nil, errors.New("终端读取需要应用配置目录")
	}
	var cfg ConnectConfig
	valid := false
	a.terminalLifecycles.ApplyCurrent(target.SessionID, func() {
		a.activeConfigsMu.RLock()
		saved, exists := a.activeConfigs[target.SessionID]
		a.activeConfigsMu.RUnlock()
		if !exists {
			return
		}
		if _, err := a.terminalControlService().Inspect(target, terminalcontrol.ReadOptions{Limit: 1}); err != nil {
			return
		}
		cfg = saved
		valid = true
	})
	if !valid {
		return nil, terminalcontrol.ErrUnavailable
	}
	path, err := filepath.Abs(filepath.Join(a.configMgr.Directory(), "audit", "terminal", uuid.NewString()+".jsonl"))
	if err != nil {
		return nil, err
	}
	journal, err := terminalcontrol.NewFileJournal(path, 64<<20)
	if err != nil {
		return nil, err
	}
	return terminalcontrol.NewObserver(ctx, a.terminalControlService(), terminalcontrol.ObservationScope{Actor: "desktop-chrys", Target: target, Node: cfg.Host, SSHUser: cfg.User, Expires: time.Now().Add(10 * time.Minute)}, journal)
}

// Only a trusted application coordinator may mint scopes or retain Approve.
// Agent adapters receive access.Operations(), never this factory or the service.
// No Wails method accepts caller-supplied scopes/approval booleans.
func (a *App) newTerminalAccess(ctx context.Context, scope terminalcontrol.AccessScope) (*terminalcontrol.Access, error) {
	if a.configMgr == nil {
		return nil, errors.New("终端授权需要应用配置目录")
	}
	scope.Targets = append([]terminalcontrol.AccessTarget(nil), scope.Targets...)
	for i := range scope.Targets {
		target := &scope.Targets[i]
		valid := false
		a.terminalLifecycles.ApplyCurrent(target.Target.SessionID, func() {
			a.activeConfigsMu.RLock()
			cfg, exists := a.activeConfigs[target.Target.SessionID]
			a.activeConfigsMu.RUnlock()
			if !exists {
				return
			}
			for _, state := range a.terminalControlService().States() {
				if state.SessionID == target.Target.SessionID && state.ConnectionID == target.Target.ConnectionID {
					target.Node, target.SSHUser = cfg.Host, cfg.User
					valid = true
					break
				}
			}
		})
		if !valid {
			return nil, terminalcontrol.ErrUnavailable
		}
	}
	auditPath, err := filepath.Abs(filepath.Join(a.configMgr.Directory(), "audit", "terminal", uuid.NewString()+".jsonl"))
	if err != nil {
		return nil, err
	}
	journal, err := terminalcontrol.NewFileJournal(auditPath, 64<<20)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	stopApp := context.AfterFunc(a.ctx, cancel)
	access, err := terminalcontrol.NewAccess(ctx, a.terminalControlService(), scope, journal)
	if err != nil {
		stopApp()
		cancel()
		return nil, err
	}
	go func() { <-access.Done(); stopApp(); cancel() }()
	return access, nil
}

// TerminalHandoffStates is the snapshot authority; events alone are not used to
// restore ownership after a page reload or a missed frontend notification.
func (a *App) TerminalHandoffStates() []terminalcontrol.State {
	return a.terminalControlService().States()
}

func (a *App) StartTerminalHandoffTest(sessionID, scenario string, confirmedOrdinaryShell bool, connectionID string) (terminalcontrol.State, error) {
	work := a.AssistantControlState()
	if work.Phase == "running" || work.Phase == "waiting" {
		return terminalcontrol.State{}, errors.New("请先停止或完成当前助手体验")
	}
	a.assistantCoordinator().ClearFinished(work.TurnID)
	return a.terminalControlService().StartForConnection(a.ctx, sessionID, connectionID, scenario, confirmedOrdinaryShell)
}

func (a *App) StopTerminalHandoffTest(sessionID, runID string) error {
	work := a.AssistantControlState()
	if work.Target.TabID == sessionID && (work.Phase == "running" || work.Phase == "waiting") {
		for _, state := range a.terminalControlService().States() {
			if state.SessionID == sessionID && state.ConnectionID == work.Target.ConnectionID && state.RunID == runID {
				return a.StopAssistantTurn(work.TurnID)
			}
		}
	}
	return a.terminalControlService().Stop(sessionID, runID)
}
