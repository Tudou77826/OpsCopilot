package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

// JSON-RPC 请求结构
type JSONRPCRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      interface{}            `json:"id"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params,omitempty"`
}

// JSON-RPC 响应结构
type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCP 工具定义
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// 工具列表结果
type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

// 工具调用结果
type ToolCallResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Bytes()

		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			sendError(nil, -32700, "Parse error")
			continue
		}

		handleRequest(req)
	}
}

func handleRequest(req JSONRPCRequest) {
	switch req.Method {
	case "initialize":
		sendResult(req.ID, map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			"serverInfo": map[string]interface{}{
				"name":    "mock-diagnostic-server",
				"version": "1.0.0",
			},
		})

	case "tools/list":
		tools := []Tool{
			{
				Name:        "mcp_diagnose_problem",
				Description: "诊断问题并提供定位指导。输入问题描述，返回详细的排查思路和可能的解决方案。",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"problem": map[string]interface{}{
							"type":        "string",
							"description": "用户遇到的问题描述",
						},
					},
					"required": []string{"problem"},
				},
			},
		}
		sendResult(req.ID, ToolsListResult{Tools: tools})

	case "tools/call":
		handleToolCall(req)

	default:
		// 对于未知方法，返回空结果（MCP 协议要求）
		sendResult(req.ID, map[string]interface{}{})
	}
}

func handleToolCall(req JSONRPCRequest) {
	params := req.Params
	if params == nil {
		sendError(req.ID, -32602, "Invalid params")
		return
	}

	toolName, _ := params["name"].(string)
	arguments, _ := params["arguments"].(map[string]interface{})

	var result string
	switch toolName {
	case "mcp_diagnose_problem":
		problem, _ := arguments["problem"].(string)
		result = generateDiagnosticResponse(problem)
	default:
		sendError(req.ID, -32601, fmt.Sprintf("Tool not found: %s", toolName))
		return
	}

	sendResult(req.ID, ToolCallResult{
		Content: []ContentItem{
			{Type: "text", Text: result},
		},
	})
}

func generateDiagnosticResponse(problem string) string {
	// 返回模拟的中文定位指导
	return fmt.Sprintf(`## MCP 诊断分析报告

**问题描述**: %s

### 初步分析

这是一个模拟的 MCP 诊断响应。在实际场景中，MCP 服务器会连接到真实的诊断系统进行分析。

### 建议排查步骤

1. **检查基础配置**
   - 确认相关服务是否正常运行
   - 检查配置文件是否正确

2. **查看日志信息**
   - 检查应用日志中的错误信息
   - 关注异常堆栈和错误码

3. **网络连通性测试**
   - 验证网络连接是否正常
   - 检查防火墙规则

4. **资源使用情况**
   - 检查 CPU、内存使用率
   - 确认磁盘空间充足

### 可能的原因

- 配置错误或缺失
- 服务依赖问题
- 资源不足
- 网络问题

### 建议解决方案

1. 根据上述排查步骤逐一检查
2. 参考官方文档获取详细配置指南
3. 如果问题持续，请联系技术支持

---
*此响应由 Mock MCP 服务器生成，用于测试 MCP 集成功能。*`, problem)
}

func sendResult(id interface{}, result interface{}) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	sendResponse(resp)
}

func sendError(id interface{}, code int, message string) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: message,
		},
	}
	sendResponse(resp)
}

func sendResponse(resp JSONRPCResponse) {
	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
}
