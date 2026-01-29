package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"opscopilot/pkg/config"
	"opscopilot/pkg/knowledge"
	"opscopilot/pkg/llm"
	"opscopilot/pkg/sshclient"
	"regexp"
	"strings"
)

type AIService struct {
	fastProvider    llm.Provider
	complexProvider llm.Provider
	cfgMgr          *config.Manager
}

type CommandQueryResult struct {
	Command     string `json:"command"`
	Explanation string `json:"explanation"`
}

func NewAIService(fastProvider llm.Provider, complexProvider llm.Provider, cfgMgr *config.Manager) *AIService {
	return &AIService{
		fastProvider:    fastProvider,
		complexProvider: complexProvider,
		cfgMgr:          cfgMgr,
	}
}

func (s *AIService) UpdateProviders(fastProvider llm.Provider, complexProvider llm.Provider) {
	s.fastProvider = fastProvider
	s.complexProvider = complexProvider
}

func (s *AIService) GenerateLinuxCommand(request string) (*CommandQueryResult, error) {
	prompt := s.cfgMgr.Config.Prompts["command_query_prompt"]
	if prompt == "" {
		prompt = config.DefaultCommandQueryPrompt
	}

	messages := []llm.ChatMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: request},
	}

	resp, err := s.fastProvider.ChatCompletion(context.Background(), messages)
	if err != nil {
		return nil, fmt.Errorf("AI provider error: %w", err)
	}

	cleaned := CleanJSONResponse(resp)
	var result CommandQueryResult
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil, fmt.Errorf("failed to parse AI response as JSON: %v. Raw: %s", err, resp)
	}

	result.Command = strings.TrimSpace(result.Command)
	result.Explanation = strings.TrimSpace(result.Explanation)
	if result.Command == "" {
		return nil, fmt.Errorf("AI response missing command. Raw: %s", resp)
	}

	return &result, nil
}

func (s *AIService) ParseConnectIntent(input string) ([]sshclient.ConnectConfig, error) {
	prompt := s.cfgMgr.Config.Prompts["smart_connect"]
	if prompt == "" {
		prompt = config.DefaultSmartConnectPrompt
	}

	messages := []llm.ChatMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: input},
	}

	log.Printf("[AIService] Sending request to LLM: %s", input)

	resp, err := s.fastProvider.ChatCompletion(context.Background(), messages)
	if err != nil {
		log.Printf("[AIService] Provider error: %v", err)
		return nil, fmt.Errorf("AI provider error: %w", err)
	}

	log.Printf("[AIService] Raw response from LLM: %s", resp)

	// 尝试解析 JSON
	var configs []sshclient.ConnectConfig

	// 清理 Markdown 代码块标记
	cleanedResp := CleanJSONResponse(resp)

	if err := json.Unmarshal([]byte(cleanedResp), &configs); err != nil {
		log.Printf("[AIService] JSON parse error: %v. Cleaned response: %s", err, cleanedResp)
		return nil, fmt.Errorf("failed to parse AI response as JSON: %v. Raw: %s", err, resp)
	}

	// 校验配置完整性
	for i, c := range configs {
		if c.Host == "" {
			return nil, fmt.Errorf("config #%d missing 'host'. AI response incomplete", i+1)
		}
		if c.User == "" {
			return nil, fmt.Errorf("config #%d missing 'user' for host %s. Please provide a username", i+1, c.Host)
		}
	}

	return configs, nil
}

// CleanJSONResponse 移除可能存在的 Markdown 代码块标记
func CleanJSONResponse(resp string) string {
	// 1. 移除 Markdown 代码块标记 ```json 或 ```
	// (?s) 开启 dot-matches-newline 模式，确保能匹配多行
	re := regexp.MustCompile("(?s)```(?:json)?(.*?)```")
	matches := re.FindStringSubmatch(resp)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}

	// 如果没有匹配到代码块，尝试移除单独的 ``` 标记（兼容性处理）
	resp = strings.ReplaceAll(resp, "```json", "")
	resp = strings.ReplaceAll(resp, "```", "")
	return strings.TrimSpace(resp)
}

func (s *AIService) AskWithContext(ctx context.Context, question string, knowledgeDir string) (string, error) {
	prompt := s.cfgMgr.Config.Prompts["qa_prompt"]
	if prompt == "" {
		prompt = config.DefaultQAPrompt
	}

	resp, err := s.RunAgent(ctx, AgentRunOptions{
		Question:     question,
		KnowledgeDir: knowledgeDir,
		SystemPrompt: prompt,
	})
	if err != nil {
		log.Printf("[AIService] Agent mode failed: %v. Falling back to legacy RAG.", err)

		// Fallback: Load all knowledge and ask directly
		contextContent, loadErr := knowledge.LoadAll(knowledgeDir)
		if loadErr != nil {
			log.Printf("[AIService] Fallback load failed: %v", loadErr)
			contextContent = "" // Continue with empty context
		}

		fullContent := fmt.Sprintf("Context:\n%s\n\nQuestion: %s", contextContent, question)
		messages := []llm.ChatMessage{
			{Role: "system", Content: prompt},
			{Role: "user", Content: fullContent},
		}

		return s.complexProvider.ChatCompletion(ctx, messages)
	}

	return resp, nil
}

func (s *AIService) AskTroubleshoot(ctx context.Context, problem string, knowledgeDir string) (string, error) {
	prompt := s.cfgMgr.Config.Prompts["troubleshoot_prompt"]
	if prompt == "" {
		prompt = config.DefaultTroubleshootPrompt
	}

	resp, err := s.RunAgent(ctx, AgentRunOptions{
		Question:     problem,
		KnowledgeDir: knowledgeDir,
		SystemPrompt: prompt,
	})
	if err != nil {
		return "", err
	}

	// Clean JSON response (remove markdown code blocks)
	return CleanJSONResponse(resp), nil
}

func (s *AIService) GenerateConclusion(timeline string, rootCause string) (string, error) {
	prompt := s.cfgMgr.Config.Prompts["conclusion_prompt"]
	if prompt == "" {
		prompt = config.DefaultConclusionPrompt
	}

	content := fmt.Sprintf("Timeline:\n%s\n\nRoot Cause:\n%s", timeline, rootCause)

	messages := []llm.ChatMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: content},
	}

	log.Printf("[AIService] Generating conclusion")

	resp, err := s.fastProvider.ChatCompletion(context.Background(), messages)
	if err != nil {
		return "", fmt.Errorf("AI provider error: %w", err)
	}

	return resp, nil
}

func (s *AIService) PolishContent(content string) (string, error) {
	prompt := s.cfgMgr.Config.Prompts["polish_prompt"]
	if prompt == "" {
		prompt = config.DefaultPolishPrompt
	}

	messages := []llm.ChatMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: content},
	}

	log.Printf("[AIService] Polishing content")

	resp, err := s.fastProvider.ChatCompletion(context.Background(), messages)
	if err != nil {
		return "", fmt.Errorf("AI provider error: %w", err)
	}

	return strings.TrimSpace(resp), nil
}
