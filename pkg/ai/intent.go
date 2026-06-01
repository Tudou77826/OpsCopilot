package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"opscopilot/pkg/config"
	"opscopilot/pkg/knowledge"
	"opscopilot/pkg/llm"
	"opscopilot/pkg/logging"
	"opscopilot/pkg/sshclient"
	"regexp"
	"strings"
)

type AIService struct {
	fastProvider    llm.Provider
	complexProvider llm.Provider
	cfgMgr          *config.Manager
	catalog         *knowledge.Catalog // 知识库目录
	knowledgeDir    string             // 知识库目录路径
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

// UpdateCatalog 构建并更新知识库目录
func (s *AIService) UpdateCatalog(knowledgeDir string) error {
	catalog, err := knowledge.BuildCatalog(knowledgeDir)
	if err != nil {
		return err
	}
	s.catalog = catalog
	s.knowledgeDir = knowledgeDir
	return nil
}

// GetCatalog 获取当前目录（供外部检查）
func (s *AIService) GetCatalog() *knowledge.Catalog {
	return s.catalog
}

// GetFastProvider 返回 fast provider（供外部模块使用）
func (s *AIService) GetFastProvider() llm.Provider {
	return s.fastProvider
}

func (s *AIService) UpdateProviders(fastProvider llm.Provider, complexProvider llm.Provider) {
	s.fastProvider = fastProvider
	s.complexProvider = complexProvider
}

func (s *AIService) GenerateLinuxCommand(request string) (*CommandQueryResult, error) {
	prompt := config.DefaultCommandQueryPrompt

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
	prompt := config.DefaultSmartConnectPrompt

	messages := []llm.ChatMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: input},
	}

	slog.Debug("ai sending request to LLM", "input", logging.Truncate(input, 100))

	resp, err := s.fastProvider.ChatCompletion(context.Background(), messages)
	if err != nil {
		slog.Error("ai provider error", "error", err)
		return nil, fmt.Errorf("AI provider error: %w", err)
	}

	slog.Debug("ai raw response", "response", logging.Truncate(resp, 200))

	// 尝试解析 JSON
	var configs []sshclient.ConnectConfig

	// 清理 Markdown 代码块标记
	cleanedResp := CleanJSONResponse(resp)

	if err := json.Unmarshal([]byte(cleanedResp), &configs); err != nil {
		slog.Warn("ai json parse error", "error", err, "response", logging.Truncate(cleanedResp, 200))
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

// normalizeAgentResponse 清理 Agent 返回结果中的格式问题
func normalizeAgentResponse(resp string) string {
	s := strings.TrimSpace(resp)

	// 1. 处理 JSON 包装的 Markdown
	// 检测 {"summary": "..."} 或 {"content": "..."} 模式
	if strings.HasPrefix(s, "{") {
		var wrapper map[string]interface{}
		if err := json.Unmarshal([]byte(s), &wrapper); err == nil {
			for _, key := range []string{"summary", "content", "answer", "text"} {
				if val, ok := wrapper[key].(string); ok && val != "" {
					s = val
					break
				}
			}
		}
	}

	// 2. 处理孤立的代码块标记
	// 统计 ``` 出现次数，奇数时移除最后一个
	count := strings.Count(s, "```")
	if count%2 == 1 {
		lastIdx := strings.LastIndex(s, "```")
		s = s[:lastIdx] + s[lastIdx+3:]
	}

	// 3. 剥离 markdown 代码块包裹（LLM 经常用 ```json 包裹 JSON 输出）
	codeBlockRe := regexp.MustCompile("(?s)```(?:json)?\\s*\\n?(.*?)\\n?\\s*```")
	if matches := codeBlockRe.FindStringSubmatch(s); len(matches) > 1 {
		extracted := strings.TrimSpace(matches[1])
		if strings.HasPrefix(extracted, "{") {
			s = extracted
		}
	}

	// 4. 从前后文字中提取 JSON（LLM 有时在 JSON 前后加解释性文字）
	if !strings.HasPrefix(s, "{") {
		firstBrace := strings.Index(s, "{")
		if firstBrace >= 0 {
			candidate := s[firstBrace:]
			var test map[string]interface{}
			if json.Unmarshal([]byte(candidate), &test) == nil {
				s = candidate
			} else {
				lastBrace := strings.LastIndex(candidate, "}")
				if lastBrace > 0 {
					inner := candidate[:lastBrace+1]
					if json.Unmarshal([]byte(inner), &test) == nil {
						s = inner
					}
				}
			}
		}
	}

	// 5. 非 JSON 响应包装成结构化格式（确保前端始终能按 JSON 路径渲染）
	if !isValidTroubleshootJSON(s) {
		s = wrapAsTroubleshootJSON(s)
	}

	return strings.TrimSpace(s)
}

// isValidTroubleshootJSON 检查字符串是否为包含 steps/commands/summary 之一的合法 JSON
func isValidTroubleshootJSON(s string) bool {
	trimmed := strings.TrimSpace(s)
	if !strings.HasPrefix(trimmed, "{") {
		return false
	}
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &data); err != nil {
		return false
	}
	_, hasSteps := data["steps"]
	_, hasCommands := data["commands"]
	_, hasSummary := data["summary"]
	return hasSteps || hasCommands || hasSummary
}

