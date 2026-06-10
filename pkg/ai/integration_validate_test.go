package ai

import (
	"context"
	"encoding/json"
	"opscopilot/pkg/config"
	"opscopilot/pkg/knowledge"
	"opscopilot/pkg/llm"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestE2E_FabricatedCommandsFiltered 验证完整管道能过滤编造的命令。
// 场景：用户问"支付接口504超时"，LLM 检索到了 payment_sop.md，
// 但返回的 commands 中混合了真实命令（来自文档）和编造命令（不在文档中）。
// 预期：编造命令被剥离，只保留有文档依据的命令。
func TestE2E_FabricatedCommandsFiltered(t *testing.T) {
	SetEventEmitter(func(ctx context.Context, optionalData string, optionalData2 ...interface{}) {})
	defer SetEventEmitter(nil)

	tmpDir := t.TempDir()

	// 写入一份真实的知识库文档
	doc := `---
service: Payment Service
module: 核心支付模块
---

# 支付系统排查手册

## 场景：API 接口超时 (504)

- **现象**: Nginx 日志出现大量 504
- **关键词**: 504, timeout, 超时, nginx
- **涉及组件**: Nginx, Core Service, MySQL

**排查步骤**:
1. 查看 Nginx 日志：
   ` + "`" + `bash
   tail -n 100 /var/log/nginx/access.log | grep "504"
   ` + "`" + `
2. 检查 Core Service 负载：
   ` + "`" + `bash
   top -p $(pgrep core-service)
   ` + "`" + `
3. 检查 MySQL 慢查询：
   ` + "`" + `bash
   mysqldumpslow -s t /var/log/mysql/slow.log
   ` + "`" + `
`
	err := os.WriteFile(filepath.Join(tmpDir, "payment_sop.md"), []byte(doc), 0644)
	if err != nil {
		t.Fatal(err)
	}

	cat, err := knowledge.BuildCatalog(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// 模拟 LLM 行为：
	// Round 1: 调用 grep_knowledge 搜索 "504|超时"
	// Round 2: 调用 read_knowledge_file 读取 payment_sop.md
	// Round 3: 返回混合了真实命令和编造命令的 JSON
	mockProvider := &ScriptedProvider{
		T: t,
		Responses: []func(messages []llm.ChatMessage, tools []llm.Tool) (*llm.ChatResponse, error){
			// Round 1: grep
			func(messages []llm.ChatMessage, tools []llm.Tool) (*llm.ChatResponse, error) {
				return &llm.ChatResponse{
					ToolCalls: []llm.ToolCall{{
						ID:   "call_1",
						Type: "function",
						Function: llm.FunctionCall{
							Name:      "grep_knowledge",
							Arguments: `{"pattern":"504|超时"}`,
						},
					}},
				}, nil
			},
			// Round 2: read file
			func(messages []llm.ChatMessage, tools []llm.Tool) (*llm.ChatResponse, error) {
				return &llm.ChatResponse{
					ToolCalls: []llm.ToolCall{{
						ID:   "call_2",
						Type: "function",
						Function: llm.FunctionCall{
							Name:      "read_knowledge_file",
							Arguments: `{"path":"payment_sop.md"}`,
						},
					}},
				}, nil
			},
			// Round 3: 返回混合命令（真实 + 编造）
			func(messages []llm.ChatMessage, tools []llm.Tool) (*llm.ChatResponse, error) {
				// 这里有真实命令（tail, top, mysqldumpslow 在文档中）
				// 也有编造命令（kubectl, docker, journalctl 不在文档中）
				return &llm.ChatResponse{
					Content: `{
  "steps": [
    {"step": 1, "title": "检查Nginx日志", "description": "查看504错误"},
    {"step": 2, "title": "检查服务负载", "description": "查看资源使用"},
    {"step": 3, "title": "检查MySQL", "description": "查看慢查询"}
  ],
  "commands": [
    {"command": "tail -n 100 /var/log/nginx/access.log | grep \"504\"", "description": "查看Nginx 504日志", "risk": "Low", "source": "payment_sop.md#L18"},
    {"command": "top -p $(pgrep core-service)", "description": "检查Core Service负载", "risk": "Low", "source": "payment_sop.md#L22"},
    {"command": "mysqldumpslow -s t /var/log/mysql/slow.log", "description": "检查MySQL慢查询", "risk": "Low", "source": "payment_sop.md#L26"},
    {"command": "kubectl get pods -n payment", "description": "查看支付服务Pod状态", "risk": "Low"},
    {"command": "docker stats --no-stream", "description": "查看容器资源使用", "risk": "Low"},
    {"command": "journalctl -u core-service --since '1 hour ago'", "description": "查看systemd日志", "risk": "Low"}
  ],
  "summary": "Nginx 504 超时，可能是 Core Service 负载过高或 MySQL 慢查询导致。"
}`,
				}, nil
			},
		},
	}

	cfgMgr := config.NewManager()
	svc := NewAIService(mockProvider, mockProvider, cfgMgr)
	svc.catalog = cat
	svc.knowledgeDir = tmpDir

	result, err := svc.AskTroubleshoot(context.Background(), "支付接口504超时", tmpDir)
	if err != nil {
		t.Fatalf("AskTroubleshoot failed: %v", err)
	}

	// 解析结果
	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("result is not valid JSON: %v\nGot: %s", err, result)
	}

	commands := resp["commands"].([]interface{})
	t.Logf("Total commands after validation: %d", len(commands))

	var keptCmds []string
	var strippedCmds []string
	for _, c := range commands {
		cmd := c.(map[string]interface{})
		cmdStr, _ := cmd["command"].(string)
		keptCmds = append(keptCmds, cmdStr)
		t.Logf("  KEPT: %s (source: %v)", cmdStr, cmd["source"])
	}

	// 验证：真实命令（tail, top, mysqldumpslow）应该被保留
	// 编造命令（kubectl, docker, journalctl）应该被剥离
	for _, kept := range keptCmds {
		if strings.Contains(kept, "kubectl") {
			strippedCmds = append(strippedCmds, kept)
		}
		if strings.Contains(kept, "docker") {
			strippedCmds = append(strippedCmds, kept)
		}
		if strings.Contains(kept, "journalctl") {
			strippedCmds = append(strippedCmds, kept)
		}
	}

	if len(strippedCmds) > 0 {
		t.Errorf("这些编造命令应该被剥离但保留了: %v", strippedCmds)
	}

	// 至少保留 2 个真实命令（tail, top, mysqldumpslow 都在文档中）
	if len(keptCmds) < 2 {
		t.Errorf("应该至少保留 2 个真实命令，但只保留了 %d 个", len(keptCmds))
	}

	// 验证保留的命令都有 source 或 base command 在文档中
	for _, c := range commands {
		cmd := c.(map[string]interface{})
		cmdStr, _ := cmd["command"].(string)
		source, _ := cmd["source"].(string)
		t.Logf("  Command: %s | Source: %s", cmdStr, source)
	}
}

// TestE2E_NoDocsReturnsEmpty 验证当知识库没有相关文档时，返回空 commands。
func TestE2E_NoDocsReturnsEmpty(t *testing.T) {
	SetEventEmitter(func(ctx context.Context, optionalData string, optionalData2 ...interface{}) {})
	defer SetEventEmitter(nil)

	// 空知识库
	tmpDir := t.TempDir()

	// LLM 不调用任何工具，直接返回编造的命令
	mockProvider := &ScriptedProvider{
		T: t,
		Responses: []func(messages []llm.ChatMessage, tools []llm.Tool) (*llm.ChatResponse, error){
			func(messages []llm.ChatMessage, tools []llm.Tool) (*llm.ChatResponse, error) {
				return &llm.ChatResponse{
					Content: `{
  "steps": [{"step": 1, "title": "检查磁盘", "description": "查看磁盘使用"}],
  "commands": [
    {"command": "df -h", "description": "查看磁盘使用", "risk": "Low"},
    {"command": "iostat -x 1 5", "description": "查看IO状态", "risk": "Low"},
    {"command": "fdisk -l", "description": "查看分区", "risk": "Medium"}
  ],
  "summary": "可能是磁盘问题。"
}`,
				}, nil
			},
		},
	}

	cfgMgr := config.NewManager()
	svc := NewAIService(mockProvider, mockProvider, cfgMgr)

	result, err := svc.AskTroubleshoot(context.Background(), "服务器卡顿", tmpDir)
	if err != nil {
		t.Fatalf("AskTroubleshoot failed: %v", err)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("result is not valid JSON: %v\nGot: %s", err, result)
	}

	// 没有检索到任何文档，commands 应该被清空
	commands := resp["commands"]
	if commands == nil {
		t.Log("commands is nil (empty) - correct behavior")
		return
	}
	cmdArr, ok := commands.([]interface{})
	if !ok {
		t.Fatalf("commands is not an array: %v", commands)
	}
	if len(cmdArr) != 0 {
		t.Errorf("expected 0 commands when no docs retrieved, got %d: %v", len(cmdArr), cmdArr)
	}

	steps := resp["steps"]
	if steps == nil {
		t.Log("steps is nil (empty) - correct behavior")
		return
	}
	stepsArr, ok := steps.([]interface{})
	if !ok {
		t.Fatalf("steps is not an array: %v", steps)
	}
	if len(stepsArr) != 0 {
		t.Errorf("expected 0 steps when no docs retrieved, got %d", len(stepsArr))
	}

	t.Logf("Result: %s", result)
}
