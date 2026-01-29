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

	// 2. Define Test Script
	// Scenario: User asks "network slow".
	// Step 1: Agent calls ListFiles
	// Step 2: Agent calls ReadFile("network.md")
	// Step 3: Agent answers

	mockProvider := &ScriptedProvider{
		T: t,
		Responses: []func(messages []llm.ChatMessage, tools []llm.Tool) (*llm.ChatResponse, error){
			// Round 1: Expect User Question, Return ToolCall ListFiles
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
								Name:      knowledge.ToolListFiles,
								Arguments: "{}",
							},
						},
					},
				}, nil
			},
			// Round 2: Expect Tool Output (List), Return ToolCall ReadFile
			func(messages []llm.ChatMessage, tools []llm.Tool) (*llm.ChatResponse, error) {
				// Verify last message is Tool Output
				lastMsg := messages[len(messages)-1]
				if lastMsg.Role != "tool" || lastMsg.Name != knowledge.ToolListFiles {
					t.Errorf("Round 2: Expected tool output for ListFiles, got %v", lastMsg)
				}
				if !strings.Contains(lastMsg.Content, "network.md") {
					t.Errorf("Round 2: Expected file list to contain network.md, got %s", lastMsg.Content)
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
			// Round 3: Expect Tool Output (Content), Return Final Answer
			func(messages []llm.ChatMessage, tools []llm.Tool) (*llm.ChatResponse, error) {
				lastMsg := messages[len(messages)-1]
				if lastMsg.Role != "tool" || lastMsg.Name != knowledge.ToolReadFile {
					t.Errorf("Round 3: Expected tool output for ReadFile, got %v", lastMsg)
				}
				if !strings.Contains(lastMsg.Content, "Check ping") {
					t.Errorf("Round 3: Expected content to contain 'Check ping', got %s", lastMsg.Content)
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

	if mockProvider.CallCount != 3 {
		t.Errorf("Expected 3 LLM calls, got %d", mockProvider.CallCount)
	}
}
