package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"opscopilot/pkg/knowledge"
	"opscopilot/pkg/llm"
	"opscopilot/pkg/mcp"
	"opscopilot/pkg/tools"
	knowledgetools "opscopilot/pkg/tools/knowledge"
	"os"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	openai "github.com/sashabaranov/go-openai"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// AgentRunOptions defines options for the agent execution
type AgentRunOptions struct {
	Question     string
	KnowledgeDir string
	SystemPrompt string
	RetryMax     int
	EnableMCP    bool // 是否启用 MCP 工具增强
}

const agentToolPrompt = "You are OpsCopilot running in Agent mode. You have access to a local knowledge base and the following tools:\n" +
	"1) search_knowledge: search within documentation content and return top matches with snippets\n" +
	"2) list_knowledge_files: list available markdown docs\n" +
	"3) read_knowledge_file: read a specific markdown doc by relative path\n" +
	"4) MCP tools: Additional diagnostic tools available through MCP (if configured and enabled)\n\n" +
	"Rules:\n" +
	"- You MUST call search_knowledge at least once before answering.\n" +
	"- When calling search_knowledge, keep the query short (keywords/phrases), not the full problem statement.\n" +
	"- Use the search results to choose which file(s) to read (usually 1-3) before answering.\n" +
	"- Call list_knowledge_files only if search results are empty or you need to explore the available docs.\n" +
	"- Only read files that are relevant to the user's question.\n" +
	"- If MCP tools are available, you may use them for advanced diagnostic capabilities.\n" +
	"- When calling MCP diagnostic tools, provide a clear and structured problem description, not just raw user input.\n" +
	"- Structure the problem description for MCP tools: include symptoms, context, and what you've already checked.\n" +
	"- If the knowledge base does not contain the answer, say so and then answer from general knowledge.\n" +
	"- Always follow additional system instructions about output format.\n" +
	"- Always answer in the same language as the user."

