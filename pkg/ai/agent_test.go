package ai

import (
	"context"
	"opscopilot/pkg/config"
	"opscopilot/pkg/knowledge"
	"opscopilot/pkg/llm"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ScriptedProvider allows us to define a sequence of responses based on input
type ScriptedProvider struct {
	T         *testing.T
	Responses []func(messages []llm.ChatMessage, tools []llm.Tool) (*llm.ChatResponse, error)
	CallCount int
}

func (p *ScriptedProvider) ChatCompletion(ctx context.Context, messages []llm.ChatMessage) (string, error) {
	resp, err := p.ChatWithTools(ctx, messages, nil)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

func (p *ScriptedProvider) ChatWithTools(ctx context.Context, messages []llm.ChatMessage, tools []llm.Tool) (*llm.ChatResponse, error) {
	if p.CallCount >= len(p.Responses) {
		p.T.Fatalf("Unexpected call to ChatWithTools (Count: %d)", p.CallCount)
	}
	handler := p.Responses[p.CallCount]
	p.CallCount++
	return handler(messages, tools)
}

func TestAgentLoop(t *testing.T) {
	SetEventEmitter(func(ctx context.Context, optionalData string, optionalData2 ...interface{}) {})
	defer SetEventEmitter(nil)

	// 1. Setup Test Knowledge Base
	tmpDir, err := os.MkdirTemp("", "agent_test_docs")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	err = os.WriteFile(filepath.Join(tmpDir, "network.md"), []byte(`---
service: Network Service
module: 基础网络
---

# Network Troubleshooting

## 场景：网络延迟高

- **现象**: 网络响应慢，ping 延迟高
- **关键词**: network, slow, ping, 延迟, 慢
- **涉及组件**: 路由器, 交换机

Check ping and traceroute.
`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// 构建 catalog
	cat, err := knowledge.BuildCatalog(tmpDir)
	if err != nil {
		t.Fatalf("BuildCatalog error: %v", err)
	}

	// 2. Define Test Script
	// 新流程（2轮LLM调用）:
	//   Round 1: LLM 看到目录上下文 → 调用 read_knowledge_file
	//   Round 2: LLM 看到文件内容 → 生成最终回答

	mockProvider := &ScriptedProvider{
		T: t,
		Responses: []func(messages []llm.ChatMessage, tools []llm.Tool) (*llm.ChatResponse, error){
			// Round 1: LLM 看到带目录的系统提示，决定读取文件
			func(messages []llm.ChatMessage, tools []llm.Tool) (*llm.ChatResponse, error) {
				// 验证系统提示包含目录
				systemMsg := messages[0]
				if systemMsg.Role != "system" {
					t.Errorf("Round 1: first message should be system, got %s", systemMsg.Role)
				}
				if !strings.Contains(systemMsg.Content, "知识库问题目录") {
					t.Errorf("Round 1: system prompt should contain catalog, got: %s", systemMsg.Content[:min(200, len(systemMsg.Content))])
				}
				if !strings.Contains(systemMsg.Content, "Network Service") {
					t.Errorf("Round 1: system prompt should contain 'Network Service'")
				}

				// 验证工具列表只有 read_knowledge_file
				foundReadTool := false
				for _, tool := range tools {
					if tool.Function.Name == "read_knowledge_file" {
						foundReadTool = true
					}
					if tool.Function.Name == "search_knowledge" || tool.Function.Name == "list_knowledge_files" {
						t.Errorf("Round 1: should NOT have search/list tools, got %s", tool.Function.Name)
					}
				}
				if !foundReadTool {
					t.Error("Round 1: should have read_knowledge_file tool")
				}

				return &llm.ChatResponse{
					Content: "Reading...",
					ToolCalls: []llm.ToolCall{
						{
							ID:   "call_1",
							Type: "function",
							Function: llm.FunctionCall{
								Name:      "read_knowledge_file",
								Arguments: `{"path": "network.md", "section": "网络延迟高"}`,
							},
						},
					},
				}, nil
			},
			// Round 2: LLM 生成最终回答
			func(messages []llm.ChatMessage, tools []llm.Tool) (*llm.ChatResponse, error) {
				// 验证收到了工具结果
				lastMsg := messages[len(messages)-1]
				if lastMsg.Role != "tool" || lastMsg.Name != "read_knowledge_file" {
					t.Errorf("Round 2: Expected tool output for read_knowledge_file, got %v", lastMsg)
				}
				if !strings.Contains(lastMsg.Content, "ping") && !strings.Contains(lastMsg.Content, "场景") {
					t.Errorf("Round 2: Expected content to contain 'ping' or '场景', got %s", lastMsg.Content)
				}

				return &llm.ChatResponse{
					Content:   "You should check ping and traceroute.",
					ToolCalls: nil,
				}, nil
			},
		},
	}

	// 3. Initialize Service
	cfgMgr := config.NewManager()
	svc := NewAIService(mockProvider, mockProvider, cfgMgr)

	// 4. Run Agent
	answer, err := svc.RunAgent(context.Background(), AgentRunOptions{
		Question:     "network slow",
		KnowledgeDir: tmpDir,
		SystemPrompt: "",
		RetryMax:     5,
		Catalog:      cat,
	})
	if err != nil {
		t.Fatalf("Agent run failed: %v", err)
	}

	// 5. Verify Result
	if answer != "You should check ping and traceroute." {
		t.Errorf("Expected 'You should check ping and traceroute.', got '%s'", answer)
	}

	if mockProvider.CallCount != 2 {
		t.Errorf("Expected 2 LLM calls (down from 4), got %d", mockProvider.CallCount)
	}
}

func TestAgentCatalogInjection(t *testing.T) {
	SetEventEmitter(func(ctx context.Context, optionalData string, optionalData2 ...interface{}) {})
	defer SetEventEmitter(nil)

	// 构建一个带条目的 catalog
	cat := &knowledge.Catalog{
		Services: []knowledge.ServiceEntry{
			{
				Name: "Payment Service",
				Modules: []knowledge.ModuleEntry{
					{
						Name: "核心支付模块",
						Scenarios: []knowledge.ScenarioEntry{
							{
								Title:     "支付超时",
								File:      "payment.md",
								LineStart: 1,
								LineEnd:   5,
								Phenomena: "支付接口504",
								Keywords:  []string{"504", "timeout", "支付"},
								Type:      "sop",
							},
						},
					},
				},
			},
		},
	}

	mockProvider := &ScriptedProvider{
		T: t,
		Responses: []func(messages []llm.ChatMessage, tools []llm.Tool) (*llm.ChatResponse, error){
			func(messages []llm.ChatMessage, tools []llm.Tool) (*llm.ChatResponse, error) {
				systemMsg := messages[0]
				// 验证目录被注入
				if !strings.Contains(systemMsg.Content, "Payment Service") {
					t.Errorf("system prompt should contain 'Payment Service', got: %s", systemMsg.Content)
				}
				if !strings.Contains(systemMsg.Content, "支付超时") {
					t.Errorf("system prompt should contain scenario title '支付超时'")
				}

				// 直接回答（不调用工具）
				return &llm.ChatResponse{
					Content:   "请检查支付网关配置。",
					ToolCalls: nil,
				}, nil
			},
		},
	}

	cfgMgr := config.NewManager()
	svc := NewAIService(mockProvider, mockProvider, cfgMgr)

	answer, err := svc.RunAgent(context.Background(), AgentRunOptions{
		Question:     "支付超时怎么办",
		KnowledgeDir: "",
		RetryMax:     5,
		Catalog:      cat,
	})
	if err != nil {
		t.Fatalf("Agent run failed: %v", err)
	}

	if answer != "请检查支付网关配置。" {
		t.Errorf("Expected '请检查支付网关配置。', got '%s'", answer)
	}

	if mockProvider.CallCount != 1 {
		t.Errorf("Expected 1 LLM call, got %d", mockProvider.CallCount)
	}
}

func TestAgentWithoutCatalog(t *testing.T) {
	SetEventEmitter(func(ctx context.Context, optionalData string, optionalData2 ...interface{}) {})
	defer SetEventEmitter(nil)

	tmpDir, err := os.MkdirTemp("", "agent_test_no_cat")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	err = os.WriteFile(filepath.Join(tmpDir, "test.md"), []byte("# Test\nSome content."), 0644)
	if err != nil {
		t.Fatal(err)
	}

	mockProvider := &ScriptedProvider{
		T: t,
		Responses: []func(messages []llm.ChatMessage, tools []llm.Tool) (*llm.ChatResponse, error){
			func(messages []llm.ChatMessage, tools []llm.Tool) (*llm.ChatResponse, error) {
				systemMsg := messages[0]
				// 无 catalog 时不应包含 "知识库问题目录" 这个标题
				if strings.Contains(systemMsg.Content, "## 知识库问题目录") {
					t.Error("system prompt should NOT contain catalog header when Catalog is nil")
				}
				// 仍然应该有 read_knowledge_file 工具
				foundReadTool := false
				for _, tool := range tools {
					if tool.Function.Name == "read_knowledge_file" {
						foundReadTool = true
					}
				}
				if !foundReadTool {
					t.Error("should still have read_knowledge_file tool")
				}

				return &llm.ChatResponse{
					Content:   "知识库暂无相关文档。",
					ToolCalls: nil,
				}, nil
			},
		},
	}

	cfgMgr := config.NewManager()
	svc := NewAIService(mockProvider, mockProvider, cfgMgr)

	answer, err := svc.RunAgent(context.Background(), AgentRunOptions{
		Question:     "test question",
		KnowledgeDir: tmpDir,
		RetryMax:     5,
		Catalog:      nil, // 无 catalog
	})
	if err != nil {
		t.Fatalf("Agent run failed: %v", err)
	}

	if answer != "知识库暂无相关文档。" {
		t.Errorf("Expected fallback answer, got '%s'", answer)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
