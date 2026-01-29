package llm

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// --- Domain Models ---

type Tool struct {
	Type     string             `json:"type"` // "function"
	Function FunctionDefinition `json:"function"`
}

type FunctionDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ChatMessage struct {
	Role       string
	Content    string
	Name       string     // Optional: Author name (e.g., for tool outputs)
	ToolCalls  []ToolCall // Optional: For assistant messages invoking tools
	ToolCallID string     // Optional: For tool messages linking back to a call
}

type ChatResponse struct {
	Content   string
	ToolCalls []ToolCall
}

// --- Interfaces ---

type Provider interface {
	// ChatCompletion Simple chat without tools
	ChatCompletion(ctx context.Context, messages []ChatMessage) (string, error)
	// ChatWithTools Chat with tool definitions
	ChatWithTools(ctx context.Context, messages []ChatMessage, tools []Tool) (*ChatResponse, error)
}

// --- Mock Implementation ---

type MockProvider struct {
	Response     string
	ToolCalls    []ToolCall
	Err          error
	LastMessages []ChatMessage
}

func (m *MockProvider) ChatCompletion(ctx context.Context, messages []ChatMessage) (string, error) {
	m.LastMessages = messages
	return m.Response, m.Err
}

func (m *MockProvider) ChatWithTools(ctx context.Context, messages []ChatMessage, tools []Tool) (*ChatResponse, error) {
	m.LastMessages = messages
	return &ChatResponse{
		Content:   m.Response,
		ToolCalls: m.ToolCalls,
	}, m.Err
}

// --- OpenAI Implementation ---

type OpenAIProvider struct {
	client *openai.Client
	model  string
}

func NewOpenAIProvider(apiKey, baseURL, model string) *OpenAIProvider {
	config := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		config.BaseURL = baseURL
	}
	return &OpenAIProvider{
		client: openai.NewClientWithConfig(config),
		model:  model,
	}
}

func (p *OpenAIProvider) ChatCompletion(ctx context.Context, messages []ChatMessage) (string, error) {
	// Re-use ChatWithTools with empty tools
	resp, err := p.ChatWithTools(ctx, messages, nil)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

func (p *OpenAIProvider) ChatWithTools(ctx context.Context, messages []ChatMessage, tools []Tool) (*ChatResponse, error) {
	if p.client == nil {
		return nil, errors.New("client not initialized")
	}
	startAt := time.Now()

	// 1. Convert Messages
	reqMessages := make([]openai.ChatCompletionMessage, len(messages))
	for i, m := range messages {
		msg := openai.ChatCompletionMessage{
			Role:       m.Role,
			Content:    m.Content,
			Name:       m.Name,
			ToolCallID: m.ToolCallID,
		}
		if len(m.ToolCalls) > 0 {
			msg.ToolCalls = make([]openai.ToolCall, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				msg.ToolCalls[j] = openai.ToolCall{
					ID:   tc.ID,
					Type: openai.ToolType(tc.Type),
					Function: openai.FunctionCall{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}
		}
		reqMessages[i] = msg
	}

	// 2. Convert Tools
	var reqTools []openai.Tool
	if len(tools) > 0 {
		reqTools = make([]openai.Tool, len(tools))
		for i, t := range tools {
			reqTools[i] = openai.Tool{
				Type: openai.ToolType(t.Type),
				Function: &openai.FunctionDefinition{
					Name:        t.Function.Name,
					Description: t.Function.Description,
					Parameters:  t.Function.Parameters,
				},
			}
		}
	}

	// Logging
	log.Printf("\n========== [LLM Request] ==========\nModel: %s\nNumMessages: %d\nNumTools: %d\n===================================", p.model, len(reqMessages), len(reqTools))

	// 3. Make Request
	req := openai.ChatCompletionRequest{
		Model:    p.model,
		Messages: reqMessages,
		Tools:    reqTools,
	}

	resp, err := p.client.CreateChatCompletion(ctx, req)
	if err != nil {
		log.Printf("[OpenAIProvider] Error: %v cost=%s", err, time.Since(startAt))
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, errors.New("no choices in response")
	}

	choice := resp.Choices[0]
	msg := choice.Message

	// 4. Convert Response
	result := &ChatResponse{
		Content: msg.Content,
	}

	if len(msg.ToolCalls) > 0 {
		result.ToolCalls = make([]ToolCall, len(msg.ToolCalls))
		for i, tc := range msg.ToolCalls {
			result.ToolCalls[i] = ToolCall{
				ID:   tc.ID,
				Type: string(tc.Type),
				Function: FunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			}
		}
	}

	// Log concise response info
	log.Printf("\n========== [LLM Response] ==========\nCost: %s\nContentLen: %d\nToolCalls: %d\n====================================", time.Since(startAt), len(result.Content), len(result.ToolCalls))
	if len(result.ToolCalls) > 0 {
		for i, tc := range result.ToolCalls {
			log.Printf("[LLM ToolCall#%d] name=%s argsLen=%d id=%s", i+1, tc.Function.Name, len(tc.Function.Arguments), tc.ID)
		}
	}

	return result, nil
}
