package opssidecar

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"opscopilot/pkg/config"
	"opscopilot/pkg/connectionstore"
	"opscopilot/pkg/core/ops"
	"opscopilot/pkg/core/security"
	"opscopilot/pkg/remote"
)

// Service 常驻运维服务。
//
// 连接树（connectionstore.Store）与执行内核（ops.Manager）是两个实例、指向同一份
// sessions.json：树编辑走 Store 并落盘，exec 走 Manager。Manager 构造时快照了连接
// 文件，因此树变更后标记 dirty，exec 前按 dirty/mtime 重建，既保证 exec 读到最新
// 配置，又避免编辑操作反复打断连接池；mtime 检查同时兜住 OpsCopilot GUI 侧的
// 外部编辑（双开场景）。
type Service struct {
	dataDir string

	store     *connectionstore.Store
	whitelist *security.WhitelistManager

	ops                *ops.Manager
	opsSessionsModTime time.Time
	opsDirty           bool
	execTimeout        time.Duration

	mu sync.Mutex // 串行 dispatch 下的双保险，保护重建/查询与 Close 交错
}

// NewService 构造服务并加载全部配置。dataDir 与 OpsCopilot 桌面模式的数据目录
// 约定一致：sessions.json、command_whitelist.json、file_access.json、config.json
// 都在该目录下。
func NewService(dataDir string) (*Service, error) {
	s := &Service{dataDir: dataDir}

	s.store = connectionstore.NewStoreWithPath(filepath.Join(dataDir, "sessions.json"))
	// 桌面模式写的是 OpsCopilot GUI 自己的 sessions.json：插件界面保存连接时
	// 可能不回传密码，必须保留已存敏感字段，否则整体替换会清空凭据。
	s.store.PreserveCredentials = true
	if err := s.store.Load(); err != nil {
		return nil, fmt.Errorf("读取连接配置失败: %w", err)
	}

	whitelist, err := loadWhitelist(filepath.Join(dataDir, "command_whitelist.json"))
	if err != nil {
		return nil, err
	}
	s.whitelist = whitelist
	s.execTimeout = loadExecTimeout(dataDir)

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.rebuildOpsLocked(); err != nil {
		return nil, err
	}
	return s, nil
}

// loadWhitelist 加载命令白名单；配置缺失或损坏时回退到内置默认策略，
// 与 ops.NewManager 的兜底语义一致。
func loadWhitelist(path string) (*security.WhitelistManager, error) {
	mgr, err := security.NewWhitelistManager(path)
	if err == nil {
		return mgr, nil
	}
	mgr, err = security.NewWhitelistManager("")
	if err != nil {
		return nil, fmt.Errorf("初始化命令白名单失败: %w", err)
	}
	_ = mgr.UpdateConfig(security.DefaultWhitelistConfig())
	return mgr, nil
}

// loadExecTimeout 读取 config.json 的 cli.exec_timeout_sec，缺省用包默认值。
func loadExecTimeout(dataDir string) time.Duration {
	data, err := os.ReadFile(filepath.Join(dataDir, "config.json"))
	if err == nil {
		var cfg struct {
			CLI config.CLIConfig `json:"cli"`
		}
		if json.Unmarshal(data, &cfg) == nil && cfg.CLI.ExecTimeoutSec > 0 {
			return time.Duration(cfg.CLI.ExecTimeoutSec) * time.Second
		}
	}
	return time.Duration(config.DefaultCLIExecTimeoutSec) * time.Second
}

// rebuildOpsLocked 重建执行内核。串行模型下仅在持锁时调用。
func (s *Service) rebuildOpsLocked() error {
	mgr, err := ops.NewManager(&ops.Config{
		SessionsFile:   filepath.Join(s.dataDir, "sessions.json"),
		WhitelistPath:  filepath.Join(s.dataDir, "command_whitelist.json"),
		FilePath:       filepath.Join(s.dataDir, "file_access.json"),
		MaxTotalBytes:  10240,
		MaxLineLength:  500,
		HeadLines:      5,
		IdleTimeoutMin: 30,
	})
	if err != nil {
		return err
	}
	if s.ops != nil {
		s.ops.Shutdown()
	}
	s.ops = mgr
	s.opsDirty = false
	s.opsSessionsModTime = sessionsModTime(s.dataDir)
	return nil
}

func sessionsModTime(dataDir string) time.Time {
	if fi, err := os.Stat(filepath.Join(dataDir, "sessions.json")); err == nil {
		return fi.ModTime()
	}
	return time.Time{}
}

// ensureOpsFreshLocked 保证 exec 前内核持有最新连接配置。
func (s *Service) ensureOpsFreshLocked() error {
	if !s.opsDirty && s.opsSessionsModTime.Equal(sessionsModTime(s.dataDir)) {
		return nil
	}
	return s.rebuildOpsLocked()
}

// Close 关闭服务并断开全部远程连接。
func (s *Service) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ops != nil {
		s.ops.Shutdown()
		s.ops = nil
	}
}

