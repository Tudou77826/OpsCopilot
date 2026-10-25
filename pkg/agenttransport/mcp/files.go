package mcp

import (
	"encoding/json"
	"strings"
)

type FileArgs struct {
	WorkspaceID string `json:"workspace_id,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	Path        string `json:"path,omitempty"`
	Side        string `json:"side,omitempty"`
	Direction   string `json:"direction,omitempty"`
	LocalPath   string `json:"local_path,omitempty"`
	RemotePath  string `json:"remote_path,omitempty"`
	TaskID      string `json:"task_id,omitempty"`
	Purpose     string `json:"purpose,omitempty"`
}

var fileFields = map[string][]string{
	"file_workspaces": {}, "file_open": {"session_id"}, "file_take": {"workspace_id"}, "file_release": {}, "file_list": {"side", "path"}, "file_transfer": {"direction", "local_path", "remote_path", "purpose"}, "file_tasks": {}, "file_cancel": {"task_id"},
}

func FileToolNames() []string {
	return []string{"file_workspaces", "file_open", "file_take", "file_list", "file_transfer", "file_tasks", "file_cancel", "file_release"}
}
func fileDefinitions() []any {
	descriptions := map[string]string{
		"file_workspaces": "List visible file workspaces, transfer jobs and connected SSH session IDs without credentials or file contents. Use existing session IDs exactly; workspace_open can establish a saved SSH connection first.",
		"file_open":       "Open/present a full file-transfer tab bound to one connected SSH session. Reuses the normal file transport identity, which can differ from a terminal su identity; host displays it and asks consent. Does not take ownership.",
		"file_take":       "Explicitly protect a file workspace during a file management sequence. Never take for chat. Terminal/database authority is released. File operations do not run in the terminal shell.",
		"file_list":       "Navigate the owned file tab and list one absolute local or remote directory. side must be local or remote. Metadata only, at most 4096 entries. Use terminal grep/zgrep/tail for log diagnosis; file tools are for file management and transfer, not log analysis.",
		"file_transfer":   "Start one FILE upload/download in the owned visible file tab, after exact host confirmation of source/destination and possible overwrite. Both paths must be absolute full file paths, not directories. No installation, execution, extraction or deletion is implied. Result is a task ID, NOT completion: inspect file_tasks. Never retry unknown outcomes. On stop/release unfinished transfers are cancelled and may leave a partial destination.",
		"file_tasks":      "Inspect actual task states on the current file workspace. running/cancelling are not success; done means transport completion only. No automatic retries.",
		"file_cancel":     "Cancel one task created by this turn, never a human's transfer. Inspect file_tasks for its final result and partial-file warning.",
		"file_release":    "Cancel/drain unfinished transfers and release file-tab ownership before chatting. Complete jobs and visible directories remain for the user. Wait for done tasks before release if you want transfers to finish.",
	}
	out := []any{}
	for _, name := range FileToolNames() {
		props := map[string]any{}
		for _, f := range fileFields[name] {
			props[f] = map[string]any{"type": "string", "minLength": 1, "maxLength": 2048}
		}
		out = append(out, map[string]any{"name": name, "description": descriptions[name], "inputSchema": map[string]any{"type": "object", "properties": props, "required": fileFields[name], "additionalProperties": false}})
	}
	return out
}
func parseFileArguments(name string, args map[string]json.RawMessage) (string, bool) {
	fields, ok := fileFields[name]
	if !ok || len(args) != len(fields) {
		return "", false
	}
	for _, f := range fields {
		var v string
		if json.Unmarshal(args[f], &v) != nil || strings.TrimSpace(v) == "" || len(v) > 2048 || strings.ContainsAny(v, "\x00\r\n") {
			return "", false
		}
	}
	data, _ := json.Marshal(args)
	return string(data), true
}
