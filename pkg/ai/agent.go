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
	"strings"
	"time"

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
	EnableMCP    bool               // 是否启用 MCP 工具增强
	Catalog      *knowledge.Catalog // 知识库目录（注入 Agent 系统提示）
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
	"- 如果目录中没有与用户问题相关的场景，如实告知用户知识库中暂无相关排障文档，不要凭空编造排查建议\n" +
	"- 输出 Markdown 格式，用 ## 分节，命令用 ```bash 代码块\n" +
	"- 用中文回答\n" +
	"- 当调用 MCP 工具时，提供清晰结构化的问题描述\n" +
	"- Always follow additional system instructions about output format."

// RunAgent executes the ReAct loop
func (s *AIService) RunAgent(ctx context.Context, opts AgentRunOptions) (string, error) {
	runID := uuid.NewString()
	startAt := time.Now()

	// 创建工具注册器并注册知识库工具
	registry := tools.NewRegistry()
	registry.Register(knowledgetools.NewReadFileTool(opts.KnowledgeDir, opts.Catalog))
	registry.Register(knowledgetools.NewGrepTool(opts.KnowledgeDir))

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
	var systemPromptBuilder strings.Builder
	systemPromptBuilder.WriteString(agentBasePrompt)

	// 注入目录上下文
	if opts.Catalog != nil {
		catalogText := opts.Catalog.RenderForLLM()
		if catalogText != "" {
			systemPromptBuilder.WriteString("\n\n## 知识库问题目录\n\n")
			systemPromptBuilder.WriteString(catalogText)
			log.Printf("[Agent][%s] Catalog injected: %d scenarios, %d bytes",
				runID, opts.Catalog.TotalScenarios(), len(catalogText))
			log.Printf("[Agent][%s] Catalog content:\n%s", runID, catalogText)
		} else {
			log.Printf("[Agent][%s] Catalog present but empty (0 scenarios)", runID)
		}
	} else {
		log.Printf("[Agent][%s] No catalog available", runID)
	}

	if opts.SystemPrompt != "" {
		systemPromptBuilder.WriteString("\n\n")
		systemPromptBuilder.WriteString(opts.SystemPrompt)
	}

	systemPrompt := systemPromptBuilder.String()
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
		if i == 0 {
			emitStatus(ctx, runID, "thinking", "正在分析问题，扫描知识库目录...")
		} else {
			emitStatus(ctx, runID, "thinking", "正在思考下一步...")
		}
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
		if resp.Content != "" {
			log.Printf("[Agent][%s] Step=%d LLM thinking: %s", runID, i+1, resp.Content)
		}
		for idx, tc := range resp.ToolCalls {
			log.Printf("[Agent][%s] Step=%d ToolCall#%d name=%s args=%s", runID, i+1, idx+1, tc.Function.Name, tc.Function.Arguments)
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

