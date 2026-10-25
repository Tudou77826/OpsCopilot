package main

import (
	"encoding/json"
	"opscopilot/pkg/terminalcontrol"
)

// Only host-selected resource metadata enters the per-message context. Never
// serialize connection configs, passwords, terminal contents or query results.
func (a *App) assistantPageContext(target terminalcontrol.Target) string {
	context := map[string]any{"kind": "none", "note": "本轮没有选中的远程资源；不要把含糊的‘这台机器’自动理解为本机，必要时询问。"}
	_, view, err := a.workspaceService().Current()
	if err == nil {
		if target.SessionID == "" && view.Active.TabID != "" {
			for _, tab := range view.Tabs {
				if tab.TabID == view.Active.TabID && tab.Status != "connected" {
					context = map[string]any{"kind": "remote-unavailable", "title": tab.Title, "connected": false, "note": "当前页是未连接的远程终端。不要改为检查本机；需要远程操作时请先确认并建立连接。"}
				}
			}
		}
		for _, tab := range view.Tabs {
			if tab.TabID == target.SessionID && tab.ConnectionID == target.ConnectionID {
				if cfg, ok := a.getConfig(tab.TabID); ok {
					context = map[string]any{"kind": "remote-terminal", "title": tab.Title, "host": cfg.Host, "port": cfg.Port, "sshUser": cfg.User, "sessionId": tab.TabID, "connectionId": tab.ConnectionID, "connected": tab.Status == "connected"}
				}
			}
		}
		if view.ActiveWorkspace != "" && target.SessionID == "" {
			context = map[string]any{"kind": "workspace", "workspaceId": view.ActiveWorkspace}
			if w, e := a.fileControlService().Snapshot(view.ActiveWorkspace); e == nil {
				context = map[string]any{"kind": "remote-files", "workspaceId": w.ID, "title": w.Endpoint.Title, "sessionId": w.Endpoint.SessionID, "connectionId": w.Endpoint.ConnectionID, "localDirectory": w.LocalPath, "remoteDirectory": w.RemotePath, "identity": w.Endpoint.Identity}
			}
		}
	}
	context["localDirectories"] = []string{}
	context["localProject"] = nil
	if projects, err := a.AssistantLocalProjects(); err == nil {
		if p, ok := projects.ActiveProject(); ok {
			context["localProject"] = map[string]string{"id": p.ID, "name": p.Name}
			context["localDirectories"] = p.Directories
		}
	} else {
		context["localProjectError"] = err.Error()
	}
	data, _ := json.Marshal(context)
	return string(data)
}
