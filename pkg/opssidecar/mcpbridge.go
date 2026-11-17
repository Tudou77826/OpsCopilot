package opssidecar

// MCP 桥（service 侧）：回环 TCP 监听，接收 stdio 代理（--mcp 模式）转发的
// 单条 MCP 消息（{token,message} 信封，一次性连接一问一答），经统一工具
// 分发执行后回 {message|error}。协议形态与 OpsCopilot pkg/agenttransport/mcp
// 的 StdioBridge 一致：仅 127.0.0.1、token 恒定时间比较、单条消息 32KB 上限。
//
// 工具面（对接 ACP 会话的 Agent）：
//   connections_list / remote_exec / remote_file_upload / remote_file_download
//   knowledge_search / knowledge_read / terminal_list / terminal_observe /
//   terminal_write（写经 Desktop 审批或 auto 放行，全程审计）。
// 执行一律复用 service 既有路径：exec 走白名单闸门，传输走内核文件访问
// 闸门，调用方绕不过。每次工具调用追加审计 JSONL。

import (
	"bufio"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"opscopilot/pkg/connectionstore"
	"opscopilot/pkg/core/ops"
	"opscopilot/pkg/knowledge"
)

const (
	dialTimeout    = 2 * time.Second
	mcpMaxMessage  = 32768
	auditFileName  = "ops-audit.jsonl"
	knowledgeMaxKB = 512
)

// auditRecord 一次工具调用的审计条目（append-only JSONL，不记密钥类参数——
// 工具面本身不接收凭据）。
type auditRecord struct {
	Time   int64  `json:"ts"`
	Tool   string `json:"tool"`
	Server string `json:"server,omitempty"`
	Detail string `json:"detail,omitempty"`
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
}

func (s *Service) audit(rec auditRecord) {
	s.auditMu.Lock()
	defer s.auditMu.Unlock()
	file, err := os.OpenFile(filepath.Join(s.dataDir, auditFileName), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	raw, err := json.Marshal(rec)
	if err != nil {
		return
	}
	_, _ = file.Write(append(raw, '\n'))
}

// approvalMode 审批档位（manual=逐次人工确认 / auto=放行仅审计），
// 持久化在 config.json 的 remoteOps 小节，由 Desktop 设置项驱动。
func (s *Service) approvalMode() string {
	raw, err := os.ReadFile(filepath.Join(s.dataDir, "config.json"))
	if err != nil {
		return "manual"
	}
	var cfg struct {
		RemoteOps struct {
			ApprovalMode string `json:"approvalMode"`
		} `json:"remoteOps"`
	}
	if json.Unmarshal(raw, &cfg) != nil {
		return "manual"
	}
	if cfg.RemoteOps.ApprovalMode == "auto" {
		return "auto"
	}
	return "manual"
}

// setApprovalMode 更新审批档位：读改写整份 config.json，保留其余小节。
func (s *Service) setApprovalMode(mode string) error {
	if mode != "manual" && mode != "auto" {
		return fmt.Errorf("审批档位只支持 manual / auto")
	}
	path := filepath.Join(s.dataDir, "config.json")
	root := map[string]json.RawMessage{}
	if raw, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(raw, &root); err != nil {
			return fmt.Errorf("config.json 损坏: %w", err)
		}
	}
	section := map[string]any{}
	if existing, ok := root["remoteOps"]; ok {
		_ = json.Unmarshal(existing, &section)
	}
	section["approvalMode"] = mode
	updated, err := json.Marshal(section)
	if err != nil {
		return err
	}
	root["remoteOps"] = updated
	raw, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

// resolveKnowledgeDir 知识库目录：config.json docs.dir → dataDir/knowledge
// → dataDir/docs（与 OpsCopilot resolveKnowledgeBase 的约定对齐的子集）。
func (s *Service) resolveKnowledgeDir() string {
	if raw, err := os.ReadFile(filepath.Join(s.dataDir, "config.json")); err == nil {
		var cfg struct {
			Docs struct {
				Dir string `json:"dir"`
			} `json:"docs"`
		}
		if json.Unmarshal(raw, &cfg) == nil && cfg.Docs.Dir != "" {
			if _, err := os.Stat(cfg.Docs.Dir); err == nil {
				return cfg.Docs.Dir
			}
		}
	}
	for _, name := range []string{"knowledge", "docs"} {
		candidate := filepath.Join(s.dataDir, name)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return ""
}

// mcpGateway MCP 消息入口：信封校验 + JSON-RPC 分发 + 工具执行。
type mcpGateway struct {
	svc   *Service
	token string
}

// bridgeRequest 单条转发信封（代理写入）。
type bridgeRequest struct {
	Token   string          `json:"token"`
	Message json.RawMessage `json:"message"`
}

// bridgeReply 单条应答信封（回给代理）。
type bridgeReply struct {
	Message json.RawMessage `json:"message,omitempty"`
	Error   string          `json:"error,omitempty"`
}

func (g *mcpGateway) serveConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(dialTimeout))
	reader := bufio.NewReader(io.LimitReader(conn, mcpMaxMessage+1024))
	var request bridgeRequest
	if json.NewDecoder(reader).Decode(&request) != nil ||
		len(request.Message) > mcpMaxMessage ||
		subtle.ConstantTimeCompare([]byte(request.Token), []byte(g.token)) != 1 {
		return
	}
	// 读超时只覆盖首帧；工具执行（含审批等待）不受它约束
	_ = conn.SetReadDeadline(time.Time{})
	reply := bridgeReply{}
	message, err := g.handleMessage(request.Message)
	switch {
	case err == errMCPNotification:
		return // 通知无应答
	case err != nil:
		reply.Error = err.Error()
	default:
		reply.Message = message
	}
	_ = conn.SetWriteDeadline(time.Now().Add(dialTimeout))
	_ = json.NewEncoder(conn).Encode(reply)
}