// RunAgent executes the ReAct loop
func (s *AIService) RunAgent(ctx context.Context, opts AgentRunOptions) (string, error) {
	runID := uuid.NewString()
	startAt := time.Now()
	termCache := &simpleTermCache{data: make(map[string][]knowledge.WeightedTerm)}

	// 创建工具注册器并注册知识库工具
	registry := tools.NewRegistry()
	registry.Register(knowledgetools.NewSearchTool(
		opts.KnowledgeDir,
		s,
		knowledgetools.WithOriginalQuery(opts.Question),
		knowledgetools.WithTermCache(termCache),
		knowledgetools.WithRetryMax(opts.RetryMax),
	))
	registry.Register(knowledgetools.NewListFilesTool(opts.KnowledgeDir))
	registry.Register(knowledgetools.NewReadFileTool(opts.KnowledgeDir))

	// 构建LLM工具列表
	llmTools := registry.ToLLMTools()

	// 添加 MCP 工具（如果启用且可用）
	// 优先使用 MCP Manager，如果没有则使用单个 mcpClient
	if opts.EnableMCP {
		if s.mcpManager != nil {
			clients := s.mcpManager.GetAllClients()
			for serverName, client := range clients {
				if client.IsReady() {
					mcpTools, err := client.ListTools(ctx)
					if err != nil {
						log.Printf("[Agent][%s] Warning: Failed to list MCP tools from %s: %v", runID, serverName, err)
					} else if len(mcpTools) > 0 {
						log.Printf("[Agent][%s] Adding %d MCP tools from %s to agent (MCP enabled)", runID, len(mcpTools), serverName)
						mcpLLMTools := mcp.ToLLMTools(mcpTools)
						llmTools = append(llmTools, mcpLLMTools...)
					}
				}
			}
		} else if s.mcpClient != nil && s.mcpClient.IsReady() {
			mcpTools, err := s.mcpClient.ListTools(ctx)
			if err != nil {
				log.Printf("[Agent][%s] Warning: Failed to list MCP tools: %v", runID, err)
			} else if len(mcpTools) > 0 {
				log.Printf("[Agent][%s] Adding %d MCP tools to agent (MCP enabled)", runID, len(mcpTools))
				mcpLLMTools := mcp.ToLLMTools(mcpTools)
				llmTools = append(llmTools, mcpLLMTools...)
			}
		}
	} else {
		log.Printf("[Agent][%s] MCP tools disabled by user", runID)
	}

	// 合并 system messages 为一个，避免某些模型报错 "System message must be at the beginning"
	systemPrompt := agentToolPrompt
	if opts.SystemPrompt != "" {
		systemPrompt = agentToolPrompt + "\n\n" + opts.SystemPrompt
	}
	messages := []llm.ChatMessage{{Role: "system", Content: systemPrompt}}
	messages = append(messages, llm.ChatMessage{Role: "user", Content: opts.Question})

	provider := s.complexProvider
	maxSteps := 10

	knowledgeExists := false
	if opts.KnowledgeDir != "" {
		if st, err := os.Stat(opts.KnowledgeDir); err == nil && st.IsDir() {
			knowledgeExists = true
		}
	}

	log.Printf("[Agent][%s] Start questionLen=%d knowledgeDir=%q knowledgeExists=%t tools=%d", runID, len(opts.Question), opts.KnowledgeDir, knowledgeExists, len(llmTools))

	for i := 0; i < maxSteps; i++ {
		emitStatus(ctx, runID, "thinking", "正在思考下一步...")
		stepAt := time.Now()
		resp, err := retryChatWithTools(ctx, runID, opts.RetryMax, func() (*llm.ChatResponse, error) {
			return provider.ChatWithTools(ctx, messages, llmTools)
		})
		llmCost := time.Since(stepAt)
		if err != nil {
			log.Printf("[Agent][%s] Step=%d LLMError cost=%s err=%v", runID, i+1, llmCost, err)
			return "", err
		}

		log.Printf("[Agent][%s] Step=%d LLMOk cost=%s contentLen=%d toolCalls=%d", runID, i+1, llmCost, len(resp.Content), len(resp.ToolCalls))
		for idx, tc := range resp.ToolCalls {
			log.Printf("[Agent][%s] Step=%d ToolCall#%d id=%s name=%s argsLen=%d", runID, i+1, idx+1, tc.ID, tc.Function.Name, len(tc.Function.Arguments))
		}

		messages = append(messages, llm.ChatMessage{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		if len(resp.ToolCalls) == 0 {
			if i == 0 {
				emitStatus(ctx, runID, "answering", "模型未调用工具，直接生成回答...")
			} else {
				emitStatus(ctx, runID, "answering", "正在生成回答...")
			}
			log.Printf("[Agent][%s] Done totalCost=%s", runID, time.Since(startAt))
			return resp.Content, nil
		}

		for _, tc := range resp.ToolCalls {
			var toolResult string

			log.Printf("[Agent][%s] ExecuteTool name=%s", runID, tc.Function.Name)

			// 优先使用注册器中的工具（知识库工具）
			if tool, ok := registry.Get(tc.Function.Name); ok {
				var args map[string]interface{}
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					toolResult = fmt.Sprintf("Error parsing arguments: %v", err)
					log.Printf("[Agent][%s] ToolErr name=%s parseArgsErr=%v", runID, tc.Function.Name, err)
				} else {
					toolAt := time.Now()
					statusEmitter := func(stage, message string) {
						emitStatus(ctx, runID, stage, message)
					}
					result, err := tool.Execute(ctx, args, statusEmitter)
					toolCost := time.Since(toolAt)
					if err != nil {
						toolResult = fmt.Sprintf("Error: %v", err)
						log.Printf("[Agent][%s] ToolErr name=%s cost=%s err=%v", runID, tc.Function.Name, toolCost, err)
					} else {
						toolResult = result
						log.Printf("[Agent][%s] ToolOk name=%s cost=%s resultLen=%d", runID, tc.Function.Name, toolCost, len(toolResult))
					}
				}
			} else if mcp.IsMCPTool(tc.Function.Name) {
				// MCP工具处理
				log.Printf("[Agent][%s] Executing MCP tool: %s", runID, tc.Function.Name)
				emitStatus(ctx, runID, "mcp_call", fmt.Sprintf("正在调用 MCP 工具: %s...", tc.Function.Name))

				var args map[string]interface{}
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					toolResult = fmt.Sprintf("Error parsing MCP tool arguments: %v", err)
					log.Printf("[Agent][%s] MCPToolErr name=%s parseArgsErr=%v", runID, tc.Function.Name, err)
				} else {
					// 尝试从 MCP Manager 查找能处理此工具的客户端
					var result string
					var err error

					if s.mcpManager != nil {
						clients := s.mcpManager.GetAllClients()
						// 遍历所有客户端，找到能处理此工具的
						for serverName, client := range clients {
							if client.IsReady() {
								// 先列出工具，看是否包含此工具
								mcpTools, listErr := client.ListTools(ctx)
								if listErr != nil {
									continue
								}

								// 检查工具是否在这个客户端中
								found := false
								for _, tool := range mcpTools {
									if tool.Name == tc.Function.Name {
										found = true
										break
									}
								}

								if found {
									toolAt := time.Now()
									result, err = client.CallTool(ctx, tc.Function.Name, args)
									toolCost := time.Since(toolAt)
									if err != nil {
										toolResult = mcp.FormatToolCallResult(tc.Function.Name, "", err)
										log.Printf("[Agent][%s] MCPToolErr name=%s server=%s cost=%s err=%v", runID, tc.Function.Name, serverName, toolCost, err)
									} else {
										toolResult = mcp.FormatToolCallResult(tc.Function.Name, result, nil)
										log.Printf("[Agent][%s] MCPToolOk name=%s server=%s cost=%s resultLen=%d", runID, tc.Function.Name, serverName, toolCost, len(result))
									}
									break
								}
							}
						}

						// 如果所有客户端都无法处理，返回错误
						if result == "" && err == nil {
							toolResult = fmt.Sprintf("Error: No MCP server found that can handle tool %s", tc.Function.Name)
							log.Printf("[Agent][%s] MCPToolErr name=%s noServerFound=true", runID, tc.Function.Name)
						}
					} else if s.mcpClient != nil && s.mcpClient.IsReady() {
						// 回退到单个客户端模式
						toolAt := time.Now()
						result, err = s.mcpClient.CallTool(ctx, tc.Function.Name, args)
						toolCost := time.Since(toolAt)
						if err != nil {
							toolResult = mcp.FormatToolCallResult(tc.Function.Name, "", err)
							log.Printf("[Agent][%s] MCPToolErr name=%s cost=%s err=%v", runID, tc.Function.Name, toolCost, err)
						} else {
							toolResult = mcp.FormatToolCallResult(tc.Function.Name, result, nil)
							log.Printf("[Agent][%s] MCPToolOk name=%s cost=%s resultLen=%d", runID, tc.Function.Name, toolCost, len(result))
						}
					} else {
						toolResult = fmt.Sprintf("Error: MCP not available for tool %s", tc.Function.Name)
						log.Printf("[Agent][%s] MCPToolErr name=%s notAvailable=true", runID, tc.Function.Name)
					}
				}
			} else {
				toolResult = fmt.Sprintf("Error: Unknown tool %s", tc.Function.Name)
				log.Printf("[Agent][%s] ToolErr name=%s unknownTool=true", runID, tc.Function.Name)
			}

			messages = append(messages, llm.ChatMessage{
				Role:       "tool",
				ToolCallID: tc.ID,
				Name:       tc.Function.Name,
				Content:    toolResult,
			})
		}

		log.Printf("[Agent][%s] Step=%d toolOutputsAppended=%d messageCount=%d", runID, i+1, len(resp.ToolCalls), len(messages))
	}

	log.Printf("[Agent][%s] ExceededMaxSteps totalCost=%s maxSteps=%d", runID, time.Since(startAt), maxSteps)
	return "", fmt.Errorf("agent exceeded maximum steps (%d) without reaching a conclusion", maxSteps)
}