// handle 分发单个请求。返回 exit=true 表示客户端请求退出，主循环应优雅结束。
func (s *Service) handle(req Request) (resp Response, exit bool) {
	defer func() {
		if r := recover(); r != nil {
			resp = errorResponse(req.ID, CodeInternal, fmt.Sprintf("内部错误: %v", r), "")
			exit = false
		}
	}()

	switch req.Method {
	case MethodShutdown:
		return okResponse(req.ID, nil), true
	case MethodList:
		return s.handleList(req)
	case MethodCreateFolder:
		return s.handleCreateFolder(req)
	case MethodCreateConnection:
		return s.handleCreateConnection(req)
	case MethodUpdateConnection:
		return s.handleUpdateConnection(req)
	case MethodRenameNode:
		return s.handleRenameNode(req)
	case MethodMoveNode:
		return s.handleMoveNode(req)
	case MethodDeleteNode:
		return s.handleDeleteNode(req)
	case MethodExec:
		return s.handleExec(req)
	default:
		return errorResponse(req.ID, CodeMethodNotFound, "未知方法: "+req.Method, ""), false
	}
}

func (s *Service) handleList(req Request) (Response, bool) {
	s.mu.Lock()
	nodes := s.store.Snapshot()
	s.mu.Unlock()
	// Node 的 JSON tag 与磁盘 sessions.json 一致（type 取值 "folder"/"session"），
	// 前端类型按同名同值定义，无需归一化。
	return okResponse(req.ID, map[string]any{"nodes": nodes}), false
}

func (s *Service) handleCreateFolder(req Request) (Response, bool) {
	var params struct {
		Name     string `json:"name"`
		ParentID string `json:"parentId"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.Name == "" {
		return invalidParams(req.ID, "name 必填"), false
	}
	node, err := s.store.CreateFolder(params.Name, params.ParentID)
	if err != nil {
		return errorResponse(req.ID, CodeInternal, err.Error(), ""), false
	}
	s.markDirty()
	return okResponse(req.ID, node), false
}

func (s *Service) handleCreateConnection(req Request) (Response, bool) {
	var params struct {
		Config   remote.ConnectConfig `json:"config"`
		ParentID string               `json:"parentId"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return invalidParams(req.ID, "config 解析失败: "+err.Error()), false
	}
	if params.Config.Host == "" || params.Config.User == "" {
		return invalidParams(req.ID, "config.host 与 config.user 必填"), false
	}
	node, err := s.store.CreateConnection(params.Config, params.ParentID)
	if err != nil {
		return errorResponse(req.ID, CodeInternal, err.Error(), ""), false
	}
	s.markDirty()
	return okResponse(req.ID, node), false
}