var errMCPNotification = errors.New("mcp notification")

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

func mcpError(id json.RawMessage, code int, message string) json.RawMessage {
	if len(id) == 0 {
		return nil
	}
	raw, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error":   map[string]any{"code": code, "message": message},
	})
	return raw
}

func (g *mcpGateway) handleMessage(raw []byte) (json.RawMessage, error) {
	var req mcpRequest
	if err := json.Unmarshal(raw, &req); err != nil || len(req.ID) == 0 {
		return nil, errMCPNotification
	}
	reply := func(result any) json.RawMessage {
		out, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
		return out
	}
	switch req.Method {
	case "initialize":
		return reply(map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "ops-sidecar", "version": "1"},
		}), nil
	case "notifications/initialized", "ping":
		if req.Method == "ping" {
			return reply(map[string]any{}), nil
		}
		return nil, errMCPNotification
	case "tools/list":
		return reply(map[string]any{"tools": g.toolDefinitions()}), nil
	case "tools/call":
		return g.handleToolCall(req), nil
	default:
		return mcpError(req.ID, -32601, "未知方法: "+req.Method), nil
	}
}

type toolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func (g *mcpGateway) toolDefinitions() []toolDefinition {
	object := func(properties map[string]any, required ...string) map[string]any {
		return map[string]any{
			"type": "object", "properties": properties,
			"required": required, "additionalProperties": false,
		}
	}
	str := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
	num := func(desc string) map[string]any { return map[string]any{"type": "number", "description": desc} }
	return []toolDefinition{
		{
			Name: "connections_list", Description: "列出可用的远程服务器（name/host）。先调用它选择目标服务器。",
			InputSchema: object(map[string]any{}),
		},
		{
			Name: "remote_exec", Description: "在远程服务器执行 shell 命令（受白名单闸门约束）。输出截断到有限行数。",
			InputSchema: object(map[string]any{
				"server":      str("服务器 name（来自 connections_list）"),
				"command":     str("要执行的命令"),
				"intent":      str("执行目的，一句话说明"),
				"timeout_sec": num("超时秒数，缺省用服务端默认"),
			}, "server", "command"),
		},
		{
			Name: "remote_file_upload", Description: "上传本地文件到远程服务器（受文件访问闸门约束）。",
			InputSchema: object(map[string]any{
				"server":      str("服务器 name"),
				"local_path":  str("本地文件绝对路径"),
				"remote_path": str("远端目标绝对路径"),
				"backup":      map[string]any{"type": "boolean", "description": "覆盖前备份远端文件"},
				"mkdir":       map[string]any{"type": "boolean", "description": "自动创建远端目录"},
			}, "server", "local_path", "remote_path"),
		},
		{
			Name: "remote_file_download", Description: "从远程服务器下载文件到本地（受文件访问闸门与大小上限约束）。",
			InputSchema: object(map[string]any{
				"server":      str("服务器 name"),
				"remote_path": str("远端文件绝对路径"),
				"local_path":  str("本地保存绝对路径"),
				"max_bytes":   num("最大字节数，缺省 10MB"),
			}, "server", "remote_path", "local_path"),
		},
		{
			Name: "knowledge_search", Description: "在运维知识库中检索历史故障结论与处置记录。",
			InputSchema: object(map[string]any{
				"query": str("检索关键词"),
				"top_n": num("返回条数，缺省 5"),
			}, "query"),
		},
		{
			Name: "knowledge_read", Description: "读取知识库文档内容（相对 knowledge_search 返回的路径）。",
			InputSchema: object(map[string]any{
				"path": str("知识库内相对路径"),
			}, "path"),
		},
		{
			Name: "terminal_list", Description: "列出当前打开的远程终端会话（接管目标）。",
			InputSchema: object(map[string]any{}),
		},
		{
			Name: "terminal_observe", Description: "读取远程终端屏幕内容（增量读取，after 用上次返回的 revision）。只读，不打扰现场。",
			InputSchema: object(map[string]any{
				"session_id": str("终端会话 ID（来自 terminal_list）"),
				"after":      num("增量水位：上次返回的 Sequence，首次传 0"),
			}, "session_id"),
		},
		{
			Name: "terminal_write", Description: "向远程终端写入按键（受审批档位约束：manual 需用户逐次确认，全程审计）。",
			InputSchema: object(map[string]any{
				"session_id": str("终端会话 ID"),
				"data":       str("要写入的字符（含回车 \\r）"),
			}, "session_id", "data"),
		},
	}
}

