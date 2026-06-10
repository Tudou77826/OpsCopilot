package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"opscopilot/pkg/knowledge"
	"opscopilot/pkg/llm"
	"opscopilot/pkg/logging"
	"opscopilot/pkg/tools"
	knowledgetools "opscopilot/pkg/tools/knowledge"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	openai "github.com/sashabaranov/go-openai"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	maxContextTokens = 75000                            // 上下文 token 上限
	charsPerToken    = 3                                // 中英混合估算：~3 字符/token
	maxContextChars  = maxContextTokens * charsPerToken // ≈225K 字符
	trimTargetRatio  = 0.7                              // 截断目标：降到 70% 以下
)

// estimateMessagesChars 估算消息列表的总字符数
func estimateMessagesChars(messages []llm.ChatMessage) int {
	total := 0
	for _, msg := range messages {
		total += len(msg.Content)
		for _, tc := range msg.ToolCalls {
			total += len(tc.Function.Arguments)
		}
	}
	return total
}

// trimEarlyToolResults 截断最早的大体积工具结果，保持最近上下文完整
// 不动 messages[0]（system）和 messages[1]（user），从 messages[2] 开始扫描
// 保留最后 4 条消息不动（通常是最近的 assistant + tool results）
func trimEarlyToolResults(messages []llm.ChatMessage, maxChars int) {
	total := estimateMessagesChars(messages)
	if total <= maxChars {
		return
	}

	target := int(float64(maxChars) * trimTargetRatio)
	protected := len(messages) - 4 // 保护最近 4 条
	if protected < 3 {              // 至少保护 system + user + 第一条
		protected = 3
	}

	for i := 2; i < protected && total > target; i++ {
		if messages[i].Role == "tool" && len(messages[i].Content) > 200 {
			head := messages[i].Content[:150]
			removed := len(messages[i].Content) - 200
			messages[i].Content = head + "\n...[已截断]..."
			total -= removed
		}
	}
}

// AgentRunOptions defines options for the agent execution
type AgentRunOptions struct {
	Question     string
	KnowledgeDir string
	SystemPrompt string
	RetryMax     int
	Catalog      *knowledge.Catalog // 知识库目录（注入 Agent 系统提示）
}

// RetrievedContent tracks what documents and lines were fetched during the agent loop.
type RetrievedContent struct {
	Lines     map[string]string // "file.md:42" → raw line content
	FilesRead map[string]bool   // which files had at least one line read
}

func NewRetrievedContent() *RetrievedContent {
	return &RetrievedContent{
		Lines:     make(map[string]string),
		FilesRead: make(map[string]bool),
	}
}

// HasContent returns true if any content was retrieved.
func (rc *RetrievedContent) HasContent() bool {
	return len(rc.Lines) > 0 || len(rc.FilesRead) > 0
}

// RecordFromGrep parses grep tool output (format: "file:lineNum: content") and records it.
func (rc *RetrievedContent) RecordFromGrep(toolResult string) {
	for _, line := range strings.Split(toolResult, "\n") {
		// Skip non-result lines like "No matches found for pattern: ..."
		if strings.HasPrefix(line, "No matches found") || strings.HasPrefix(line, "... ") {
			continue
		}
		idx := strings.Index(line, ": ")
		if idx < 0 {
			continue
		}
		prefix := line[:idx] // e.g. "payment_sop.md:42"
		content := line[idx+2:]
		rc.Lines[prefix] = content
		fileOnly := strings.SplitN(prefix, ":", 2)[0]
		rc.FilesRead[fileOnly] = true
	}
}

// RecordFromRead parses read_knowledge_file output (format: "  N | content") and records it.
func (rc *RetrievedContent) RecordFromRead(toolResult string, filePath string) {
	rc.FilesRead[filePath] = true
	for _, line := range strings.Split(toolResult, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed == "...(truncated)..." {
			continue
		}
		idx := strings.Index(trimmed, " | ")
		if idx < 0 {
			continue
		}
		lineNum := strings.TrimSpace(trimmed[:idx])
		content := trimmed[idx+3:]
		key := filePath + ":" + lineNum
		rc.Lines[key] = content
	}
}

