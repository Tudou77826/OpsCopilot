package mcp

import (
	"encoding/json"
	"strings"
)

func LocalDirectoryToolNames() []string {
	return []string{"local_directories", "local_add_directories"}
}
func localDirectoryDefinitions() []any {
	return []any{
		map[string]any{"name": "local_directories", "description": "List saved LOCAL projects and their directory groups, with activeId identifying the user's current project. Use its directories unless the user explicitly names another project. Project configuration persists across conversations/restarts. These are working locations, not sandbox boundaries or blanket permission. Does not read directory contents or require SSH.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}},
		map[string]any{"name": "local_add_directories", "description": "Request one or more existing absolute LOCAL directories to be SAVED in one explicit project_id from local_directories. Host confirms project name, exact canonical paths and purpose. Does not switch the user's selected project, scan contents, grant read/write access or execute commands. Use returned absolute paths with native local tools under existing Chrys review. If no project exists, ask the user to create/select one. Do not invent IDs or retry denial/stop.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"project_id": map[string]any{"type": "string", "minLength": 1, "maxLength": 128}, "paths": map[string]any{"type": "array", "items": map[string]any{"type": "string", "minLength": 1, "maxLength": 2048}, "minItems": 1, "maxItems": 16}, "purpose": map[string]any{"type": "string", "minLength": 1, "maxLength": 512}}, "required": []string{"project_id", "paths", "purpose"}, "additionalProperties": false}},
	}
}
func parseLocalDirectoryArguments(name string, args map[string]json.RawMessage) (string, bool) {
	if name == "local_directories" {
		return "{}", len(args) == 0
	}
	var paths []string
	var purpose string
	var projectID string
	if name != "local_add_directories" || len(args) != 3 || json.Unmarshal(args["project_id"], &projectID) != nil || strings.TrimSpace(projectID) == "" || len(projectID) > 128 || strings.ContainsAny(projectID, "\r\n\x00") || json.Unmarshal(args["paths"], &paths) != nil || len(paths) == 0 || len(paths) > 16 || json.Unmarshal(args["purpose"], &purpose) != nil || strings.TrimSpace(purpose) == "" || len(purpose) > 512 {
		return "", false
	}
	for _, p := range paths {
		if p == "" || len(p) > 2048 || strings.ContainsAny(p, "\x00\r\n") {
			return "", false
		}
	}
	data, _ := json.Marshal(args)
	return string(data), true
}
