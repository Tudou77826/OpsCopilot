package mcp

import (
	"encoding/json"
	"net/netip"
	"strings"
	"unicode"
)

// SSHConnectionGuide is shared by standard ACP and the managed agent profile.
const SSHConnectionGuide = `For SSH requests, workspace_list, workspace_create and workspace_open work even when no remote terminal is connected. Start with workspace_list: it returns saved SSH profiles (connection_id) and open terminals (tab_id). For a saved profile call workspace_open with its exact ID to establish a fresh visible SSH terminal. If the requested host has no saved profile, call workspace_create with the known host and optional name, port and username; the user completes and saves the SSH form directly in OpsCopilot. It returns connection_id. When the user asked to connect, follow with workspace_open; for a save-only request stop after creation. Use workspace_select to reuse a specific already-open terminal. Ask only if the target is genuinely ambiguous. Never ask for credentials in chat or tool arguments. The host handles connection approval. Opening a terminal does not grant screen access or command ownership. If a terminal command has no target, discover or open the requested resource through these tools in the same turn. If a tool was rejected or its outcome is uncertain, do not blindly repeat it.`

type SSHConnectionDraft struct {
	Host string `json:"host"`
	Name string `json:"name,omitempty"`
	User string `json:"user,omitempty"`
	Port int    `json:"port,omitempty"`
}

func WorkspaceToolNames() []string {
	return []string{"workspace_list", "workspace_create", "workspace_open", "workspace_select", "terminal_context", "terminal_take", "terminal_release", "terminal_execute"}
}

func ToolNames() []string {
	names := WorkspaceToolNames()
	names = append(names, DatabaseToolNames()...)
	names = append(names, FileToolNames()...)
	names = append(names, LocalDirectoryToolNames()...)
	return append(names, ReferenceToolNames()...)
}

func workspaceCreateDefinition() map[string]any {
	return map[string]any{
		"name":        "workspace_create",
		"description": "Create a saved SSH connection when workspace_list has no matching profile. Works without an open terminal. Prefill the known host and optional name, port and username. The user completes and saves the secure SSH form in OpsCopilot. Returns connection_id; call workspace_open with it if the user asked to connect. The form is the only place for credentials.",
		"inputSchema": map[string]any{"type": "object", "required": []string{"host"}, "additionalProperties": false, "properties": map[string]any{
			"host": map[string]any{"type": "string", "minLength": 1, "maxLength": 255, "description": "Bare hostname or IP address, without scheme, credentials or port."},
			"name": map[string]any{"type": "string", "maxLength": 128},
			"user": map[string]any{"type": "string", "maxLength": 128},
			"port": map[string]any{"type": "integer", "minimum": 1, "maximum": 65535},
		}},
	}
}

func parseWorkspaceCreateArguments(args map[string]json.RawMessage) (string, bool) {
	for key, value := range args {
		switch key {
		case "host", "port", "name", "user":
			if string(value) == "null" {
				return "", false
			}
		default:
			return "", false
		}
	}
	data, _ := json.Marshal(args)
	var draft SSHConnectionDraft
	if json.Unmarshal(data, &draft) != nil || draft.Host == "" || len(draft.Host) > 255 || len(draft.User) > 128 || len(draft.Name) > 128 ||
		draft.Port < 0 || draft.Port > 65535 || (args["port"] != nil && draft.Port == 0) ||
		strings.ContainsAny(draft.Host, "/\\@?#") || strings.ContainsFunc(draft.Host, unicode.IsSpace) ||
		strings.ContainsFunc(draft.Host+draft.Name+draft.User, unicode.IsControl) {
		return "", false
	}
	if strings.ContainsAny(draft.Host, ":[]") {
		host := strings.TrimSuffix(strings.TrimPrefix(draft.Host, "["), "]")
		if _, err := netip.ParseAddr(host); err != nil {
			return "", false
		}
	}
	return string(data), true
}