const agentBasePrompt = "你是 OpsCopilot 运维诊断助手。你可以查阅本地知识库来辅助诊断。\n\n" +
	"## 可用工具\n" +
	"1. read_knowledge_file: 读取知识库文档，返回带行号的内容。支持 start_line/end_line 按行号截断读取。\n" +
	"2. grep_knowledge: 在知识库文档中搜索，支持正则表达式。用 | 表示 OR 匹配多个词，如 'timeout|504|超时'。\n\n" +
	"## 工具使用策略\n" +
	"- 先用 grep_knowledge 定位关键词所在文件和行号\n" +
	"- 搜索时用 | 组合同义词、中英文、错误码，提高召回率（如 'OOM|out of memory|内存溢出'）\n" +
	"- 再用 read_knowledge_file 配合 start_line/end_line 定向阅读相关段落\n" +
	"- 无需 grep 时也可直接 read_knowledge_file 读全文（带行号）\n\n" +
	"## 检索策略\n" +
	"参考上方「知识库问题目录」，综合以下维度找出所有可能相关的场景：\n" +
	"- 现象匹配：用户描述的故障现象是否与目录中某场景类似\n" +
	"- 组件关联：用户涉及的组件（如 MySQL、Nginx、Redis）是否出现在目录场景的「涉及组件」中\n" +
	"- 关键词交叉：用户问题中的关键词与目录场景的关键词是否有重叠\n" +
	"一次可以读取多个相关场景（多次调用 read_knowledge_file），然后综合分析给出推断。\n" +
	"不必要求「完全相同的问题」才检索，症状相似、组件相关、根因相近的场景都值得参考。\n\n" +
	"## 规则\n" +
	"- 【严格】如果工具搜索后知识库中没有与用户问题相关的场景，必须返回空结果（空steps、空commands），禁止编造任何命令\n" +
	"- 如果知识库中有相关文档，基于文档内容给出详细、准确的排查建议，所有命令必须来自知识库文档\n" +
	"- 回答长度必须与知识库匹配度成正比：无匹配=空结果，弱匹配=简短，强匹配=详细\n" +
	"- 用中文回答\n" +
	"- 输出格式由后续追加的系统指令决定，严格遵循追加指令中的格式要求。"

