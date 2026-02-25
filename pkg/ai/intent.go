package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"opscopilot/pkg/config"
	"opscopilot/pkg/knowledge"
	"opscopilot/pkg/llm"
	"opscopilot/pkg/mcp"
	"opscopilot/pkg/sshclient"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type AIService struct {
	fastProvider    llm.Provider
	complexProvider llm.Provider
	cfgMgr          *config.Manager
	mcpClient       mcp.Client // MCP 客户端（可选）
	mcpManager      MCPManagerProvider // MCP 管理器（可选）
}

// MCPManagerProvider MCP 管理器接口
type MCPManagerProvider interface {
	GetAllClients() map[string]mcp.Client
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
		mcpClient:       nil, // 将在 App 启动后设置
	}
}

// SetMCPClient 设置 MCP 客户端（旧接口，保持兼容）
func (s *AIService) SetMCPClient(client mcp.Client) {
	s.mcpClient = client
}

// SetMCPManager 设置 MCP 管理器
func (s *AIService) SetMCPManager(manager MCPManagerProvider) {
	s.mcpManager = manager
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
		RetryMax:     5,
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

		runID := uuid.NewString()
		maxAttempts := 5
		for attempt := 1; attempt <= maxAttempts; attempt++ {
			resp, err := s.complexProvider.ChatCompletion(ctx, messages)
			if err == nil {
				return resp, nil
			}
			if !isRetriableLLMError(err) || attempt == maxAttempts {
				emitStatus(ctx, runID, "error", fmt.Sprintf("请求失败：%s", shortErr(err)))
				return "", err
			}
			wait := retryBackoff(attempt)
			emitStatus(ctx, runID, "retrying", fmt.Sprintf("请求失败，正在重试（%d/%d），等待 %s... %s", attempt+1, maxAttempts, wait, shortErr(err)))
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return "", ctx.Err()
			case <-timer.C:
			}
		}
		return "", err
	}

	return resp, nil
}

// TroubleshootResult 故障排查结果结构
type TroubleshootResult struct {
	OpsCopilotAnswer  string `json:"opsCopilotAnswer"`
	ExternalAnswer    string `json:"externalAnswer"`
	IntegratedAnswer  string `json:"integratedAnswer"`
	OpsCopilotReady   bool   `json:"opsCopilotReady"`
	ExternalReady     bool   `json:"externalReady"`
	IntegratedReady   bool   `json:"integratedReady"`
	ExternalError     string `json:"externalError,omitempty"`
}