func (g *mcpGateway) handleToolCall(req mcpRequest) json.RawMessage {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if json.Unmarshal(req.Params, &params) != nil || params.Name == "" {
		return mcpError(req.ID, -32602, "invalid tool request")
	}
	started := time.Now()
	result, toolErr := g.dispatchTool(params.Name, params.Arguments)
	record := auditRecord{
		Time: time.Now().Unix(),
		Tool: params.Name,
		OK:   toolErr == nil,
	}
	if toolErr != nil {
		record.Error = toolErr.Error()
	} else if text, ok := result.(map[string]any); ok {
		record.Server, _ = text["server"].(string)
		record.Detail = fmt.Sprintf("%.1fs", time.Since(started).Seconds())
	}
	g.svc.audit(record)
	g.svc.touch()

	content := map[string]any{}
	if toolErr != nil {
		content = map[string]any{
			"content": []map[string]any{{"type": "text", "text": "工具执行失败: " + toolErr.Error()}},
			"isError": true,
		}
	} else {
		text, _ := json.Marshal(result)
		content = map[string]any{
			"content": []map[string]any{{"type": "text", "text": string(text)}},
		}
	}
	out, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": content})
	return out
}

// dispatchTool 工具名 → 服务操作。ctx 仅用于传输等待；所有闸门在内核侧。
func (g *mcpGateway) dispatchTool(name string, args json.RawMessage) (any, error) {
	var input map[string]any
	if len(args) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return nil, fmt.Errorf("参数不是合法 JSON 对象")
		}
	}
	argString := func(key string) string {
		value, _ := input[key].(string)
		return strings.TrimSpace(value)
	}
	argBool := func(key string) bool {
		value, _ := input[key].(bool)
		return value
	}
	argNumber := func(key string) int {
		value, _ := input[key].(float64)
		return int(value)
	}

	switch name {
	case "connections_list":
		return g.svc.listServersForAgent()
	case "remote_exec":
		server, command := argString("server"), argString("command")
		if server == "" || command == "" {
			return nil, errors.New("server 与 command 必填")
		}
		params := ExecParams{
			ServerID:   server,
			Command:    command,
			Intent:     argString("intent"),
			TimeoutSec: argNumber("timeout_sec"),
		}
		result, err := g.svc.execForAgent(params)
		if err != nil {
			return nil, err
		}
		return result, nil
	case "remote_file_upload":
		result, err := g.svc.transferForAgent("upload", argString("server"), argString("local_path"), argString("remote_path"), 0, argBool("backup"), argBool("mkdir"))
		return result, err
	case "remote_file_download":
		result, err := g.svc.transferForAgent("download", argString("server"), argString("local_path"), argString("remote_path"), argNumber("max_bytes"), false, false)
		return result, err
	case "knowledge_search":
		return g.svc.knowledgeSearch(argString("query"), argNumber("top_n"))
	case "knowledge_read":
		return g.svc.knowledgeRead(argString("path"))
	case "terminal_list":
		return g.svc.terminals.listForAgent()
	case "terminal_observe":
		return g.svc.terminals.observeForAgent(argString("session_id"), argNumber("after"))
	case "terminal_write":
		return g.svc.terminalWriteForAgent(argString("session_id"), argString("data"))
	default:
		return nil, fmt.Errorf("未知工具: %s", name)
	}
}

// ---- 工具实现（service 层） ----

