package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"opscopilot/pkg/config"
	"opscopilot/pkg/llm"
	"opscopilot/pkg/sshclient"
	"regexp"
	"strings"
)

type AIService struct {
	provider llm.Provider
	cfgMgr   *config.Manager
}

func NewAIService(provider llm.Provider, cfgMgr *config.Manager) *AIService {
	return &AIService{
		provider: provider,
		cfgMgr:   cfgMgr,
	}
}

func (s *AIService) UpdateProvider(provider llm.Provider) {
	s.provider = provider
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

	resp, err := s.provider.ChatCompletion(context.Background(), messages)
	if err != nil {
        log.Printf("[AIService] Provider error: %v", err)
		return nil, fmt.Errorf("AI provider error: %w", err)
	}

    log.Printf("[AIService] Raw response from LLM: %s", resp)

	// 尝试解析 JSON
	var configs []sshclient.ConnectConfig
	
    // 清理 Markdown 代码块标记
    cleanedResp := cleanJSONResponse(resp)
    
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

// cleanJSONResponse 移除可能存在的 Markdown 代码块标记
func cleanJSONResponse(resp string) string {
    // 移除 ```json 和 ``` 标记
    re := regexp.MustCompile("```(?:json)?")
    resp = re.ReplaceAllString(resp, "")
    return strings.TrimSpace(resp)
}