func (s *Service) handleUpdateConnection(req Request) (Response, bool) {
	var params struct {
		ID     string               `json:"id"`
		Config remote.ConnectConfig `json:"config"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.ID == "" {
		return invalidParams(req.ID, "id 与 config 必填"), false
	}
	if err := s.store.UpdateConnection(params.ID, params.Config); err != nil {
		return errorResponse(req.ID, CodeInternal, err.Error(), ""), false
	}
	s.markDirty()
	return okResponse(req.ID, nil), false
}

func (s *Service) handleRenameNode(req Request) (Response, bool) {
	var params struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.ID == "" || params.Name == "" {
		return invalidParams(req.ID, "id 与 name 必填"), false
	}
	if err := s.store.RenameNode(params.ID, params.Name); err != nil {
		return errorResponse(req.ID, CodeInternal, err.Error(), ""), false
	}
	s.markDirty()
	return okResponse(req.ID, nil), false
}

func (s *Service) handleMoveNode(req Request) (Response, bool) {
	var params struct {
		ID       string `json:"id"`
		ParentID string `json:"parentId"`
		Index    int    `json:"index"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.ID == "" {
		return invalidParams(req.ID, "id 必填"), false
	}
	if err := s.store.MoveNode(params.ID, params.ParentID, params.Index); err != nil {
		return errorResponse(req.ID, CodeInternal, err.Error(), ""), false
	}
	s.markDirty()
	return okResponse(req.ID, nil), false
}

func (s *Service) handleDeleteNode(req Request) (Response, bool) {
	var params struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.ID == "" {
		return invalidParams(req.ID, "id 必填"), false
	}
	if err := s.store.DeleteNode(params.ID); err != nil {
		return errorResponse(req.ID, CodeInternal, err.Error(), ""), false
	}
	s.markDirty()
	return okResponse(req.ID, nil), false
}

func (s *Service) handleExec(req Request) (Response, bool) {
	var params struct {
		ServerID      string `json:"serverId"`
		Command       string `json:"command"`
		Intent        string `json:"intent"`
		MaxLineLength int    `json:"maxLineLength"`
		TimeoutSec    int    `json:"timeoutSec"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return invalidParams(req.ID, "params 解析失败: "+err.Error()), false
	}
	if params.ServerID == "" || params.Command == "" {
		return invalidParams(req.ID, "serverId 与 command 必填"), false
	}
	if params.TimeoutSec < 0 {
		return invalidParams(req.ID, "timeoutSec 不能为负"), false
	}

	s.mu.Lock()
	if err := s.ensureOpsFreshLocked(); err != nil {
		s.mu.Unlock()
		return errorResponse(req.ID, CodeInternal, err.Error(), ""), false
	}
	node := findNode(s.store.Snapshot(), params.ServerID)
	whitelist := s.whitelist
	mgr := s.ops
	s.mu.Unlock()

	if node == nil || node.Type != connectionstore.KindConnection || node.Config == nil {
		return errorResponse(req.ID, CodeNotFound, "服务器未找到", "请先在连接面板中添加该服务器"), false
	}
	host := node.Config.Host

	// 白名单预检：执行内核内置同一道闸门（调用方绕不过），这里先挡一层，
	// 为前端提供结构化的拒绝码与原因。
	if check := whitelist.Check(params.Command, host); !check.Allowed {
		return errorResponse(req.ID, CodeWhitelistDeny, check.Reason,
			"可在 OpsCopilot 白名单配置中为该 IP 段放行对应命令"), false
	}

	timeout := s.execTimeout
	if params.TimeoutSec > 0 {
		timeout = time.Duration(params.TimeoutSec) * time.Second
	}
	maxLineLength := params.MaxLineLength
	if maxLineLength <= 0 {
		maxLineLength = 500
	}

	result, err := mgr.Exec(context.Background(), host, params.Command, ops.ExecOptions{
		MaxLineLength: maxLineLength,
		Timeout:       timeout,
		Intent:        params.Intent,
	})
	if err != nil {
		return errorResponse(req.ID, classifyExecError(err.Error()), err.Error(), ""), false
	}
	return okResponse(req.ID, result), false
}

// markDirty 连接树变更后标记内核待重建。
func (s *Service) markDirty() {
	s.mu.Lock()
	s.opsDirty = true
	s.mu.Unlock()
}

// findNode 按 ID 深度查找树节点。
func findNode(nodes []*connectionstore.Node, id string) *connectionstore.Node {
	for _, node := range nodes {
		if node.ID == id {
			return node
		}
		if node.Type == connectionstore.KindFolder {
			if found := findNode(node.Children, id); found != nil {
				return found
			}
		}
	}
	return nil
}

// classifyExecError 把执行内核的字符串错误映射为结构化错误码。
// 内核错误未定义哨兵值，只能按消息前缀归类；未命中一律 exec_failed。
func classifyExecError(message string) string {
	switch {
	case strings.Contains(message, "白名单"):
		return CodeWhitelistDeny
	case strings.Contains(message, "密码未找到"):
		return CodeNoPassword
	case strings.HasPrefix(message, "连接失败"):
		return CodeConnectFailed
	case strings.Contains(message, "未找到"):
		return CodeNotFound
	default:
		return CodeExecFailed
	}
}

// Run 启动 stdio 主循环：每行一个请求、串行处理；数据目录由 OPSCOPILOT_DATA_DIR
// 注入，缺省退回 exe 同目录（与 OpsCopilot 桌面 CLI 的路径语义一致）。
func Run() int {
	svc, err := NewService(resolveDataDir())
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ops-sidecar] 初始化失败: %v\n", err)
		return 1
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	writer := bufio.NewWriter(os.Stdout)
	encoder := json.NewEncoder(writer)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			_ = encoder.Encode(errorResponse("", CodeInvalidParams, "请求不是合法 JSON: "+err.Error(), ""))
			_ = writer.Flush()
			continue
		}
		resp, exit := svc.handle(req)
		if err := encoder.Encode(resp); err != nil {
			fmt.Fprintf(os.Stderr, "[ops-sidecar] 写响应失败: %v\n", err)
			return 1
		}
		if err := writer.Flush(); err != nil {
			fmt.Fprintf(os.Stderr, "[ops-sidecar] 刷新输出失败: %v\n", err)
			return 1
		}
		if exit {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "[ops-sidecar] 读取请求失败: %v\n", err)
	}
	svc.Close()
	return 0
}

func resolveDataDir() string {
	if dir := os.Getenv("OPSCOPILOT_DATA_DIR"); dir != "" {
		return dir
	}
	if execPath, err := os.Executable(); err == nil {
		return filepath.Dir(execPath)
	}
	return "."
}

func okResponse(id string, data any) Response {
	return Response{ID: id, OK: true, Data: data}
}

func errorResponse(id, code, message, hint string) Response {
	return Response{ID: id, OK: false, Error: &ErrorWire{Code: code, Message: message, Hint: hint}}
}

func invalidParams(id, message string) Response {
	return errorResponse(id, CodeInvalidParams, message, "")
}
