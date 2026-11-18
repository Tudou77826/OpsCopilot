package opssidecar

// MCP 桥（service 侧）：随机回环端口监听，接收 --mcp 代理转发的单条 MCP
// 消息（{token,message} 信封，与 OpsCopilot agenttransport/mcp 的 StdioBridge
// 同形态），执行后回 {message|error}。
//
// token 支持组合形态 "hint.baseToken"：Desktop 为每个 ACP 会话生成 hint 经
// env 注入代理；gateway 校验后缀等于本实例 token、解出 hint 随 handoff 事件
// 回传——project 侧据此把审批卡路由进发起会话，且不同会话的 token 天然
// 互不可用（会话隔离）。
//
// 工具面（7 个）：connections_list / remote_exec / remote_file_upload /
// remote_file_download / terminal_list / terminal_observe / terminal_write。
// 执行复用 service 既有路径：exec 走白名单闸门，Agent 传输走内核文件闸门；
// 每次调用追加审计（命令内容可追溯）。

import (
	"bufio"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"opscopilot/pkg/connectionstore"
	"opscopilot/pkg/core/ops"
)

const (
	dialTimeout   = 2 * time.Second
	mcpMaxMessage = 32768
	auditFileName = "ops-audit.jsonl"
)

// auditRecord 审计条目（append-only JSONL；Command 记录命令/路径/写入内容）。
type auditRecord struct {
	Time    int64  `json:"ts"`
	Tool    string `json:"tool"`
	Server  string `json:"server,omitempty"`
	Command string `json:"command,omitempty"`
	Detail  string `json:"detail,omitempty"`
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
}

type mcpGateway struct {
	svc   *Service
	token string
}

// splitSessionToken 拆组合 token：返回 (hint, 是否匹配)。
func splitSessionToken(candidate, base string) (string, bool) {
	if candidate == base {
		return "", true
	}
	if idx := strings.LastIndex(candidate, "."); idx > 0 && candidate[idx+1:] == base {
		return candidate[:idx], true
	}
	return "", false
}

type bridgeRequest struct {
	Token   string          `json:"token"`
	Message json.RawMessage `json:"message"`
}

type bridgeReply struct {
	Message json.RawMessage `json:"message,omitempty"`
	Error   string          `json:"error,omitempty"`
}

func (g *mcpGateway) serveConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(dialTimeout))
	reader := bufio.NewReader(io.LimitReader(conn, mcpMaxMessage+1024))
	var request bridgeRequest
	if json.NewDecoder(reader).Decode(&request) != nil || len(request.Message) > mcpMaxMessage {
		return
	}
	hint, matched := splitSessionToken(request.Token, g.token)
	if !matched && subtle.ConstantTimeCompare([]byte(request.Token), []byte(g.token)) != 1 {
		return
	}
	_ = conn.SetReadDeadline(time.Time{})
	reply := bridgeReply{}
	message, err := g.handleMessage(hint, request.Message)
	switch {
	case err == errMCPNotification:
		return
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
		"jsonrpc": "2.0", "id": id,
		"error": map[string]any{"code": code, "message": message},
	})
	return raw
}