func safeEmit(ctx context.Context, eventName string, data interface{}) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from emit panic: %v", r)
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
	log.Printf("[Agent][%s] Status stage=%s message=%s", runID, stage, message)
	safeEmit(ctx, "agent:status", payload)
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

func shortText(s string, max int) string {
	t := strings.TrimSpace(s)
	t = strings.ReplaceAll(t, "\r", " ")
	t = strings.ReplaceAll(t, "\n", " ")
	if max <= 0 || len(t) <= max {
		return t
	}
	return t[:max] + "..."
}

func chooseSearchKey(original string, model string) string {
	o := strings.TrimSpace(original)
	m := strings.TrimSpace(model)
	if o == "" && m == "" {
		return ""
	}
	if m == "" {
		return o
	}
	if o == "" {
		return m
	}
	if containsHan(o) && !containsHan(m) {
		return o
	}
	if containsHan(m) && !containsHan(o) {
		return m
	}
	if len([]rune(m)) > 0 && len([]rune(m)) < len([]rune(o)) {
		return m
	}
	return o
}

func formatWeightedTerms(terms []knowledge.WeightedTerm, maxItems int, maxChars int) string {
	if len(terms) == 0 {
		return ""
	}
	cp := append([]knowledge.WeightedTerm(nil), terms...)
	sort.Slice(cp, func(i, j int) bool {
		if cp[i].Weight == cp[j].Weight {
			return cp[i].Term < cp[j].Term
		}
		return cp[i].Weight > cp[j].Weight
	})
	if maxItems > 0 && len(cp) > maxItems {
		cp = cp[:maxItems]
	}
	parts := make([]string, 0, len(cp))
	for _, t := range cp {
		term := strings.TrimSpace(t.Term)
		if term == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s(%.1f)", term, t.Weight))
	}
	out := strings.Join(parts, ", ")
	return shortText(out, maxChars)
}

