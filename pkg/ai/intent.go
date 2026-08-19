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

	var resp string
	var err error
	if p, ok := s.fastProvider.(*llm.OpenAIProvider); ok {
		resp, err = p.ChatCompletionNoThinking(context.Background(), messages)
	} else {
		resp, err = s.fastProvider.ChatCompletion(context.Background(), messages)
	}
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

	var resp string
	var err error
	if p, ok := s.fastProvider.(*llm.OpenAIProvider); ok {
		resp, err = p.ChatCompletionNoThinking(context.Background(), messages)
	} else {
		resp, err = s.fastProvider.ChatCompletion(context.Background(), messages)
	}
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
			// 只在 JSON 是单字段包装时拆包（如 {"summary": "..."}）
			// 如果同时包含 steps/commands，说明是完整的 troubleshoot JSON，不要拆
			_, hasSteps := wrapper["steps"]
			_, hasCommands := wrapper["commands"]
			if !hasSteps && !hasCommands {
				for _, key := range []string{"summary", "content", "answer", "text"} {
					if val, ok := wrapper[key].(string); ok && val != "" {
						s = val
						break
					}
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

// TroubleshootCommand represents a single command in the troubleshooting response.
type TroubleshootCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
	Risk        string `json:"risk,omitempty"`
	Source      string `json:"source,omitempty"`
}

// validateTroubleshootResponse checks that commands are grounded in retrieved content.
// If commands cannot be traced to source documents, they are stripped.
func validateTroubleshootResponse(normalized string, retrieved *RetrievedContent) string {
	if retrieved == nil || !retrieved.HasContent() {
		// No content was retrieved. Strip all commands and steps.
		var resp map[string]interface{}
		if err := json.Unmarshal([]byte(normalized), &resp); err != nil {
			return `{"summary":"知识库中未找到相关文档。","steps":[],"commands":[]}`
		}
		delete(resp, "commands")
		delete(resp, "steps")
		resp["commands"] = []interface{}{}
		resp["steps"] = []interface{}{}
		if summary, ok := resp["summary"].(string); !ok || summary == "" {
			resp["summary"] = "知识库中未找到相关文档。"
		}
		cleaned, _ := json.Marshal(resp)
		return string(cleaned)
	}

	// Content was retrieved. Validate commands have sources or match retrieved content.
	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(normalized), &resp); err != nil {
		return normalized
	}

	commandsRaw, ok := resp["commands"].([]interface{})
	if !ok {
		return normalized
	}

	var validated []interface{}
	for _, cmdRaw := range commandsRaw {
		cmd, ok := cmdRaw.(map[string]interface{})
		if !ok {
			continue
		}
		if isCommandGrounded(cmd, retrieved) {
			validated = append(validated, cmd)
		} else {
			cmdStr, _ := cmd["command"].(string)
			slog.Warn("stripping ungrounded command", "command", cmdStr)
		}
	}
	resp["commands"] = validated
	cleaned, _ := json.Marshal(resp)
	return string(cleaned)
}

// isCommandGrounded checks if a command has a valid source or its text appears in retrieved content.
func isCommandGrounded(cmd map[string]interface{}, rc *RetrievedContent) bool {
	// Check source field
	if source, ok := cmd["source"].(string); ok && source != "" {
		filePart := source
		if idx := strings.Index(source, "#L"); idx >= 0 {
			filePart = source[:idx]
		}
		if rc.FilesRead[filePart] {
			return true
		}
	}

	// Check if command text appears in retrieved lines
	cmdStr, _ := cmd["command"].(string)
	if cmdStr == "" {
		return false
	}
	return commandInRetrieved(cmdStr, rc)
}

// commandInRetrieved checks if a command appears in any retrieved line.
// Uses the first two tokens (e.g., "systemctl status") to reduce false positives
// from single-word matches like "cat" or "rm".
func commandInRetrieved(cmd string, rc *RetrievedContent) bool {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return false
	}
	key := parts[0]
	if len(parts) >= 2 {
		key = parts[0] + " " + parts[1]
	}
	for _, content := range rc.Lines {
		if strings.Contains(content, key) {
			return true
		}
	}
	return false
}

func (s *AIService) AskWithContext(ctx context.Context, question string, knowledgeDir string) (string, error) {
	prompt := config.DefaultQAPrompt

	resp, _, err := s.RunAgent(ctx, AgentRunOptions{
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

	resp, retrieved, err := s.RunAgent(ctx, AgentRunOptions{
		Question:     problem,
		KnowledgeDir: knowledgeDir,
		SystemPrompt: prompt,
		RetryMax:     5,
		Catalog:      s.catalog,
	})
	if err != nil {
		return "", fmt.Errorf("故障排查失败: %w", err)
	}

	normalized := normalizeAgentResponse(resp)
	validated := validateTroubleshootResponse(normalized, retrieved)
	return validated, nil
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

	var resp string
	var err error
	if p, ok := s.fastProvider.(*llm.OpenAIProvider); ok {
		resp, err = p.ChatCompletionNoThinking(context.Background(), messages)
	} else {
		resp, err = s.fastProvider.ChatCompletion(context.Background(), messages)
	}
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

// SummarizeUpdateNotesStream 用快速模型流式总结累积更新说明（禁用思考）。
// notes 为 updater.buildCumulativeChangelog 合并的多版本 Markdown。
func (s *AIService) SummarizeUpdateNotesStream(ctx context.Context, notes string, onToken func(string)) (string, error) {
	prompt := `你是 OpsCopilot（AI 运维助手桌面应用）的发布助手。用户即将升级版本，下面是自用户当前版本以来所有版本的更新说明（多个版本合并，按 "## v版本号" 分节，可能有嵌套小节）。请整合成一份简明的中文升级摘要，帮助用户快速判断"这次升级能带来什么"：

- 按「新功能 / 问题修复 / 体验优化」分组，合并各版本同类项、去除重复
- 每条一行并标注来源版本，例如：- 会话连接信息团队共享（v1.8.9.5）
- 使用面向最终用户的表述，去除内部术语
- 没有内容的分组不要输出；总计不超过 15 行
- 直接输出 Markdown 无序列表，不要标题、引言或结尾说明`

	messages := []llm.ChatMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: notes},
	}

	slog.Info("ai summarizing update notes stream")

	if p, ok := s.fastProvider.(*llm.OpenAIProvider); ok {
		full, err := p.ChatCompletionStreamNoThinking(ctx, messages, onToken)
		if err != nil {
			return "", fmt.Errorf("AI provider stream error: %w", err)
		}
		return full, nil
	}

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

	var resp string
	var err error
	if p, ok := s.fastProvider.(*llm.OpenAIProvider); ok {
		resp, err = p.ChatCompletionNoThinking(context.Background(), messages)
	} else {
		resp, err = s.fastProvider.ChatCompletion(context.Background(), messages)
	}
	if err != nil {
		return "", fmt.Errorf("AI provider error: %w", err)
	}

	return strings.TrimSpace(resp), nil
}
