package llm

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
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

// StreamTokenCallback 流式 token 回调函数类型
type StreamTokenCallback func(token string)

type Provider interface {
	// ChatCompletion Simple chat without tools
	ChatCompletion(ctx context.Context, messages []ChatMessage) (string, error)
	// ChatWithTools Chat with tool definitions
	ChatWithTools(ctx context.Context, messages []ChatMessage, tools []Tool) (*ChatResponse, error)
	// ChatCompletionStream 流式 chat，每个 token 通过 onToken 回调返回，最终返回完整内容
	ChatCompletionStream(ctx context.Context, messages []ChatMessage, onToken StreamTokenCallback) (string, error)
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

func (m *MockProvider) ChatCompletionStream(ctx context.Context, messages []ChatMessage, onToken StreamTokenCallback) (string, error) {
	m.LastMessages = messages
	if m.Err != nil {
		return "", m.Err
	}
	if onToken != nil {
		onToken(m.Response)
	}
	return m.Response, nil
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

// ChatCompletionStream 流式输出 chat completion，每个 token 通过 onToken 回调
func (p *OpenAIProvider) ChatCompletionStream(ctx context.Context, messages []ChatMessage, onToken StreamTokenCallback) (string, error) {
	return p.streamChatCompletion(ctx, messages, onToken, false)
}

// ChatCompletionStreamNoThinking 流式输出并禁用思考模式，仅用于结论生成等不需要推理的场景。
// 此方法不在 Provider 接口中，通过类型断言调用。
func (p *OpenAIProvider) ChatCompletionStreamNoThinking(ctx context.Context, messages []ChatMessage, onToken StreamTokenCallback) (string, error) {
	return p.streamChatCompletion(ctx, messages, onToken, true)
}

// streamChatCompletion 内部共用实现
func (p *OpenAIProvider) streamChatCompletion(ctx context.Context, messages []ChatMessage, onToken StreamTokenCallback, disableThinking bool) (string, error) {
	if p.client == nil {
		return "", errors.New("client not initialized")
	}
	startAt := time.Now()

	reqMessages := make([]openai.ChatCompletionMessage, len(messages))
	for i, m := range messages {
		reqMessages[i] = openai.ChatCompletionMessage{
			Role:       m.Role,
			Content:    m.Content,
			Name:       m.Name,
			ToolCallID: m.ToolCallID,
		}
	}

	req := openai.ChatCompletionRequest{
		Model:    p.model,
		Messages: reqMessages,
		Stream:   true,
	}

	label := "stream"
	if disableThinking {
		req.ChatTemplateKwargs = map[string]any{
			"enable_thinking": false,
		}
		label = "stream (no-thinking)"
	}

	log.Printf("[OpenAIProvider] Starting %s: model=%s numMessages=%d", label, p.model, len(reqMessages))

	stream, err := p.client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		log.Printf("[OpenAIProvider] Stream error: %v cost=%s", err, time.Since(startAt))
		return "", err
	}
	defer stream.Close()

	var fullContent strings.Builder
	for {
		resp, err := stream.Recv()
		if err != nil {
			if err.Error() == "EOF" || err.Error() == "stream finished" {
				break
			}
			if strings.Contains(err.Error(), "stream") && strings.Contains(err.Error(), "finish") {
				break
			}
			log.Printf("[OpenAIProvider] Stream recv error: %v", err)
			break
		}

		if len(resp.Choices) > 0 {
			delta := resp.Choices[0].Delta
			if delta.Content != "" {
				fullContent.WriteString(delta.Content)
				if onToken != nil {
					onToken(delta.Content)
				}
			}
		}
	}

	result := fullContent.String()
	log.Printf("[OpenAIProvider] %s done: cost=%s contentLen=%d", label, time.Since(startAt), len(result))
	return result, nil
}