// wrapAsTroubleshootJSON 将非 JSON 字符串包装成前端可渲染的结构
func wrapAsTroubleshootJSON(s string) string {
	summary := strings.TrimSpace(s)
	if summary == "" {
		return `{"steps":[],"commands":[]}`
	}
	escaped, err := json.Marshal(summary)
	if err != nil {
		return `{"steps":[],"commands":[]}`
	}
	return fmt.Sprintf(`{"summary":%s,"steps":[],"commands":[]}`, string(escaped))
}

func (s *AIService) AskWithContext(ctx context.Context, question string, knowledgeDir string) (string, error) {
	prompt := config.DefaultQAPrompt

	resp, err := s.RunAgent(ctx, AgentRunOptions{
		Question:     question,
		KnowledgeDir: knowledgeDir,
		SystemPrompt: prompt,
		RetryMax:     5,
	})
	if err != nil {
		return "", fmt.Errorf("AI 问答失败: %w", err)
	}

	return resp, nil
}

func (s *AIService) AskTroubleshoot(ctx context.Context, problem string, knowledgeDir string) (string, error) {
	prompt := config.DefaultTroubleshootPrompt

	resp, err := s.RunAgent(ctx, AgentRunOptions{
		Question:     problem,
		KnowledgeDir: knowledgeDir,
		SystemPrompt: prompt,
		RetryMax:     5,
		Catalog:      s.catalog,
	})
	if err != nil {
		return "", fmt.Errorf("故障排查失败: %w", err)
	}

	return normalizeAgentResponse(resp), nil
}

func (s *AIService) GenerateConclusion(timeline string, rootCause string) (string, error) {
	// conclusion_prompt 的 section 格式是 archiver 的硬依赖，不读取用户配置
	prompt := config.DefaultConclusionPrompt

	content := fmt.Sprintf("Timeline:\n%s\n\nRoot Cause:\n%s", timeline, rootCause)

	messages := []llm.ChatMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: content},
	}

	slog.Info("ai generating conclusion")

	resp, err := s.fastProvider.ChatCompletion(context.Background(), messages)
	if err != nil {
		return "", fmt.Errorf("AI provider error: %w", err)
	}

	return resp, nil
}

// GenerateConclusionStream 流式生成结论，通过 onToken 回调逐步推送 token。
// 禁用思考模式以加速响应。
func (s *AIService) GenerateConclusionStream(ctx context.Context, timeline string, rootCause string, onToken func(string)) (string, error) {
	prompt := config.DefaultConclusionPrompt

	content := fmt.Sprintf("Timeline:\n%s\n\nRoot Cause:\n%s", timeline, rootCause)

	messages := []llm.ChatMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: content},
	}

	slog.Info("ai generating conclusion stream")

	// 优先使用 OpenAIProvider 的 no-thinking 专用方法
	if p, ok := s.fastProvider.(*llm.OpenAIProvider); ok {
		full, err := p.ChatCompletionStreamNoThinking(ctx, messages, onToken)
		if err != nil {
			return "", fmt.Errorf("AI provider stream error: %w", err)
		}
		return full, nil
	}

	// fallback：其他 Provider 实现走通用接口
	full, err := s.fastProvider.ChatCompletionStream(ctx, messages, onToken)
	if err != nil {
		return "", fmt.Errorf("AI provider stream error: %w", err)
	}
	return full, nil
}

func (s *AIService) PolishContent(content string) (string, error) {
	prompt := config.DefaultPolishPrompt

	messages := []llm.ChatMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: content},
	}

	slog.Info("ai polishing content")

	resp, err := s.fastProvider.ChatCompletion(context.Background(), messages)
	if err != nil {
		return "", fmt.Errorf("AI provider error: %w", err)
	}

	return strings.TrimSpace(resp), nil
}
