package mcp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"opscopilot/pkg/agenttransport/mcp"
	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/assistanttools"
)

func TestReferenceToolsThroughAuthenticatedMCP(t *testing.T) {
	dir := t.TempDir()
	body := "---\nservice: api\nmodule: checkout\n---\n\n## 场景：查询超时\n- **关键词**: E215\n\n检查当前日志。\n"
	if err := os.WriteFile(filepath.Join(dir, "experience.md"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	c := &assistantcontrol.Coordinator{}
	turn, _ := c.Begin(context.Background(), assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	tools := &assistanttools.ReferenceTools{Turn: turn, KnowledgeDirectory: func() string { return dir }, Commands: func() ([]assistanttools.ReferenceCommand, error) {
		return []assistanttools.ReferenceCommand{{Kind: "terminal", ID: "logs", Name: "日志", Enabled: true, Content: "ls /tmp"}}, nil
	}}
	s, err := mcp.Start(context.Background(), tools)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	call := func(name string, args any) map[string]any {
		t.Helper()
		request, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
		_, response := referenceRequestTo(t, s, string(request), "Bearer "+s.Token, "")
		if response["error"] != nil {
			t.Fatalf("MCP failed: %v", response)
		}
		result := response["result"].(map[string]any)
		if result["isError"] == true {
			t.Fatalf("tool failed: %v", result)
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(result["content"].([]any)[0].(map[string]any)["text"].(string)), &payload); err != nil {
			t.Fatal(err)
		}
		return payload["result"].(map[string]any)
	}
	_, response := referenceRequestTo(t, s, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, "Bearer "+s.Token, "")
	names := map[string]bool{}
	for _, item := range response["result"].(map[string]any)["tools"].([]any) {
		d := item.(map[string]any)
		name := d["name"].(string)
		names[name] = true
		if strings.HasPrefix(name, "knowledge_") || strings.HasPrefix(name, "quick_command_") {
			if d["annotations"].(map[string]any)["readOnlyHint"] != true {
				t.Fatal("missing read-only declaration", name)
			}
		}
	}
	for _, name := range mcp.ReferenceToolNames() {
		if !names[name] {
			t.Fatal("not discoverable", name)
		}
	}
	if call("knowledge_list", map[string]any{})["total"] != float64(1) {
		t.Fatal("wrong knowledge source")
	}
	hit := call("knowledge_search", map[string]any{"query": "E215"})["hits"].([]any)[0].(map[string]any)
	if call("knowledge_read", map[string]any{"id": hit["id"]})["source"] != "experience.md" {
		t.Fatal("wrong read source")
	}
	if call("quick_command_list", map[string]any{})["total"] != float64(1) {
		t.Fatal("missing saved commands")
	}
	if call("quick_command_read", map[string]any{"kind": "terminal", "id": "logs"})["command"].(map[string]any)["content"] != "ls /tmp" {
		t.Fatal("command changed")
	}
	for _, name := range mcp.ReferenceToolNames() {
		request, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": map[string]any{"approved": true, "path": "../secret"}}})
		_, response = referenceRequestTo(t, s, string(request), "Bearer "+s.Token, "")
		if response["error"] == nil {
			t.Fatal("authority/path argument accepted", name)
		}
	}
	if turn.Snapshot().Prompt != nil || turn.Snapshot().Target != (assistantcontrol.Target{}) {
		t.Fatal("read requested workspace/approval")
	}
	if _, err := os.Stat(filepath.Join(dir, ".catalog.json")); !os.IsNotExist(err) {
		t.Fatal("read mutated knowledge")
	}
}

func referenceRequestTo(t *testing.T, s *mcp.Server, body, token, origin string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest("POST", s.URL, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", token)
	req.Header.Set("Origin", origin)
	client := http.Client{Timeout: 3 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result map[string]any
	if err = json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, result
}
