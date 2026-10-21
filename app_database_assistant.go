package main

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"time"

	"opscopilot/pkg/agenttransport/mcp"
	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/assistanttools"
	"opscopilot/pkg/dbcontrol"
	"opscopilot/pkg/dbworkspace"
)

func (a *App) databaseControlService() *dbcontrol.Service {
	a.databaseControlOnce.Do(func() {
		dir := "."
		if a.configMgr != nil {
			dir = a.configMgr.Directory()
		}
		a.databaseControl = dbcontrol.New(a.databaseService(), &dbcontrol.FileJournal{Path: filepath.Join(dir, "audit", "database", "executions.jsonl")})
	})
	return a.databaseControl
}

type DatabaseWorkspaceUpdate struct {
	Revision uint64                `json:"revision"`
	Items    []dbcontrol.Workspace `json:"items"`
}

func (a *App) DatabaseWorkspacePoll(client string, since uint64) (DatabaseWorkspaceUpdate, error) {
	if err := a.workspaceService().Check(client); err != nil {
		return DatabaseWorkspaceUpdate{}, err
	}
	revision, items := a.databaseControlService().List(since)
	return DatabaseWorkspaceUpdate{revision, items}, nil
}
func (a *App) DatabaseWorkspacePresented(client, id, nonce, sessionID, sql string) error {
	if err := a.workspaceService().Check(client); err != nil {
		return err
	}
	return a.databaseControlService().Acknowledge(id, nonce, sessionID, sql)
}
func (a *App) DatabaseWorkspaceEdit(id, sql string) error {
	return a.databaseControlService().Edit(id, sql)
}
func (a *App) DatabaseWorkspaceTakeover(id string) error {
	w, err := a.databaseControlService().Snapshot(id)
	if err != nil {
		return err
	}
	if w.OwnerTurn != "" {
		return a.assistantCoordinator().Stop(w.OwnerTurn)
	}
	return nil
}
func (a *App) DatabaseWorkspaceClose(id string) error {
	return a.databaseControlService().Close(context.Background(), id, "")
}
func (a *App) DatabaseWorkspaceDismiss(id string) error {
	if err := a.DatabaseWorkspaceClose(id); err != nil {
		return err
	}
	return a.databaseControlService().Dismiss(id)
}
func (a *App) assistantDatabaseTools(turn *assistantcontrol.Turn) *assistanttools.DatabaseTools {
	return &assistanttools.DatabaseTools{Turn: turn, Service: a.databaseControlService(), List: func(ctx context.Context) (any, error) {
		profiles, err := a.databaseService().List()
		if err != nil {
			return nil, err
		}
		jdbc, err := a.databaseService().JDBCStatus()
		if err != nil {
			return nil, err
		}
		_, items := a.databaseControlService().List(0)
		workspaces := []map[string]any{}
		for _, w := range items {
			if !w.Dismissed {
				workspaces = append(workspaces, map[string]any{"id": w.ID, "profileId": w.Profile.ID, "name": w.Profile.Name, "sessionId": w.Session.ID, "connected": w.Connected, "ownerTurn": w.OwnerTurn})
			}
		}
		return map[string]any{"profiles": profiles, "drivers": jdbc.Drivers, "runtimeAvailable": jdbc.Available, "gateways": a.DatabaseGateways(), "workspaces": workspaces}, nil
	}, Create: func(ctx context.Context, in mcp.DatabaseArgs) (dbworkspace.Profile, error) {
		p := dbworkspace.Profile{Name: in.Name, Engine: "jdbc", DriverID: in.DriverID, Host: in.Host, Port: in.Port, User: in.User, Database: in.Database, GatewayID: in.GatewayID, TLS: in.TLS, GatewayMode: dbworkspace.GatewayModeAuto}
		if p.TLS == "" {
			p.TLS = "off"
		}
		if p.Host == "" {
			p.Host = "127.0.0.1"
		}
		data, _ := json.Marshal(p)
		before, err := a.databaseService().List()
		if err != nil {
			return p, err
		}
		old := map[string]bool{}
		for _, profile := range before {
			old[profile.ID] = true
		}
		answer, err := turn.Request(ctx, assistantcontrol.Prompt{Kind: "database_connection", Title: "补全并保存数据库连接", Description: "助手已填写已知信息；请在连接面板补全密码等内容并保存。密码由应用保管，不会交给模型。", Data: data, Choices: []assistantcontrol.Choice{{ID: "allow", Label: "已保存"}, {ID: "reject", Label: "取消"}}, Expires: time.Now().Add(5 * time.Minute)})
		if err != nil {
			return p, err
		}
		if len(answer.Selected) != 1 || answer.Selected[0] != "allow" || old[answer.Text] {
			return p, errors.New("没有保存新的数据库连接")
		}
		profiles, err := a.databaseService().List()
		if err != nil {
			return p, err
		}
		for _, saved := range profiles {
			if saved.ID == answer.Text {
				return saved, nil
			}
		}
		return p, errors.New("保存的连接不存在")
	}}
}
