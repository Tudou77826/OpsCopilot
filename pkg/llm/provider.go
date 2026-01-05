package llm

import (
	"context"
	"encoding/json"
	"errors"
	"log"

	openai "github.com/sashabaranov/go-openai"
)

type ChatMessage struct {
	Role    string
	Content string
}

type Provider interface {
	ChatCompletion(ctx context.Context, messages []ChatMessage) (string, error)
}

type MockProvider struct {
	Response string
	Err      error
}

func (m *MockProvider) ChatCompletion(ctx context.Context, messages []ChatMessage) (string, error) {
	return m.Response, m.Err
}

// OpenAIProvider 实现 Provider 接口
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
	if p.client == nil {
		return "", errors.New("client not initialized")
	}

	reqMessages := make([]openai.ChatCompletionMessage, len(messages))
	for i, m := range messages {
		reqMessages[i] = openai.ChatCompletionMessage{
			Role:    m.Role,
			Content: m.Content,
		}
	}

	// Logging Request
	reqBytes, _ := json.MarshalIndent(reqMessages, "", "  ")
	log.Printf("\n========== [LLM Request] ==========\nModel: %s\nMessages:\n%s\n===================================", p.model, string(reqBytes))

	resp, err := p.client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model:    p.model,
			Messages: reqMessages,
		},
	)

	if err != nil {
		log.Printf("[OpenAIProvider] CreateChatCompletion error: %v", err)
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", errors.New("no choices in response")
	}

	content := resp.Choices[0].Message.Content

	// Logging Response
	log.Printf("\n========== [LLM Response] ==========\nContent:\n%s\n====================================", content)

	return content, nil
}