// RunAgent executes the ReAct loop
func (s *AIService) RunAgent(ctx context.Context, opts AgentRunOptions) (string, *RetrievedContent, error) {
	runID := uuid.NewString()
	startAt := time.Now()

	// 创建工具注册器并注册知识库工具
	registry := tools.NewRegistry()
	registry.Register(knowledgetools.NewReadFileTool(opts.KnowledgeDir, opts.Catalog))
	registry.Register(knowledgetools.NewGrepTool(opts.KnowledgeDir))

	// 构建LLM工具列表
	llmTools := registry.ToLLMTools()

	// 合并 system messages 为一个，避免某些模型报错 "System message must be at the beginning"
	var systemPromptBuilder strings.Builder
	systemPromptBuilder.WriteString(agentBasePrompt)

	// 注入目录上下文
	if opts.Catalog != nil {
		catalogText := opts.Catalog.RenderForLLM()
		if catalogText != "" {
			systemPromptBuilder.WriteString("\n\n## 知识库问题目录\n\n")
			systemPromptBuilder.WriteString(catalogText)
			slog.Debug("agent catalog injected", "runId", runID, "scenarios", opts.Catalog.TotalScenarios(), "bytes", len(catalogText))
		} else {
			slog.Debug("agent catalog present but empty", "runId", runID)
		}
	} else {
		slog.Debug("agent no catalog available", "runId", runID)
	}

	if opts.SystemPrompt != "" {
		systemPromptBuilder.WriteString("\n\n")
		systemPromptBuilder.WriteString(opts.SystemPrompt)
	}

	systemPrompt := systemPromptBuilder.String()
	messages := []llm.ChatMessage{{Role: "system", Content: systemPrompt}}
	messages = append(messages, llm.ChatMessage{Role: "user", Content: opts.Question})

	provider := s.complexProvider
	maxSteps := 30

	knowledgeExists := false
	if opts.KnowledgeDir != "" {
		if st, err := os.Stat(opts.KnowledgeDir); err == nil && st.IsDir() {
			knowledgeExists = true
		}
	}

	slog.Info("agent start", "runId", runID, "questionLen", len(opts.Question), "knowledgeDir", opts.KnowledgeDir, "knowledgeExists", knowledgeExists, "tools", len(llmTools))

	var prevToolCalls []llm.ToolCall
	retrieved := NewRetrievedContent()

	for i := 0; i < maxSteps; i++ {
		if i == 0 {
			emitStatus(ctx, runID, "thinking", "正在分析问题，扫描知识库目录...")
		} else {
			msg := inferNextStepMessage(prevToolCalls, i, maxSteps)
			emitStatus(ctx, runID, "thinking", msg)
		}
		stepAt := time.Now()
		resp, err := retryChatWithTools(ctx, runID, opts.RetryMax, func() (*llm.ChatResponse, error) {
			return provider.ChatWithTools(ctx, messages, llmTools)
		})
		llmCost := time.Since(stepAt)
		if err != nil {
			slog.Error("agent llm error", "runId", runID, "step", i+1, "cost", logging.Cost(llmCost), "error", err)
			return "", retrieved, err
		}

		slog.Debug("agent llm response", "runId", runID, "step", i+1, "cost", logging.Cost(llmCost), "contentLen", len(resp.Content), "toolCalls", len(resp.ToolCalls))
		if resp.Content != "" {
			slog.Debug("agent llm thinking", "runId", runID, "step", i+1, "content", logging.Truncate(resp.Content, 200))
		}
		for idx, tc := range resp.ToolCalls {
			slog.Debug("agent tool call", "runId", runID, "step", i+1, "index", idx+1, "name", tc.Function.Name, "args", logging.Truncate(tc.Function.Arguments, 200))
		}

		messages = append(messages, llm.ChatMessage{
			Role:      "assistant",
			Content:          resp.Content,
			ToolCalls:        resp.ToolCalls,
			ReasoningContent: resp.ReasoningContent,
		})
		prevToolCalls = resp.ToolCalls

		if len(resp.ToolCalls) == 0 {
			if i == 0 {
				emitStatus(ctx, runID, "answering", "模型未调用工具，直接生成回答...")
			} else {
				emitStatus(ctx, runID, "answering", "正在生成回答...")
			}
			slog.Info("agent done", "runId", runID, "cost", logging.Cost(time.Since(startAt)))
			return resp.Content, retrieved, nil
		}

		for _, tc := range resp.ToolCalls {
			var toolResult string

			slog.Info("agent calling tool", "runId", runID, "step", i+1, "name", tc.Function.Name)
			slog.Debug("agent tool args", "runId", runID, "args", logging.Truncate(tc.Function.Arguments, 200))

			// 优先使用注册器中的工具（知识库工具）
			if tool, ok := registry.Get(tc.Function.Name); ok {
				var args map[string]interface{}
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					toolResult = fmt.Sprintf("Error parsing arguments: %v", err)
					slog.Error("agent tool parse args error", "runId", runID, "name", tc.Function.Name, "error", err)
				} else {
					toolAt := time.Now()
					statusEmitter := func(stage, message string) {
						emitStatus(ctx, runID, stage, message)
					}
					result, err := tool.Execute(ctx, args, statusEmitter)
					toolCost := time.Since(toolAt)
					if err != nil {
						toolResult = fmt.Sprintf("Error: %v", err)
						slog.Error("agent tool error", "runId", runID, "name", tc.Function.Name, "cost", logging.Cost(toolCost), "error", err)
					} else {
						toolResult = result
						slog.Info("agent tool done", "runId", runID, "name", tc.Function.Name, "cost", logging.Cost(toolCost))
						slog.Debug("agent tool result", "runId", runID, "resultLen", len(toolResult))
					}
				}
			} else {
				toolResult = fmt.Sprintf("Error: Unknown tool %s", tc.Function.Name)
				slog.Error("agent unknown tool", "runId", runID, "name", tc.Function.Name)
			}

			messages = append(messages, llm.ChatMessage{
				Role:       "tool",
				ToolCallID: tc.ID,
				Name:       tc.Function.Name,
				Content:    toolResult,
			})

			// Track retrieved content for post-processing validation
			if toolResult != "" && !strings.HasPrefix(toolResult, "Error") {
				switch tc.Function.Name {
				case "grep_knowledge":
					retrieved.RecordFromGrep(toolResult)
				case "read_knowledge_file":
					var readArgs map[string]interface{}
					if json.Unmarshal([]byte(tc.Function.Arguments), &readArgs) == nil {
						if path, ok := readArgs["path"].(string); ok {
							retrieved.RecordFromRead(toolResult, path)
						}
					}
				}
			}
		}

		slog.Debug("agent step outputs appended", "runId", runID, "step", i+1, "outputs", len(resp.ToolCalls), "messageCount", len(messages))

		// 检测累积上下文，超限时截断最早的工具结果
		trimEarlyToolResults(messages, maxContextChars)

		// 发送上下文用量到前端
		totalChars := estimateMessagesChars(messages)
		estimatedTokens := totalChars / charsPerToken
		emitContextUsage(ctx, runID, estimatedTokens, maxContextTokens)
		slog.Debug("agent context estimate", "runId", runID, "step", i+1, "chars", totalChars, "tokensK", estimatedTokens/1000, "messages", len(messages))
	}

	slog.Error("agent exceeded max steps", "runId", runID, "totalCost", time.Since(startAt), "maxSteps", maxSteps)
	return "", retrieved, fmt.Errorf("agent exceeded maximum steps (%d) without reaching a conclusion", maxSteps)
}

