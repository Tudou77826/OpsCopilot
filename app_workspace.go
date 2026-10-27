package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/assistanttools"
	"opscopilot/pkg/connectionstore"
	"opscopilot/pkg/remote"
	"opscopilot/pkg/terminalcontrol"
	"opscopilot/pkg/workspace"
)

func (a *App) workspaceService() *workspace.Service {
	a.workspaceOnce.Do(func() { a.workspace = &workspace.Service{} })
	return a.workspace
}
func (a *App) WorkspaceAttach() string {
	a.assistantConversationMu.Lock()
	defer a.assistantConversationMu.Unlock()
	// A replacement page cannot inherit an executing old page's authority.
	state := a.assistantCoordinator().Snapshot()
	if state.Phase == "running" || state.Phase == "waiting" {
		_ = a.assistantCoordinator().Stop(state.TurnID)
	}
	return a.workspaceService().Attach()
}
func (a *App) WorkspaceRegisterPage(key string) (string, error) {
	// Serialize page replacement with submission: a reload must not revoke a
	// newer turn that was accepted between replacing the document and stopping it.
	a.assistantConversationMu.Lock()
	defer a.assistantConversationMu.Unlock()
	client, replaced, err := a.workspaceService().RegisterPage(key)
	if err == nil && replaced {
		state := a.assistantCoordinator().Snapshot()
		if state.Phase == "running" || state.Phase == "waiting" {
			_ = a.assistantCoordinator().Stop(state.TurnID)
		}
	}
	return client, err
}
func (a *App) WorkspaceDetach(client string) {
	a.assistantConversationMu.Lock()
	defer a.assistantConversationMu.Unlock()
	if a.workspaceService().Detach(client) {
		state := a.assistantCoordinator().Snapshot()
		if state.Phase == "running" || state.Phase == "waiting" {
			_ = a.assistantCoordinator().Stop(state.TurnID)
		}
	}
}
func (a *App) WorkspacePull(client string, view workspace.Snapshot) ([]workspace.Presentation, error) {
	return a.workspaceService().Pull(client, view)
}
func (a *App) WorkspaceAcknowledge(client, id string, success bool) error {
	return a.workspaceService().Acknowledge(client, id, success)
}
func (a *App) WorkspaceDiscard(tabID, connectionID string) {
	if tabID == "" || connectionID == "" {
		return
	}
	if a.closeSession(tabID, nil, connectionID) {
		runtime.EventsEmit(a.ctx, "workspace-session-closed", workspace.Target{TabID: tabID, ConnectionID: connectionID})
	}
}
func workspaceTarget(target workspace.Target) terminalcontrol.Target {
	return terminalcontrol.Target{SessionID: target.TabID, ConnectionID: target.ConnectionID}
}
func (a *App) checkWorkspaceTarget(tab workspace.Tab) error {
	_, err := a.terminalControlService().Inspect(workspaceTarget(tab.Target), terminalcontrol.ReadOptions{Limit: 1})
	if err != nil {
		return fmt.Errorf("目标页签已断开或实际连接已改变: %w", err)
	}
	return nil
}
func savedWorkspaceConnections(nodes []*connectionstore.Node) ([]workspace.Connection, map[string]ConnectConfig) {
	public := []workspace.Connection{}
	configs := map[string]ConnectConfig{}
	var visit func([]*connectionstore.Node)
	visit = func(nodes []*connectionstore.Node) {
		for _, node := range nodes {
			if node.Config == nil {
				visit(node.Children)
				continue
			}
			cfg := toAppConfig(*node.Config)
			if cfg.Protocol != "" && cfg.Protocol != "ssh" {
				continue
			}
			var route []workspace.Endpoint
			var walk func(*ConnectConfig)
			walk = func(c *ConnectConfig) {
				if c == nil {
					return
				}
				walk(c.Bastion)
				route = append(route, workspace.Endpoint{Host: c.Host, Port: c.Port, User: c.User})
			}
			walk(&cfg)
			public = append(public, workspace.Connection{ID: node.ID, Name: node.Name, Route: route})
			cfg.Name = node.Name
			configs[node.ID] = cfg
		}
	}
	visit(nodes)
	return public, configs
}
func (a *App) workspaceInventory(context.Context) (workspace.Inventory, error) {
	_, view, err := a.workspaceService().Current()
	if err != nil {
		return workspace.Inventory{}, err
	}
	for i := range view.Tabs {
		if a.checkWorkspaceTarget(view.Tabs[i]) != nil {
			view.Tabs[i].Status = "unavailable"
			view.Tabs[i].Ready = false
		}
	}
	connections := []workspace.Connection{}
	if a.savedSessionMgr != nil {
		connections, _ = savedWorkspaceConnections(a.savedSessionMgr.Snapshot())
	}
	return workspace.Inventory{Snapshot: view, Connections: connections}, nil
}
func (a *App) selectWorkspaceTab(ctx context.Context, id string) (workspace.Tab, error) {
	client, view, err := a.workspaceService().Current()
	if err != nil {
		return workspace.Tab{}, err
	}
	for _, tab := range view.Tabs {
		if tab.TabID == id {
			if err = a.checkWorkspaceTarget(tab); err != nil {
				return workspace.Tab{}, err
			}
			if err = a.workspaceService().Present(ctx, client, "select", tab); err != nil {
				return workspace.Tab{}, err
			}
			if err = a.checkWorkspaceTarget(tab); err != nil {
				return workspace.Tab{}, err
			}
			return tab, nil
		}
	}
	return workspace.Tab{}, errors.New("页签不存在，请重新读取工作台")
}
func (a *App) prepareWorkspaceConnection(_ context.Context, id string) (assistanttools.ConnectionPlan, error) {
	client, _, err := a.workspaceService().Current()
	if err != nil {
		return assistanttools.ConnectionPlan{}, err
	}
	if a.savedSessionMgr == nil {
		return assistanttools.ConnectionPlan{}, errors.New("尚无已保存的 SSH 连接")
	}
	connections, configs := savedWorkspaceConnections(a.savedSessionMgr.Snapshot())
	cfg, ok := configs[id]
	if !ok {
		return assistanttools.ConnectionPlan{}, errors.New("已保存连接不存在或不是 SSH 连接")
	}
	for _, connection := range connections {
		if connection.ID == id {
			// Capture the approved configuration; a concurrent edit cannot change its endpoint.
			return assistanttools.ConnectionPlan{Connection: connection, Open: func(ctx context.Context) (workspace.Tab, error) {
				return a.openWorkspaceConnection(ctx, client, cfg, remote.Dial)
			}}, nil
		}
	}
	return assistanttools.ConnectionPlan{}, errors.New("连接不存在")
}
func (a *App) openWorkspaceConnection(ctx context.Context, client string, cfg ConnectConfig, dial remote.Dialer) (workspace.Tab, error) {
	if err := a.workspaceService().Check(client); err != nil {
		return workspace.Tab{}, err
	}
	id := uuid.NewString()
	type outcome struct {
		tab workspace.Tab
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		committed := false
		var connectionID string
		defer func() {
			if !committed {
				a.WorkspaceDiscard(id, connectionID)
			}
		}()
		result := a.connectTerminalContext(ctx, cfg, id, dial, nil)
		connectionID = result.ConnectionID
		if !result.Success {
			done <- outcome{err: errors.New(result.Message)}
			return
		}
		tab := workspace.Tab{Target: workspace.Target{TabID: id, ConnectionID: result.ConnectionID}, Title: cfg.Name, Status: "connected"}
		if tab.Title == "" {
			tab.Title = cfg.User + "@" + cfg.Host
		}
		if ctx.Err() != nil {
			done <- outcome{err: ctx.Err()}
			return
		}
		if err := a.workspaceService().Present(ctx, client, "open", tab); err != nil {
			done <- outcome{err: err}
			return
		}
		if err := a.checkWorkspaceTarget(tab); err != nil {
			done <- outcome{err: err}
			return
		}
		committed = true // A presented tab is now user-owned data, even if AI subsequently stops.
		tab.Ready = true
		done <- outcome{tab: tab}
	}()
	select {
	case <-ctx.Done():
		return workspace.Tab{}, ctx.Err()
	case result := <-done:
		return result.tab, result.err
	}
}
func (a *App) assistantWorkspaceTools(turn *assistantcontrol.Turn, target terminalcontrol.Target, reviewers ...assistantcontrol.Reviewer) *assistanttools.WorkspaceTools {
	tools := assistanttools.NewWorkspaceTools(turn, target, a.workspaceInventory, a.prepareWorkspaceConnection, a.selectWorkspaceTab, func(target terminalcontrol.Target) *assistanttools.TerminalTools {
		terminal := a.assistantTerminalTools(turn, target)
		if cfg, ok := a.getConfig(target.SessionID); ok {
			terminal.Location = fmt.Sprintf("%s@%s:%d", cfg.User, cfg.Host, cfg.Port)
		}
		if len(reviewers) > 0 {
			terminal.Review = reviewers[0]
		}
		return terminal
	})
	tools.OnOperation = func(action, phase string, target terminalcontrol.Target) {
		// Only identifiers and lifecycle phases belong in application logs.
		slog.Info("assistant workspace operation", "turn", turn.ID(), "action", action, "phase", phase, "tab", target.SessionID, "connection", target.ConnectionID)
	}
	tools.Database = a.assistantDatabaseTools(turn)
	tools.Files = a.assistantFileTools(turn)
	if len(reviewers) > 0 {
		tools.Review = reviewers[0]
		tools.Database.Review = reviewers[0]
		tools.Files.Review = reviewers[0]
	}
	tools.LocalDirectories = &assistanttools.LocalDirectoryTools{Turn: turn, Projects: a.assistantLocalProjectStore()}
	if len(reviewers) > 0 {
		tools.LocalDirectories.Review = reviewers[0]
	}
	tools.References = &assistanttools.ReferenceTools{Turn: turn, KnowledgeDirectory: a.resolveKnowledgeBase, Commands: a.assistantReferenceCommands}
	return tools
}
