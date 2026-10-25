package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

// ChoiceClient makes one small classification request. It has no tools, agent
// loop or retries and never returns/logs provider bodies, URLs or credentials.
type ChoiceClient struct {
	endpoint string
	key      string
	model    string
	client   *http.Client
}

func NewChoiceClient(baseURL, key, model string) *ChoiceClient {
	return &ChoiceClient{endpoint: strings.TrimRight(baseURL, "/") + "/chat/completions", key: key, model: model, client: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("分类请求不跟随重定向") }}}
}

func (c *ChoiceClient) Choose(ctx context.Context, messages []ChatMessage) (string, error) {
	failure := errors.New("快速模型请求失败")
	requestMessages := make([]openai.ChatCompletionMessage, 0, len(messages))
	for _, message := range messages {
		requestMessages = append(requestMessages, openai.ChatCompletionMessage{Role: message.Role, Content: message.Content})
	}
	// go-openai's request struct cannot express the top-level thinking switch
	// used by DeepSeek/GLM. Keep this one-shot payload separate from chat calls.
	request := struct {
		openai.ChatCompletionRequest
		Thinking map[string]string `json:"thinking,omitempty"`
	}{ChatCompletionRequest: openai.ChatCompletionRequest{Model: c.model, Messages: requestMessages, MaxTokens: 16}}
	name := strings.ToLower(c.model)
	if strings.Contains(name, "deepseek") || strings.Contains(name, "glm") {
		request.Thinking = map[string]string{"type": "disabled"}
	}
	if strings.Contains(name, "qwen") {
		request.ChatTemplateKwargs = map[string]any{"enable_thinking": false}
	}
	body, err := json.Marshal(request)
	if err != nil {
		return "", failure
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", failure
	}
	req.Header.Set("Content-Type", "application/json")
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	response, err := c.client.Do(req)
	if err != nil {
		return "", failure
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", failure
	}
	const maxResponse = 256 << 10
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	if err != nil || len(data) > maxResponse {
		return "", failure
	}
	var completion openai.ChatCompletionResponse
	if json.Unmarshal(data, &completion) != nil {
		return "", failure
	}
	// A successful but incomplete/invalid model answer is an uncertain decision,
	// not an endpoint failure. The host must ask a human instead of retrying.
	if len(completion.Choices) != 1 {
		return "", nil
	}
	choice := completion.Choices[0]
	if choice.FinishReason != openai.FinishReasonStop || len(choice.Message.ToolCalls) > 0 || len(choice.Message.Content) > 64 {
		return "", nil
	}
	return choice.Message.Content, nil
}