func safeEmit(ctx context.Context, eventName string, data interface{}) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("recovered from emit panic", "error", r)
		}
	}()

	if eventEmitter != nil {
		eventEmitter(ctx, eventName, data)
	}
}

var eventEmitter = runtime.EventsEmit

func SetEventEmitter(f func(ctx context.Context, optionalData string, optionalData2 ...interface{})) {
	eventEmitter = f
}

func emitStatus(ctx context.Context, runID string, stage string, message string) {
	payload := map[string]string{
		"runId":   runID,
		"stage":   stage,
		"message": message,
	}
	slog.Debug("agent status", "runId", runID, "stage", stage, "message", message)
	safeEmit(ctx, "agent:status", payload)
}

func emitContextUsage(ctx context.Context, runID string, usedTokens, maxTokens int) {
	payload := map[string]string{
		"runId":      runID,
		"usedTokens": strconv.Itoa(usedTokens),
		"maxTokens":  strconv.Itoa(maxTokens),
	}
	safeEmit(ctx, "agent:context", payload)
}

func retryChatWithTools(ctx context.Context, runID string, maxAttempts int, fn func() (*llm.ChatResponse, error)) (*llm.ChatResponse, error) {
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		resp, err := fn()
		if err == nil {
			return resp, nil
		}
		lastErr = err

		if !isRetriableLLMError(err) || attempt == maxAttempts {
			emitStatus(ctx, runID, "error", fmt.Sprintf("请求失败：%s", shortErr(err)))
			return nil, err
		}

		wait := retryBackoff(attempt)
		emitStatus(ctx, runID, "retrying", fmt.Sprintf("请求失败，正在重试（%d/%d），等待 %s... %s", attempt+1, maxAttempts, wait, shortErr(err)))

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}

	return nil, lastErr
}

func retryBackoff(attempt int) time.Duration {
	base := 300 * time.Millisecond
	max := 4 * time.Second
	wait := base * time.Duration(1<<(attempt-1))
	if wait > max {
		wait = max
	}
	jitter := time.Duration(time.Now().UnixNano()%250) * time.Millisecond
	return wait + jitter
}

func isRetriableLLMError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		if apiErr.HTTPStatusCode == 429 {
			return true
		}
		if apiErr.HTTPStatusCode >= 500 && apiErr.HTTPStatusCode <= 599 {
			return true
		}
		if apiErr.HTTPStatusCode == 408 {
			return true
		}
		return false
	}

	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "rate") && strings.Contains(msg, "limit") {
		return true
	}
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "temporarily") {
		return true
	}

	return true
}

func shortErr(err error) string {
	if err == nil {
		return ""
	}
	s := strings.TrimSpace(err.Error())
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}

// inferNextStepMessage 根据上一轮的工具调用推断下一步的意图，生成更有信息量的状态消息
func inferNextStepMessage(toolCalls []llm.ToolCall, step, maxSteps int) string {
	stepHint := fmt.Sprintf("（第 %d 步）", step+1)

	if len(toolCalls) == 0 {
		return "正在综合分析..." + stepHint
	}

	// 分析上一轮调用了哪些工具
	grepCount := 0
	readCount := 0
	for _, tc := range toolCalls {
		switch {
		case tc.Function.Name == "grep_knowledge":
			grepCount++
		case tc.Function.Name == "read_knowledge_file":
			readCount++
		}
	}

	switch {
	case grepCount > 0 && readCount > 0:
		return fmt.Sprintf("正在分析搜索和文档内容...%s", stepHint)
	case grepCount > 0:
		return fmt.Sprintf("正在根据搜索结果进一步分析...%s", stepHint)
	case readCount > 0:
		return fmt.Sprintf("正在综合文档内容...%s", stepHint)
	default:
		return fmt.Sprintf("正在思考下一步...%s", stepHint)
	}
}

