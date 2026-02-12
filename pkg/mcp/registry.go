package mcp

import (
	"encoding/json"
	"fmt"
	"opscopilot/pkg/llm"
)

// ToLLMTool 将 MCP 工具转换为 LLM Tool 格式
func ToLLMTool(tool Tool) llm.Tool {
	// 将 InputSchema (map[string]interface{}) 转换为 json.RawMessage
	var parameters json.RawMessage
	if tool.InputSchema != nil {
		data, err := json.Marshal(tool.InputSchema)
		if err == nil {
			parameters = data
		}
	}

	return llm.Tool{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  parameters,
		},
	}
}

// ToLLMTools 批量转换 MCP 工具列表
func ToLLMTools(mcpTools []Tool) []llm.Tool {
	tools := make([]llm.Tool, len(mcpTools))
	for i, t := range mcpTools {
		tools[i] = ToLLMTool(t)
	}
	return tools
}

// IsMCPTool 判断工具名称是否为 MCP 工具
// 知识库工具前缀: search_, list_, read_
func IsMCPTool(toolName string) bool {
	return !isKnowledgeTool(toolName)
}

// isKnowledgeTool 判断是否为知识库工具
func isKnowledgeTool(toolName string) bool {
	switch toolName {
	case "search_knowledge", "list_knowledge_files", "read_knowledge_file":
		return true
	default:
		return false
	}
}

// FormatToolCallResult 格式化 MCP 工具调用结果为 Agent 可理解的文本
func FormatToolCallResult(toolName string, result string, err error) string {
	if err != nil {
		return fmt.Sprintf("Tool %s error: %v", toolName, err)
	}
	return result
}