// listServersForAgent 连接树的服务器清单（agent 按 name 选目标）。
func (s *Service) listServersForAgent() ([]map[string]any, error) {
	s.mu.Lock()
	nodes := s.store.Snapshot()
	s.mu.Unlock()
	servers := []map[string]any{}
	var walk func(list []*connectionstore.Node)
	walk = func(list []*connectionstore.Node) {
		for _, node := range list {
			if node.Type == connectionstore.KindConnection && node.Config != nil {
				servers = append(servers, map[string]any{
					"name": node.Name, "host": node.Config.Host, "protocol": node.Config.Protocol,
				})
			}
			if node.Type == connectionstore.KindFolder {
				walk(node.Children)
			}
		}
	}
	walk(nodes)
	return servers, nil
}

// findServerByName 按 name（或 host 兜底）解析服务器节点。
func (s *Service) findServerByName(name string) *connectionstore.Node {
	s.mu.Lock()
	nodes := s.store.Snapshot()
	s.mu.Unlock()
	var byName, byHost *connectionstore.Node
	var walk func(list []*connectionstore.Node)
	walk = func(list []*connectionstore.Node) {
		for _, node := range list {
			if node.Type == connectionstore.KindConnection && node.Config != nil {
				if node.Name == name && byName == nil {
					byName = node
				}
				if node.Config.Host == name && byHost == nil {
					byHost = node
				}
			}
			if node.Type == connectionstore.KindFolder {
				walk(node.Children)
			}
		}
	}
	walk(nodes)
	if byName != nil {
		return byName
	}
	return byHost
}

// execForAgent 按 name 解析 + 白名单预检 + 内核执行（复用 exec 语义）。
func (s *Service) execForAgent(params ExecParams) (*ops.ExecResult, error) {
	node := s.findServerByName(params.ServerID)
	if node == nil {
		return nil, fmt.Errorf("服务器 %q 不存在，请先 connections_list 查看可用服务器", params.ServerID)
	}
	return s.execByNode(node, params)
}

// transferForAgent 同步传输（agent 工具一问一答，无进度事件）。
func (s *Service) transferForAgent(kind, server, localPath, remotePath string, maxBytes int, backup, mkdir bool) (map[string]any, error) {
	if server == "" || localPath == "" || remotePath == "" {
		return nil, errors.New("server、local_path 与 remote_path 必填")
	}
	node := s.findServerByName(server)
	if node == nil {
		return nil, fmt.Errorf("服务器 %q 不存在", server)
	}
	mgr, host, err := s.resolveServer(node.ID)
	if err != nil {
		return nil, err
	}
	if kind == "upload" {
		result, err := mgr.Upload(host, remotePath, ops.UploadOptions{LocalPath: localPath, Backup: backup, Mkdir: mkdir})
		if err != nil {
			return nil, err
		}
		return result.Meta, nil
	}
	result, err := mgr.Download(host, remotePath, ops.DownloadOptions{LocalPath: localPath, MaxBytes: maxBytes})
	if err != nil {
		return nil, err
	}
	return result.Meta, nil
}

// knowledgeSearch 知识库检索（目录未配置时给出可操作的错误说明）。
func (s *Service) knowledgeSearch(query string, topN int) (any, error) {
	if query == "" {
		return nil, errors.New("query 必填")
	}
	dir := s.resolveKnowledgeDir()
	if dir == "" {
		return nil, errors.New("知识库目录不存在（config.json docs.dir 或数据目录 knowledge/）")
	}
	catalog, err := knowledge.ReadCatalog(dir)
	if err != nil {
		return nil, fmt.Errorf("读取知识目录失败: %w", err)
	}
	if topN <= 0 || topN > 20 {
		topN = 5
	}
	return knowledge.Search(dir, catalog, query, topN)
}

// knowledgeRead 读取知识库内文档（路径限定在库目录内，大小上限保护）。
func (s *Service) knowledgeRead(relPath string) (any, error) {
	if relPath == "" {
		return nil, errors.New("path 必填")
	}
	dir := s.resolveKnowledgeDir()
	if dir == "" {
		return nil, errors.New("知识库目录不存在")
	}
	content, err := knowledge.ReadFile(dir, relPath)
	if err != nil {
		return nil, err
	}
	if len(content) > knowledgeMaxKB*1024 {
		content = content[:knowledgeMaxKB*1024]
	}
	return map[string]any{"path": relPath, "content": string(content)}, nil
}

// whitelistCheckForAgent 结构化白名单拒绝说明（与面板 exec 的语义一致）。
func (s *Service) whitelistCheckForAgent(command, host string) error {
	s.mu.Lock()
	whitelist := s.whitelist
	s.mu.Unlock()
	if check := whitelist.Check(command, host); !check.Allowed {
		return fmt.Errorf("%s（可在 OpsCopilot 白名单配置中为该 IP 段放行）", check.Reason)
	}
	return nil
}
