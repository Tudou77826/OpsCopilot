package mcpserver

import (
	"encoding/json"
	"fmt"
)

// HandleRequest 处理 MCP 请求
func (s *Server) HandleRequest(req *JSONRPCRequest) {
	// 通知消息没有 ID，不需要响应
	if req.ID == nil {
		return
	}

	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "notifications/initialized":
		// 客户端确认初始化完成，无需响应
	case "tools/list":
		s.handleToolsList(req)
	case "tools/call":
		s.handleToolsCall(req)
	case "ping":
		s.SendResponse(req.ID, map[string]interface{}{})
	default:
		// 对于未知方法，返回空结果（MCP 协议要求）
		s.SendResponse(req.ID, map[string]interface{}{})
	}
}

// handleInitialize 处理初始化请求
func (s *Server) handleInitialize(req *JSONRPCRequest) {
	s.SendResponse(req.ID, map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{
				"listChanged": false,
			},
		},
		"serverInfo": map[string]interface{}{
			"name":    "opscopilot-mcp-server",
			"version": "1.0.0",
		},
	})
}

// Tool MCP 工具定义
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// handleToolsList 处理工具列表请求
func (s *Server) handleToolsList(req *JSONRPCRequest) {
	tools := []Tool{
		{
			Name:        "server",
			Description: "服务器连接管理。通过 action 参数执行不同操作：list（列出服务器）、connect（连接）、disconnect（断开）。",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"action": map[string]interface{}{
						"type":        "string",
						"description": "操作类型：list、connect、disconnect",
						"enum":        []string{"list", "connect", "disconnect"},
					},
					"server": map[string]interface{}{
						"type":        "string",
						"description": "服务器名称（connect/disconnect 时必填）",
					},
				},
				"required": []string{"action"},
			},
		},
		{
			Name: "ssh_exec",
			Description: `在远程服务器上执行命令。

命令执行受白名单策略控制，不同服务器可能有不同的命令权限。
白名单可通过 OpsCopilot Web UI 进行细粒度配置。

常见允许的命令类别（具体取决于配置）：
- 文件：cat, head, tail, ls, find, grep, awk, jq
- 进程：ps, top, pgrep, pstree
- 资源：free, df, du, uptime, iostat, vmstat
- 网络：netstat, ss, ip, ping, curl
- 服务：systemctl status, journalctl, dmesg
- 容器：docker ps/logs, kubectl get/logs
- Java：jps, jstat, jstack, jmap -histo

⚠️ 注意：
- 实际可用命令取决于服务器 IP 对应的白名单策略
- 如果命令被拒绝，错误信息会显示该 IP 可用的命令
- 可联系管理员配置更多命令

输出控制：
- 总输出限制为 10KB
- 可通过 max_line_length 参数控制单行最大长度（默认 500 字）`,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"server": map[string]interface{}{
						"type":        "string",
						"description": "服务器名称（必须是已连接的服务器）",
					},
					"command": map[string]interface{}{
						"type":        "string",
						"description": "要执行的命令",
					},
					"max_line_length": map[string]interface{}{
						"type":        "integer",
						"description": "单行最大长度，默认 500",
						"default":     500,
					},
					"note": map[string]interface{}{
						"type":        "string",
						"description": "命令说明（用于审计记录）",
					},
				},
				"required": []string{"server", "command"},
			},
		},
		{
			Name:        "file_transfer",
			Description: "远程文件传输（SFTP）。通过 action 参数执行不同操作：download（下载）、upload（上传）。",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"action": map[string]interface{}{
						"type":        "string",
						"description": "操作类型：download、upload",
						"enum":        []string{"download", "upload"},
					},
					"server": map[string]interface{}{
						"type":        "string",
						"description": "服务器名称（必须是已连接的服务器）",
					},
					"remote_path": map[string]interface{}{
						"type":        "string",
						"description": "远程文件路径",
					},
					"local_path": map[string]interface{}{
						"type":        "string",
						"description": "本地文件路径",
					},
					"max_bytes": map[string]interface{}{
						"type":        "integer",
						"description": "最大下载字节数，默认 10MB（download 时有效）",
						"default":     10485760,
					},
					"backup": map[string]interface{}{
						"type":        "boolean",
						"description": "覆盖前自动备份远程文件，默认 true（upload 时有效）",
						"default":     true,
					},
					"mkdir": map[string]interface{}{
						"type":        "boolean",
						"description": "自动创建远程目标目录，默认 false（upload 时有效）",
						"default":     false,
					},
				},
				"required": []string{"action", "server", "remote_path", "local_path"},
			},
		},
	}

	s.SendResponse(req.ID, map[string]interface{}{
		"tools": tools,
	})
}

// handleToolsCall 处理工具调用
func (s *Server) handleToolsCall(req *JSONRPCRequest) {
	params := req.Params
	if params == nil {
		s.SendError(req.ID, -32602, "Invalid params: missing params")
		return
	}

	toolName, ok := params["name"].(string)
	if !ok {
		s.SendError(req.ID, -32602, "Invalid params: missing tool name")
		return
	}

	arguments, _ := params["arguments"].(map[string]interface{})

	var result interface{}
	var err error

	switch toolName {
	case "server":
		result, err = s.toolServer(arguments)
	case "ssh_exec":
		result, err = s.toolSSHExec(arguments)
	case "file_transfer":
		result, err = s.toolFileTransfer(arguments)
	default:
		s.SendError(req.ID, -32601, fmt.Sprintf("Tool not found: %s", toolName))
		return
	}

	if err != nil {
		s.SendResponse(req.ID, map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": err.Error(),
				},
			},
			"isError": true,
		})
		return
	}

	// 将结果转换为 JSON 字符串
	resultJSON, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		s.SendResponse(req.ID, map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": fmt.Sprintf("%v", result),
				},
			},
		})
		return
	}

	s.SendResponse(req.ID, map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": string(resultJSON),
			},
		},
	})
}