func (s *AIService) AskTroubleshoot(ctx context.Context, problem string, knowledgeDir string, enableMCP bool) (string, error) {
	prompt := s.cfgMgr.Config.Prompts["troubleshoot_prompt"]
	if prompt == "" {
		prompt = config.DefaultTroubleshootPrompt
	}

	result := TroubleshootResult{
		OpsCopilotReady:  false,
		ExternalReady:    false,
		IntegratedReady:  false,
	}

	// 如果不启用 MCP，只运行知识库问答
	if !enableMCP {
		resp, err := s.RunAgent(ctx, AgentRunOptions{
			Question:     problem,
			KnowledgeDir: knowledgeDir,
			SystemPrompt: prompt,
			RetryMax:     5,
			EnableMCP:    false,
		})
		if err != nil {
			return "", err
		}
		return CleanJSONResponse(resp), nil
	}

	// 启用 MCP 时，并行运行知识库问答和 MCP 诊断
	var opsCopilotAnswer, externalAnswer string
	var opsCopilotErr, externalErr error
	var wg sync.WaitGroup

	// 使用 channel 来实现"先完成先返回"的逻辑
	opsCopilotDone := make(chan struct{})
	mcpDone := make(chan struct{})

	// 知识库问答
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(opsCopilotDone)
		resp, err := s.RunAgent(ctx, AgentRunOptions{
			Question:     problem,
			KnowledgeDir: knowledgeDir,
			SystemPrompt: prompt,
			RetryMax:     5,
			EnableMCP:    false, // 知识库问答不使用 MCP
		})
		if err != nil {
			opsCopilotErr = err
			return
		}
		opsCopilotAnswer = CleanJSONResponse(resp)
	}()

	// MCP 诊断
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(mcpDone)

		// 首先检查是否有可用的 MCP 工具
		hasMCPTools := false
		log.Printf("[AskTroubleshoot.MCP] Checking for MCP tools...")
		if s.mcpManager != nil {
			clients := s.mcpManager.GetAllClients()
			log.Printf("[AskTroubleshoot.MCP] Got %d clients from manager", len(clients))
			for serverName, client := range clients {
				log.Printf("[AskTroubleshoot.MCP] Checking server '%s', IsReady=%v", serverName, client.IsReady())
				if client.IsReady() {
					tools, err := client.ListTools(ctx)
					if err != nil {
						log.Printf("[AskTroubleshoot.MCP] Failed to list tools from '%s': %v", serverName, err)
					} else {
						log.Printf("[AskTroubleshoot.MCP] Server '%s' has %d tools", serverName, len(tools))
						if len(tools) > 0 {
							hasMCPTools = true
							for _, t := range tools {
								log.Printf("[AskTroubleshoot.MCP] Tool: %s - %s", t.Name, t.Description)
							}
							break
						}
					}
				}
			}
		} else {
			log.Printf("[AskTroubleshoot.MCP] MCP Manager is nil!")
		}

		if !hasMCPTools {
			log.Printf("[AskTroubleshoot] No MCP tools available")
			externalAnswer = "## MCP 诊断结果\n\n**当前没有可用的 MCP 诊断工具。**\n\n请确保：\n1. 已在 `mcp.json` 中正确配置 MCP 服务器\n2. MCP 服务器已正确启动\n3. MCP 服务器提供了诊断工具\n\n您可以在设置页面查看 MCP 服务器状态。"
			return
		}

		resp, err := s.RunAgent(ctx, AgentRunOptions{
			Question:     problem,
			KnowledgeDir: "", // MCP 诊断不需要知识库
			SystemPrompt: "你是 MCP 诊断助手。请使用可用的 MCP 工具对用户的问题进行诊断分析。\n\n规则：\n- 必须使用 MCP 工具获取诊断信息\n- 不要编造或幻觉出任何工具\n- 只使用实际可用的 MCP 工具\n- 用中文回答\n- 提供结构化的诊断报告",
			RetryMax:     5,
			EnableMCP:    true, // MCP 诊断使用 MCP 工具
		})
		if err != nil {
			externalErr = err
			return
		}
		externalAnswer = CleanJSONResponse(resp)
	}()

	// 等待知识库问答完成（这是主要结果，必须等待）
	<-opsCopilotDone

	// 设置知识库问答结果
	if opsCopilotErr != nil {
		log.Printf("[AskTroubleshoot] OpsCopilot error: %v", opsCopilotErr)
		result.OpsCopilotAnswer = fmt.Sprintf("知识库分析失败: %v", opsCopilotErr)
		result.OpsCopilotReady = false
	} else {
		result.OpsCopilotAnswer = opsCopilotAnswer
		result.OpsCopilotReady = true
	}

	// 等待 MCP 结果，但设置超时（最多等待 30 秒）
	select {
	case <-mcpDone:
		// MCP 完成
		if externalErr != nil {
			log.Printf("[AskTroubleshoot] MCP error: %v", externalErr)
			result.ExternalError = fmt.Sprintf("MCP 诊断失败: %v", externalErr)
			result.ExternalAnswer = ""
			result.ExternalReady = false
		} else {
			result.ExternalAnswer = externalAnswer
			result.ExternalReady = true
		}
	case <-time.After(30 * time.Second):
		// MCP 超时，先返回知识库结果
		log.Printf("[AskTroubleshoot] MCP timeout after 30s, returning LLM result first")
		result.ExternalAnswer = "MCP 诊断正在进行中，请稍后刷新查看结果..."
		result.ExternalReady = false
	}

	// 生成综合答复
	if result.OpsCopilotReady && result.ExternalReady {
		integratedPrompt := fmt.Sprintf(`请综合以下两个来源的诊断信息，生成一份完整的故障排查报告。

## 知识库分析结果
%s

## MCP 诊断结果
%s

## 要求
1. 综合两个来源的信息
2. 如果有冲突，以 MCP 诊断的实时数据为准
3. 提供明确的排查步骤和建议
4. 用中文回答`, result.OpsCopilotAnswer, result.ExternalAnswer)

		integratedResp, err := s.fastProvider.ChatCompletion(ctx, []llm.ChatMessage{
			{Role: "system", Content: "你是专业的运维诊断助手，负责综合多个来源的诊断信息生成报告。"},
			{Role: "user", Content: integratedPrompt},
		})
		if err != nil {
			log.Printf("[AskTroubleshoot] Integrated answer error: %v", err)
			result.IntegratedAnswer = "综合答复生成失败"
			result.IntegratedReady = false
		} else {
			result.IntegratedAnswer = integratedResp
			result.IntegratedReady = true
		}
	} else {
		// 如果 MCP 未就绪，使用知识库结果作为综合答复
		result.IntegratedAnswer = result.OpsCopilotAnswer
		result.IntegratedReady = result.OpsCopilotReady
	}

	// 返回 JSON 格式
	jsonData, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}
	return string(jsonData), nil
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
