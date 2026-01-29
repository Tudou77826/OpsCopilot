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
	// Disable Wails Emit for tests
	SetEventEmitter(func(ctx context.Context, optionalData string, optionalData2 ...interface{}) {
		// Do nothing
	})
	defer SetEventEmitter(nil) // Reset or set back to original if accessible, but here just nil is risky if tests run parallel.
	// Better: Set a mock that logs?
	// Or just do nothing.

	// 1. Setup Test Knowledge Base
	tmpDir, err := os.MkdirTemp("", "agent_test_docs")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	err = os.WriteFile(filepath.Join(tmpDir, "network.md"), []byte("# Network Troubleshooting\nCheck ping."), 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(tmpDir, "payment.md"), []byte("# 支付系统排查\n支付超时常见原因：网关异常、DNS、路由。"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Define Test Script
	// Scenario: User asks "network slow".
	// Step 1: Agent calls SearchKnowledge
	// Step 2: Agent (internal) calls LLM to extract weighted terms
	// Step 3: Agent calls ReadFile("network.md")
	// Step 4: Agent answers

	mockProvider := &ScriptedProvider{
		T: t,
		Responses: []func(messages []llm.ChatMessage, tools []llm.Tool) (*llm.ChatResponse, error){
			// Round 1: Expect User Question, Return ToolCall SearchKnowledge
			func(messages []llm.ChatMessage, tools []llm.Tool) (*llm.ChatResponse, error) {
				lastMsg := messages[len(messages)-1]
				if lastMsg.Role != "user" {
					t.Errorf("Round 1: Expected user message, got %s", lastMsg.Role)
				}
				return &llm.ChatResponse{
					Content: "Thinking...",
					ToolCalls: []llm.ToolCall{
						{
							ID:   "call_1",
							Type: "function",
							Function: llm.FunctionCall{
								Name:      knowledge.ToolSearch,
								Arguments: `{"query":"network slow","top_k":5}`,
							},
						},
					},
				}, nil
			},
			// Round 2: Keyword extraction ChatCompletion (no tools)
			func(messages []llm.ChatMessage, tools []llm.Tool) (*llm.ChatResponse, error) {
				if tools != nil && len(tools) != 0 {
					t.Errorf("Round 2: Expected no tools, got %d", len(tools))
				}
				lastMsg := messages[len(messages)-1]
				if lastMsg.Role != "user" {
					t.Errorf("Round 2: Expected user message, got %s", lastMsg.Role)
				}
				return &llm.ChatResponse{
					Content: `[{"term":"network","weight":5},{"term":"ping","weight":3}]`,
				}, nil
			},
			// Round 3: Expect Tool Output (Search hits), Return ToolCall ReadFile
			func(messages []llm.ChatMessage, tools []llm.Tool) (*llm.ChatResponse, error) {
				lastMsg := messages[len(messages)-1]
				if lastMsg.Role != "tool" || lastMsg.Name != knowledge.ToolSearch {
					t.Errorf("Round 3: Expected tool output for SearchKnowledge, got %v", lastMsg)
				}
				if !strings.Contains(lastMsg.Content, "network.md") {
					t.Errorf("Round 3: Expected search hits to contain network.md, got %s", lastMsg.Content)
				}

				return &llm.ChatResponse{
					Content: "Reading file...",
					ToolCalls: []llm.ToolCall{
						{
							ID:   "call_2",
							Type: "function",
							Function: llm.FunctionCall{
								Name:      knowledge.ToolReadFile,
								Arguments: `{"path": "network.md"}`,
							},
						},
					},
				}, nil
			},
			// Round 4: Expect Tool Output (Content), Return Final Answer
			func(messages []llm.ChatMessage, tools []llm.Tool) (*llm.ChatResponse, error) {
				lastMsg := messages[len(messages)-1]
				if lastMsg.Role != "tool" || lastMsg.Name != knowledge.ToolReadFile {
					t.Errorf("Round 4: Expected tool output for ReadFile, got %v", lastMsg)
				}
				if !strings.Contains(lastMsg.Content, "Check ping") {
					t.Errorf("Round 4: Expected content to contain 'Check ping', got %s", lastMsg.Content)
				}

				return &llm.ChatResponse{
					Content:   "You should check ping.",
					ToolCalls: nil, // Done
				}, nil
			},
		},
	}

	// 3. Initialize Service
	cfgMgr := config.NewManager() // Defaults
	svc := NewAIService(mockProvider, mockProvider, cfgMgr)

	// 4. Run Agent
	answer, err := svc.AskWithContext(context.Background(), "network slow", tmpDir)
	if err != nil {
		t.Fatalf("Agent run failed: %v", err)
	}

	// 5. Verify Result
	if answer != "You should check ping." {
		t.Errorf("Expected final answer 'You should check ping.', got '%s'", answer)
	}

	if mockProvider.CallCount != 4 {
		t.Errorf("Expected 4 LLM calls, got %d", mockProvider.CallCount)
	}
}

func TestAgentSearchChineseInputWithEnglishTerms(t *testing.T) {
	SetEventEmitter(func(ctx context.Context, optionalData string, optionalData2 ...interface{}) {})
	defer SetEventEmitter(nil)

	tmpDir, err := os.MkdirTemp("", "agent_test_docs_zh")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	err = os.WriteFile(filepath.Join(tmpDir, "payment.md"), []byte("# 支付系统排查\n支付超时常见原因：网关异常、DNS、路由。"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	mockProvider := &ScriptedProvider{
		T: t,
		Responses: []func(messages []llm.ChatMessage, tools []llm.Tool) (*llm.ChatResponse, error){
			func(messages []llm.ChatMessage, tools []llm.Tool) (*llm.ChatResponse, error) {
				lastMsg := messages[len(messages)-1]
				if lastMsg.Role != "user" {
					t.Errorf("Round 1: Expected user message, got %s", lastMsg.Role)
				}
				return &llm.ChatResponse{
					Content: "Thinking...",
					ToolCalls: []llm.ToolCall{
						{
							ID:   "call_1",
							Type: "function",
							Function: llm.FunctionCall{
								Name:      knowledge.ToolSearch,
								Arguments: `{"query":"payment timeout troubleshooting","top_k":5}`,
							},
						},
					},
				}, nil
			},
			func(messages []llm.ChatMessage, tools []llm.Tool) (*llm.ChatResponse, error) {
				return &llm.ChatResponse{
					Content: `[{"term":"payment","weight":5},{"term":"timeout","weight":5}]`,
				}, nil
			},
			func(messages []llm.ChatMessage, tools []llm.Tool) (*llm.ChatResponse, error) {
				lastMsg := messages[len(messages)-1]
				if lastMsg.Role != "tool" || lastMsg.Name != knowledge.ToolSearch {
					t.Errorf("Round 3: Expected tool output for SearchKnowledge, got %v", lastMsg)
				}
				if !strings.Contains(lastMsg.Content, "payment.md") {
					t.Errorf("Round 3: Expected search hits to contain payment.md, got %s", lastMsg.Content)
				}

				return &llm.ChatResponse{
					Content: "Reading file...",
					ToolCalls: []llm.ToolCall{
						{
							ID:   "call_2",
							Type: "function",
							Function: llm.FunctionCall{
								Name:      knowledge.ToolReadFile,
								Arguments: `{"path": "payment.md"}`,
							},
						},
					},
				}, nil
			},
			func(messages []llm.ChatMessage, tools []llm.Tool) (*llm.ChatResponse, error) {
				lastMsg := messages[len(messages)-1]
				if lastMsg.Role != "tool" || lastMsg.Name != knowledge.ToolReadFile {
					t.Errorf("Round 4: Expected tool output for ReadFile, got %v", lastMsg)
				}
				if !strings.Contains(lastMsg.Content, "支付超时") {
					t.Errorf("Round 4: Expected content to contain '支付超时', got %s", lastMsg.Content)
				}

				return &llm.ChatResponse{
					Content:   "请检查网关与路由。",
					ToolCalls: nil,
				}, nil
			},
		},
	}

	cfgMgr := config.NewManager()
	svc := NewAIService(mockProvider, mockProvider, cfgMgr)

	answer, err := svc.AskWithContext(context.Background(), "支付超时怎么排查", tmpDir)
	if err != nil {
		t.Fatalf("Agent run failed: %v", err)
	}
	if answer != "请检查网关与路由。" {
		t.Errorf("Expected final answer, got '%s'", answer)
	}
	if mockProvider.CallCount != 4 {
		t.Errorf("Expected 4 LLM calls, got %d", mockProvider.CallCount)
	}
}
