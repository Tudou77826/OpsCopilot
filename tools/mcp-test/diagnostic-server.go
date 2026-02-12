package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// JSONRPCRequest JSON-RPC 2.0 请求
type JSONRPCRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      interface{}            `json:"id"`
	Method  string                 `json:"method,omitempty"`
	Params  map[string]interface{} `json:"params,omitempty"`
}

// JSONRPCResponse JSON-RPC 2.0 响应
type JSONRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      interface{}   `json:"id"`
	Result  interface{}   `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
}

// JSONRPCError JSON-RPC 2.0 错误
type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// ServerInfo 服务器信息
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ServerCapabilities 服务器能力
type ServerCapabilities struct {
	Tools map[string]interface{} `json:"tools"`
}

// Tool 工具定义
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// ToolListResult tools/list 响应
type ToolListResult struct {
	Tools []Tool `json:"tools"`
}

// ToolCallResult tools/call 响应
type ToolCallResult struct {
	Content []interface{} `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// TextContent 文本内容
type TextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func main() {
	// 服务器信息
	serverInfo := ServerInfo{
		Name:    "mcp-diagnostic-server",
		Version: "0.1.0",
	}

	// 定义可用的工具
	tools := []Tool{
		{
			Name:        "get_system_info",
			Description: "获取系统诊断信息，包括操作系统、架构等",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "read_log_file",
			Description: "读取指定的日志文件内容",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "日志文件路径",
					},
					"lines": map[string]interface{}{
						"type":        "integer",
						"description": "读取行数（默认100）",
					},
				},
				"required": []string{"path"},
			},
		},
		{
			Name:        "check_process",
			Description: "检查指定进程是否正在运行",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "进程名称",
					},
				},
				"required": []string{"name"},
			},
		},
	}

	// 从标准输入读取 JSON-RPC 请求
	scanner := bufio.NewScanner(os.Stdin)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			sendError(nil, -32700, "Parse error", nil)
			continue
		}

		// 处理请求
		handleRequest(req, serverInfo, tools)
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
		os.Exit(1)
	}
}

func handleRequest(req JSONRPCRequest, serverInfo ServerInfo, tools []Tool) {
	switch req.Method {
	case "initialize":
		sendResponse(req.ID, map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"serverInfo":      serverInfo,
			"capabilities":    ServerCapabilities{},
		})

	case "tools/list":
		sendResponse(req.ID, ToolListResult{Tools: tools})

	case "tools/call":
		handleToolCall(req, tools)

	default:
		sendError(req.ID, -32601, fmt.Sprintf("Method not found: %s", req.Method), nil)
	}
}

func handleToolCall(req JSONRPCRequest, tools []Tool) {
	name, ok := req.Params["name"].(string)
	if !ok {
		sendError(req.ID, -32602, "Invalid tool name", nil)
		return
	}

	args, _ := req.Params["arguments"].(map[string]interface{})

	var result string
	var err error

	switch name {
	case "get_system_info":
		result = getSystemInfo()

	case "read_log_file":
		path, _ := args["path"].(string)
		lines := 100
		if l, ok := args["lines"].(float64); ok {
			lines = int(l)
		}
		result, err = readLogFile(path, lines)

	case "check_process":
		procName, _ := args["name"].(string)
		result = checkProcess(procName)

	default:
		sendError(req.ID, -32602, fmt.Sprintf("Unknown tool: %s", name), nil)
		return
	}

	if err != nil {
		sendResponse(req.ID, ToolCallResult{
			Content: []interface{}{
				TextContent{Type: "text", Text: fmt.Sprintf("Error: %v", err)},
			},
			IsError: true,
		})
		return
	}

	sendResponse(req.ID, ToolCallResult{
		Content: []interface{}{
			TextContent{Type: "text", Text: result},
		},
	})
}

func getSystemInfo() string {
	return fmt.Sprintf("操作系统: %s\n架构: %s\n", os.Getenv("GOOS"), os.Getenv("GOARCH"))
}

func readLogFile(path string, lines int) (string, error) {
	// 简化实现：只返回模拟数据
	return fmt.Sprintf("[模拟] 读取日志文件 %s 的最后 %d 行\n[last line from log]\n...", path, lines), nil
}

func checkProcess(name string) string {
	// 简化实现：返回模拟状态
	return fmt.Sprintf("[模拟] 进程 %s 的状态检查结果\n进程状态: 运行中\nPID: 12345\n", name)
}

func sendResponse(id interface{}, result interface{}) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	sendJSON(resp)
}

func sendError(id interface{}, code int, message string, data interface{}) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	sendJSON(resp)
}

func sendJSON(resp interface{}) {
	data, err := json.Marshal(resp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling response: %v\n", err)
		return
	}
	fmt.Println(string(data))
}
