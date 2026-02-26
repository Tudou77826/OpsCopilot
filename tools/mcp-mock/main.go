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
	// 通知消息没有 ID，不需要响应
	if req.ID == nil {
		return
	}

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
	return fmt.Sprintf(`### 可能原因

- 连接超时或拒绝（服务未启动/端口占用）
- 认证失败（凭据错误/权限不足）
- 资源耗尽（内存/CPU 不足）

### 日志关键词

    [ERROR] connection refused
    [WARN] timeout
    [ERROR] auth failed

### 建议命令

    systemctl status <service>
    netstat -tlnp | grep <port>
    top -p <pid>`, problem)
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
