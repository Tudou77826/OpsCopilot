package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"opscopilot/pkg/knowledge"
	"opscopilot/pkg/llm"
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
}

const agentToolPrompt = "You are OpsCopilot running in Agent mode. You have access to a local knowledge base and the following tools:\n" +
	"1) list_knowledge_files: list available markdown docs\n" +
	"2) read_knowledge_file: read a specific markdown doc by relative path\n\n" +
	"Rules:\n" +
	"- You MUST call list_knowledge_files at least once before answering.\n" +
	"- Then call read_knowledge_file for the most relevant file(s) (usually 1-3) before answering.\n" +
	"- Only read files that are relevant to the user's question.\n" +
	"- If the knowledge base does not contain the answer, say so and then answer from general knowledge.\n" +
	"- Always follow additional system instructions about output format.\n" +
	"- Always answer in the same language as the user."

// RunAgent executes the ReAct loop
func (s *AIService) RunAgent(ctx context.Context, opts AgentRunOptions) (string, error) {
	runID := uuid.NewString()
	startAt := time.Now()

	toolDefs := knowledge.GetToolDefinitions()
	tools := []llm.Tool{
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        knowledge.ToolListFiles,
				Description: "List all available documentation files in the knowledge base. Use this first to explore available topics.",
				Parameters:  toolDefs[knowledge.ToolListFiles],
			},
		},
		{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        knowledge.ToolReadFile,
				Description: "Read the content of a specific documentation file. Only read files that are relevant to the user's question.",
				Parameters:  toolDefs[knowledge.ToolReadFile],
			},
		},
	}

	messages := []llm.ChatMessage{{Role: "system", Content: agentToolPrompt}}
	if opts.SystemPrompt != "" {
		messages = append(messages, llm.ChatMessage{Role: "system", Content: opts.SystemPrompt})
	}
	messages = append(messages, llm.ChatMessage{Role: "user", Content: opts.Question})

	provider := s.complexProvider
	maxSteps := 10

	knowledgeExists := false
	if opts.KnowledgeDir != "" {
		if st, err := os.Stat(opts.KnowledgeDir); err == nil && st.IsDir() {
			knowledgeExists = true
		}
	}

	log.Printf("[Agent][%s] Start questionLen=%d knowledgeDir=%q knowledgeExists=%t tools=%d", runID, len(opts.Question), opts.KnowledgeDir, knowledgeExists, len(tools))

	for i := 0; i < maxSteps; i++ {
		emitStatus(ctx, runID, "thinking", "正在思考下一步...")
		stepAt := time.Now()
		resp, err := retryChatWithTools(ctx, runID, opts.RetryMax, func() (*llm.ChatResponse, error) {
			return provider.ChatWithTools(ctx, messages, tools)
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

			switch tc.Function.Name {
			case knowledge.ToolListFiles:
				emitStatus(ctx, runID, "searching", "正在检索文档列表...")
				toolAt := time.Now()
				files, err := knowledge.ListFiles(opts.KnowledgeDir)
				toolCost := time.Since(toolAt)
				if err != nil {
					toolResult = fmt.Sprintf("Error listing files: %v", err)
					log.Printf("[Agent][%s] ToolErr name=%s cost=%s err=%v", runID, tc.Function.Name, toolCost, err)
				} else {
					js, _ := json.Marshal(files)
					toolResult = string(js)
					log.Printf("[Agent][%s] ToolOk name=%s cost=%s files=%d", runID, tc.Function.Name, toolCost, len(files))
				}

			case knowledge.ToolReadFile:
				var args struct {
					Path string `json:"path"`
				}
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					toolResult = fmt.Sprintf("Error parsing arguments: %v", err)
					log.Printf("[Agent][%s] ToolErr name=%s parseArgsErr=%v", runID, tc.Function.Name, err)
				} else {
					emitStatus(ctx, runID, "reading", fmt.Sprintf("正在阅读文档: %s...", args.Path))
					toolAt := time.Now()
					content, err := knowledge.ReadFile(opts.KnowledgeDir, args.Path)
					toolCost := time.Since(toolAt)
					if err != nil {
						toolResult = fmt.Sprintf("Error reading file: %v", err)
						log.Printf("[Agent][%s] ToolErr name=%s cost=%s path=%q err=%v", runID, tc.Function.Name, toolCost, args.Path, err)
					} else {
						if len(content) > 20000 {
							toolResult = content[:20000] + "\n...(truncated)..."
							log.Printf("[Agent][%s] ToolOk name=%s cost=%s path=%q contentLen=%d truncated=true", runID, tc.Function.Name, toolCost, args.Path, len(content))
						} else {
							toolResult = content
							log.Printf("[Agent][%s] ToolOk name=%s cost=%s path=%q contentLen=%d truncated=false", runID, tc.Function.Name, toolCost, args.Path, len(content))
						}
					}
				}

			default:
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
