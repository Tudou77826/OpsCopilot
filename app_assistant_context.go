package main

import (
	"encoding/json"
	"fmt"
	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/terminalcontrol"
)

// Resolve only metadata through the existing resource services. This neither
// reads contents nor acquires ownership. Never retarget a stale reference by name.
func (a *App) resolveAssistantPageReferences(refs []assistantcontrol.PageReference) ([]assistantcontrol.PageReference, error) {
	if err := assistantcontrol.ValidatePageReferences(refs); err != nil {
		return nil, err
	}
	resolved := append([]assistantcontrol.PageReference(nil), refs...)
	for i, ref := range resolved {
		valid := false
		switch ref.Kind {
		case "terminal":
			if _, view, err := a.workspaceService().Current(); err == nil {
				for _, tab := range view.Tabs {
					if tab.TabID == ref.TabID && tab.ConnectionID == ref.ConnectionID && tab.Status == "connected" {
						resolved[i].Title, valid = tab.Title, true
					}
				}
			}
		case "database":
			if w, err := a.databaseControlService().Snapshot(ref.TabID); err == nil && w.Connected && w.Session.ID == ref.ConnectionID {
				resolved[i].Title, valid = w.Profile.Name, true
			}
		case "file":
			if w, err := a.fileControlService().Snapshot(ref.TabID); err == nil && w.Endpoint.ConnectionID == ref.ConnectionID {
				if endpoint, e := a.resolveFileEndpoint(a.ctx, w.Endpoint.SessionID); e == nil && endpoint == w.Endpoint {
					resolved[i].Title, valid = "文件 · "+w.Endpoint.Title, true
				}
			}
		}
		if !valid {
			return nil, fmt.Errorf("引用页签“%s”已关闭、未连接或连接已更换，请移除后重新选择", assistantcontrol.DisplayExcerpt(ref.Title, 120))
		}
		resolved[i].Title = assistantcontrol.DisplayExcerpt(resolved[i].Title, 512)
	}
	return resolved, nil
}

func (a *App) assistantReferencedPageContext(refs []assistantcontrol.PageReference) string {
	// Reuse local project metadata without inheriting whatever tab happens to be
	// active when a queued message starts. References are resource selectors only.
	var page map[string]any
	_ = json.Unmarshal([]byte(a.assistantPageContext(terminalcontrol.Target{})), &page)
	metadata := make([]map[string]any, 0, len(refs))
	for _, ref := range refs {
		item := map[string]any{"kind": ref.Kind, "tabId": ref.TabID, "connectionId": ref.ConnectionID, "title": ref.Title}
		switch ref.Kind {
		case "terminal":
			if cfg, ok := a.getConfig(ref.TabID); ok {
				item["host"], item["port"] = cfg.Host, cfg.Port
			}
		case "file":
			if w, err := a.fileControlService().Snapshot(ref.TabID); err == nil && w.Endpoint.ConnectionID == ref.ConnectionID {
				item["sessionId"], item["localDirectory"], item["remoteDirectory"], item["identity"] = w.Endpoint.SessionID, w.LocalPath, w.RemotePath, w.Endpoint.Identity
			}
		case "database":
			if w, err := a.databaseControlService().Snapshot(ref.TabID); err == nil && w.Session.ID == ref.ConnectionID {
				item["host"], item["port"], item["database"] = w.Profile.Host, w.Profile.Port, w.Session.Database
			}
		}
		metadata = append(metadata, item)
	}
	data, _ := json.Marshal(map[string]any{
		"references": metadata, "localProject": page["localProject"], "localDirectories": page["localDirectories"], "localProjectError": page["localProjectError"],
		"note": "用户明确引用的页签。标题是标签，ID 用于定位；内容尚未读取，引用不授予读取、执行或接管权限。按需使用现有工具核实并读取，不要把本机当作远端。terminal 使用 workspace_list/workspace_select 核对 tabId 和 connectionId；database、file 使用各自工具核对 workspaceId（即 tabId）和连接身份。多个引用不代表要求全部操作；失效时询问，不按同名页签替换。",
	})
	return string(data)
}

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
