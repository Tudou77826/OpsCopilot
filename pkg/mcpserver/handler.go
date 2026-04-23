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
			Name: "server_list",
			Description: "列出所有可用服务器及其连接状态。\n\n返回两类服务器：\n- connected: 已建立连接，可直接使用\n- available: 已保存凭证，可以尝试连接（可能失败）",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name: "server_connect",
			Description: "连接到指定服务器。\n\n使用已保存的凭证进行连接。如果通过跳板机，会自动处理。",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"server": map[string]interface{}{
						"type":        "string",
						"description": "服务器名称",
					},
				},
				"required": []string{"server"},
			},
		},
		{
			Name: "server_disconnect",
			Description: "断开指定服务器的连接。",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"server": map[string]interface{}{
						"type":        "string",
						"description": "服务器名称",
					},
				},
				"required": []string{"server"},
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
			Name:        "session_start",
			Description: "开始一个新的排查会话。\n\n所有后续操作都会关联到这个会话，用于审计录制和知识归档。",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"problem": map[string]interface{}{
						"type":        "string",
						"description": "问题描述（用于标识和归档）",
					},
					"servers": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "可能涉及的服务器列表（可选）",
					},
				},
				"required": []string{"problem"},
			},
		},
		{
			Name:        "session_status",
			Description: "查看当前排查会话的状态。",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "session_end",
			Description: "结束当前排查会话。\n\n会断开所有连接，生成排查报告，并将经验归档到知识库。",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"summary": map[string]interface{}{
						"type":        "string",
						"description": "排查总结（问题原因、解决方案）",
					},
					"findings": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "关键发现列表",
					},
					"root_cause": map[string]interface{}{
						"type":        "string",
						"description": "根本原因（如果已确定）",
					},
				},
				"required": []string{"summary"},
			},
		},
		{
			Name: "get_hints",
			Description: `基于知识库获取排查思路提示。

输入问题描述，返回 Markdown 格式的定位指导文档。

适用场景：
- 开始排查前获取思路
- 遇到问题时寻求指导

注意：需要 OpsCopilot 配置了 LLM API（config.json 中的 llm.APIKey）`,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"problem": map[string]interface{}{
						"type":        "string",
						"description": "问题描述或症状",
					},
					"context": map[string]interface{}{
						"type":        "string",
						"description": "额外上下文（如服务器类型、应用名称、错误信息）",
					},
				},
				"required": []string{"problem"},
			},
		},
		{
			Name: "file_download",
			Description: `从远程服务器通过 SFTP 下载文件到本地。

适用于排障场景中拉取配置文件、日志、数据文件到本地分析。
文件访问受路径级访问控制策略限制。

使用场景：
- 下载远程日志文件到本地分析
- 拉取配置文件查看或备份
- 获取运行数据用于诊断

注意：
- 文件大小有上限限制（默认 10MB）
- 远程路径和本地路径均受访问策略控制
- 需要服务器支持 SFTP 协议`,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
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
						"description": "本地保存路径（必须在允许的本地目录内）",
					},
					"max_bytes": map[string]interface{}{
						"type":        "integer",
						"description": "最大下载字节数（默认 10MB）",
						"default":     10485760,
					},
				},
				"required": []string{"server", "remote_path", "local_path"},
			},
		},
		{
			Name: "file_upload",
			Description: `从本地上传文件到远程服务器（通过 SFTP）。

适用于推送修改后的配置文件、补丁脚本等到远程服务器。
上传操作默认自动备份远程已有文件，防止误覆盖。

使用场景：
- 推送修改后的配置文件
- 上传补丁脚本到远程执行
- 部署小型文件

注意：
- 写入路径需要管理员显式配置（默认为空）
- 上传上限默认 5MB
- 默认自动备份被覆盖的远程文件`,
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"server": map[string]interface{}{
						"type":        "string",
						"description": "服务器名称（必须是已连接的服务器）",
					},
					"local_path": map[string]interface{}{
						"type":        "string",
						"description": "本地文件路径（必须在允许的本地目录内）",
					},
					"remote_path": map[string]interface{}{
						"type":        "string",
						"description": "远程目标路径",
					},
					"backup": map[string]interface{}{
						"type":        "boolean",
						"description": "覆盖前自动备份远程文件（默认 true）",
						"default":     true,
					},
					"mkdir": map[string]interface{}{
						"type":        "boolean",
						"description": "自动创建远程目标目录（默认 false）",
						"default":     false,
					},
				},
				"required": []string{"server", "local_path", "remote_path"},
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
	case "server_list":
		result, err = s.toolServerList()
	case "server_connect":
		result, err = s.toolServerConnect(arguments)
	case "server_disconnect":
		result, err = s.toolServerDisconnect(arguments)
	case "ssh_exec":
		result, err = s.toolSSHExec(arguments)
	case "session_start":
		result, err = s.toolSessionStart(arguments)
	case "session_status":
		result, err = s.toolSessionStatus()
	case "session_end":
		result, err = s.toolSessionEnd(arguments)
	case "get_hints":
		result, err = s.toolGetHints(arguments)
	case "file_download":
		result, err = s.toolFileDownload(arguments)
	case "file_upload":
		result, err = s.toolFileUpload(arguments)
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