func (s *AIService) extractWeightedTerms(ctx context.Context, runID string, maxAttempts int, text string) ([]knowledge.WeightedTerm, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("empty text")
	}
	if s.fastProvider == nil {
		return nil, fmt.Errorf("fast provider not initialized")
	}

	const prompt = "You are a search query analyzer. Extract weighted keywords from the user's description for document retrieval.\n" +
		"Output ONLY a valid JSON array of objects, no markdown and no extra text.\n" +
		"Each object: {\"term\": string, \"weight\": number}.\n" +
		"Rules:\n" +
		"- 5-10 terms.\n" +
		"- Prefer nouns/metrics/components/errors/commands.\n" +
		"- Terms should be short (1-6 words) and searchable.\n" +
		"- weight is 1-5 where 5 is most important.\n" +
		"- IMPORTANT LANGUAGE RULE: Always keep key terms in the user's original language.\n" +
		"- If you add translations/synonyms, include them as additional terms. Do NOT replace the original language terms.\n" +
		"- Keep commands/error codes/service names as-is.\n"

	messages := []llm.ChatMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: text},
	}

	raw, err := retryChatCompletion(ctx, runID, maxAttempts, func() (string, error) {
		return s.fastProvider.ChatCompletion(ctx, messages)
	})
	if err != nil {
		return nil, err
	}

	cleaned := CleanJSONResponse(raw)

	var terms []knowledge.WeightedTerm
	if err := json.Unmarshal([]byte(cleaned), &terms); err != nil {
		var wrapper struct {
			Terms []knowledge.WeightedTerm `json:"terms"`
		}
		if err2 := json.Unmarshal([]byte(cleaned), &wrapper); err2 != nil {
			return nil, err
		}
		terms = wrapper.Terms
	}

	seen := map[string]float64{}
	out := make([]knowledge.WeightedTerm, 0, len(terms))
	for _, t := range terms {
		term := strings.ToLower(strings.TrimSpace(t.Term))
		if term == "" {
			continue
		}
		w := t.Weight
		if w <= 0 {
			w = 1
		}
		if w > 5 {
			w = 5
		}
		if cur, ok := seen[term]; ok && cur >= w {
			continue
		}
		seen[term] = w
		out = append(out, knowledge.WeightedTerm{Term: term, Weight: w})
	}

	if containsHan(text) && !anyContainsHanTerms(out) {
		for _, seg := range extractHanSegments(text, 6) {
			term := strings.ToLower(strings.TrimSpace(seg))
			if term == "" {
				continue
			}
			if _, ok := seen[term]; ok {
				continue
			}
			seen[term] = 4
			out = append(out, knowledge.WeightedTerm{Term: term, Weight: 4})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Weight == out[j].Weight {
			return out[i].Term < out[j].Term
		}
		return out[i].Weight > out[j].Weight
	})
	if len(out) > 10 {
		out = out[:10]
	}
	return out, nil
}

func containsHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func anyContainsHanTerms(terms []knowledge.WeightedTerm) bool {
	for _, t := range terms {
		if containsHan(t.Term) {
			return true
		}
	}
	return false
}

func extractHanSegments(s string, maxItems int) []string {
	var segs []string
	var sb strings.Builder
	flush := func() {
		txt := strings.TrimSpace(sb.String())
		sb.Reset()
		if len([]rune(txt)) < 2 {
			return
		}
		segs = append(segs, txt)
	}
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			sb.WriteRune(r)
			continue
		}
		flush()
		if maxItems > 0 && len(segs) >= maxItems {
			return segs
		}
	}
	flush()
	if maxItems > 0 && len(segs) > maxItems {
		return segs[:maxItems]
	}
	return segs
}

func retryChatCompletion(ctx context.Context, runID string, maxAttempts int, fn func() (string, error)) (string, error) {
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
			return "", err
		}
		wait := retryBackoff(attempt)
		emitStatus(ctx, runID, "retrying", fmt.Sprintf("分词请求失败，正在重试（%d/%d），等待 %s... %s", attempt+1, maxAttempts, wait, shortErr(err)))
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}
	return "", lastErr
}

// ExtractWeightedTerms 实现TermExtractor接口，提取加权关键词
func (s *AIService) ExtractWeightedTerms(ctx context.Context, text string) ([]knowledge.WeightedTerm, error) {
	return s.extractWeightedTerms(ctx, "", 3, text)
}

// ExtractWeightedTermsWithRetry 实现TermExtractorWithRetry接口
func (s *AIService) ExtractWeightedTermsWithRetry(ctx context.Context, text string, maxAttempts int) ([]knowledge.WeightedTerm, error) {
	return s.extractWeightedTerms(ctx, "", maxAttempts, text)
}

// simpleTermCache 简单的词项缓存实现
type simpleTermCache struct {
	data map[string][]knowledge.WeightedTerm
}

func (c *simpleTermCache) Get(key string) []knowledge.WeightedTerm {
	return c.data[key]
}

func (c *simpleTermCache) Set(key string, terms []knowledge.WeightedTerm) {
	c.data[key] = terms
}
