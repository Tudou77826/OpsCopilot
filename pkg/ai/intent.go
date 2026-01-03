package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"opscopilot/pkg/llm"
	"opscopilot/pkg/sshclient"
	"regexp"
	"strings"
)

type AIService struct {
	provider llm.Provider
}

func NewAIService(provider llm.Provider) *AIService {
	return &AIService{provider: provider}
}

// Prompt 模板
const systemPrompt = `
You are a smart DevOps assistant. Your task is to parse natural language intent into structured SSH connection configurations.

Output Format:
Return ONLY a JSON array of objects. No markdown, no explanations.
Each object should match this structure:
{
  "host": "string (IP or hostname)",
  "port": int (default 22),
  "user": "string",
  "password": "string (optional)",
  "root_password": "string (optional, for auto-sudo or su -)",
  "name": "string (optional display name)",
  "bastion": {
    "host": "string",
    "port": int,
    "user": "string",
    "password": "string"
  } (optional)
}

Rules:
1. Extract all target hosts mentioned. If a range or list is provided (e.g., "192.168.1.1-3" or "1.1, 1.2"), expand them into separate objects.
2. If user/password is mentioned once, apply it to all applicable hosts unless specified otherwise.
3. If a bastion/jump server is mentioned, structure it in the "bastion" field for each target.
4. If no port is specified, default to 22.
5. If information is missing (like password), leave it empty or null.
6. If the user mentions "switch to root" or "sudo" and provides a password, put it in "root_password". If the password is the same as the login password, copy it.
7. For bastion configuration: if user/password is not explicitly specified for the bastion but is provided for the main connection, assume the bastion uses the SAME credentials (user/password) as the target host, unless clearly stated otherwise.
`

func (s *AIService) ParseConnectIntent(input string) ([]sshclient.ConnectConfig, error) {
	messages := []llm.ChatMessage{
		{Role: "system", Content: systemPrompt},
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