// serveHTTP MCP Streamable HTTP 端点（POST /mcp）：Authorization 头携带
// 组合 token（"hint.baseToken"，与 stdio 代理同一语义），body 为单条 JSON-RPC
// 消息。OpenCode 等 ACP 宿主只声明 mcpCapabilities.http——HTTP 直连省去
// stdio 代理中转，Desktop 经 mcpServers 直接下发本端点。
func (g *mcpGateway) handleHTTP(w http.ResponseWriter, r *http.Request) {
	slog.Info("[ops-mcp-http] 请求", "method", r.Method, "path", r.URL.Path, "auth", r.Header.Get("Authorization") != "")
	if r.Method != http.MethodPost {
		http.Error(w, "仅支持 POST", http.StatusMethodNotAllowed)
		return
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	hint, matched := splitSessionToken(token, g.token)
	if !matched && subtle.ConstantTimeCompare([]byte(token), []byte(g.token)) != 1 {
		http.Error(w, "未授权", http.StatusUnauthorized)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, mcpMaxMessage+1024))
	if err != nil || len(raw) > mcpMaxMessage {
		http.Error(w, "消息超长", http.StatusRequestEntityTooLarge)
		return
	}
	reply, err := g.handleMessage(hint, raw)
	if err != nil {
		if err == errMCPNotification {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		http.Error(w, "消息解析失败", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(reply)
}

// oneConnListener 把单个已建立连接包装成 http.Server 可 Serve 的监听器
// （TCP 桥与 HTTP 端点共用同一端口的嗅探分派用）。
type oneConnListener struct {
	conn net.Conn
	done chan struct{}
}

func (l *oneConnListener) Accept() (net.Conn, error) {
	select {
	case <-l.done:
		return nil, io.EOF
	default:
		close(l.done)
		return l.conn, nil
	}
}
func (l *oneConnListener) Close() error   { return nil }
func (l *oneConnListener) Addr() net.Addr { return l.conn.LocalAddr() }

func (g *mcpGateway) handleMessage(hint string, raw []byte) (json.RawMessage, error) {
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
	case "ping":
		return reply(map[string]any{}), nil
	case "notifications/initialized":
		return nil, errMCPNotification
	case "tools/list":
		return reply(map[string]any{"tools": g.toolDefinitions()}), nil
	case "tools/call":
		return g.handleToolCall(hint, req), nil
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
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	}
	str := func(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
	num := func(desc string) map[string]any { return map[string]any{"type": "number", "description": desc} }
	return []toolDefinition{
		{Name: "connections_list", Description: "列出可用的远程服务器（name/host）。先调用它选择目标服务器。", InputSchema: object(map[string]any{})},
		{Name: "remote_exec", Description: "在远程服务器执行 shell 命令（受白名单闸门约束）。", InputSchema: object(map[string]any{
			"server": str("服务器 name"), "command": str("要执行的命令"),
			"intent": str("执行目的"), "timeout_sec": num("超时秒数"),
		}, "server", "command")},
		{Name: "remote_file_upload", Description: "上传本地文件到远程服务器（受文件访问闸门约束）。", InputSchema: object(map[string]any{
			"server": str("服务器 name"), "local_path": str("本地文件绝对路径"), "remote_path": str("远端目标绝对路径"),
		}, "server", "local_path", "remote_path")},
		{Name: "remote_file_download", Description: "从远程服务器下载文件到本地（受文件访问闸门与大小上限约束）。", InputSchema: object(map[string]any{
			"server": str("服务器 name"), "remote_path": str("远端文件绝对路径"), "local_path": str("本地保存绝对路径"),
			"max_bytes": num("最大字节数，缺省 10MB"),
		}, "server", "remote_path", "local_path")},
		{Name: "terminal_list", Description: "列出当前打开的远程终端会话。", InputSchema: object(map[string]any{})},
		{Name: "terminal_observe", Description: "读取远程终端屏幕内容（增量，after 用上次返回的 sequence）。只读不打扰。", InputSchema: object(map[string]any{
			"session_id": str("终端会话 ID"), "after": num("增量水位"),
		}, "session_id")},
		{Name: "terminal_write", Description: "向远程终端写入按键（受审批档位约束：manual 需用户逐次确认，全程审计）。", InputSchema: object(map[string]any{
			"session_id": str("终端会话 ID"), "data": str("要写入的字符（含回车 \r）"),
		}, "session_id", "data")},
	}
}

func (g *mcpGateway) handleToolCall(hint string, req mcpRequest) json.RawMessage {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if json.Unmarshal(req.Params, &params) != nil || params.Name == "" {
		return mcpError(req.ID, -32602, "invalid tool request")
	}
	started := time.Now()
	result, toolErr := g.dispatchTool(hint, params.Name, params.Arguments)
	if toolErr != nil {
		g.svc.audit(auditRecord{Time: time.Now().Unix(), Tool: params.Name, OK: false, Error: toolErr.Error()})
	} else {
		g.svc.audit(auditRecord{Time: time.Now().Unix(), Tool: params.Name, OK: true, Detail: fmt.Sprintf("%.1fs", time.Since(started).Seconds())})
	}
	g.svc.touch()
	content := map[string]any{}
	if toolErr != nil {
		content = map[string]any{
			"content": []map[string]any{{"type": "text", "text": "工具执行失败: " + toolErr.Error()}},
			"isError": true,
		}
	} else {
		text, _ := json.Marshal(result)
		content = map[string]any{"content": []map[string]any{{"type": "text", "text": string(text)}}}
	}
	out, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": content})
	return out
}

func (g *mcpGateway) dispatchTool(hint, name string, args json.RawMessage) (any, error) {
	var input map[string]any
	if len(args) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return nil, errors.New("参数不是合法 JSON 对象")
		}
	}
	argStr := func(key string) string {
		value, _ := input[key].(string)
		return strings.TrimSpace(value)
	}
	argNum := func(key string) int {
		value, _ := input[key].(float64)
		return int(value)
	}
	switch name {
	case "connections_list":
		return g.svc.listServersForAgent()
	case "remote_exec":
		server, command := argStr("server"), argStr("command")
		if server == "" || command == "" {
			return nil, errors.New("server 与 command 必填")
		}
		node := g.svc.findServerByName(server)
		if node == nil {
			return nil, fmt.Errorf("服务器 %q 不存在，请先 connections_list 查看", server)
		}
		return g.svc.execByNode(node, ExecParams{
			ServerID: node.ID, Command: command,
			Intent: argStr("intent"), TimeoutSec: argNum("timeout_sec"),
		})
	case "remote_file_upload":
		return g.svc.transferForAgent("upload", argStr("server"), argStr("local_path"), argStr("remote_path"), 0)
	case "remote_file_download":
		return g.svc.transferForAgent("download", argStr("server"), argStr("local_path"), argStr("remote_path"), argNum("max_bytes"))
	case "terminal_list":
		return g.svc.terminals.listForAgent()
	case "terminal_observe":
		return g.svc.terminals.observeForAgent(argStr("session_id"), argNum("after"))
	case "terminal_write":
		return g.svc.terminalWriteForAgent(hint, argStr("session_id"), argStr("data"))
	default:
		return nil, fmt.Errorf("未知工具: %s", name)
	}
}

func (s *Service) listServersForAgent() ([]map[string]any, error) {
	s.mu.Lock()
	nodes := s.store.Snapshot()
	s.mu.Unlock()
	servers := []map[string]any{}
	var walk func(list []*connectionstore.Node)
	walk = func(list []*connectionstore.Node) {
		for _, node := range list {
			if node.Type == connectionstore.KindConnection && node.Config != nil {
				servers = append(servers, map[string]any{"name": node.Name, "host": node.Config.Host, "protocol": node.Config.Protocol})
			}
			if node.Type == connectionstore.KindFolder {
				walk(node.Children)
			}
		}
	}
	walk(nodes)
	return servers, nil
}

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

func (s *Service) transferForAgent(kind, server, localPath, remotePath string, maxBytes int) (map[string]any, error) {
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
	s.audit(auditRecord{Time: time.Now().Unix(), Tool: "remote_file_" + kind, Server: server, Command: remotePath, Detail: localPath, OK: true})
	if kind == "upload" {
		result, err := mgr.Upload(host, remotePath, ops.UploadOptions{LocalPath: localPath, Backup: true, Mkdir: true, Context: context.Background()})
		if err != nil {
			return nil, err
		}
		return result.Meta, nil
	}
	result, err := mgr.Download(host, remotePath, ops.DownloadOptions{LocalPath: localPath, MaxBytes: maxBytes, Context: context.Background()})
	if err != nil {
		return nil, err
	}
	return result.Meta, nil
}
