package opssidecar

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"opscopilot/pkg/config"
	"opscopilot/pkg/connectionstore"
	"opscopilot/pkg/core/ops"
	"opscopilot/pkg/core/security"
	"opscopilot/pkg/remote"
	"opscopilot/pkg/terminalcontrol"
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

	// out 是 stdout 的唯一写入者：响应（串行路径）与事件（终端 pump、传输
	// 进度 goroutine）经它整行落盘。
	out       *lineEmitter
	terminals *terminalManager
	transfers *transferManager
	tcSvc     *terminalcontrol.Service

	// Agent 工具通道：MCP 桥监听与审计；lastTouch 供空闲退出判定。
	auditMu        sync.Mutex
	bridgeListener net.Listener
	lastTouch      atomic.Int64

	// 手工审批：同一时刻至多一条等待确认的终端写入。
	pendingMu     sync.Mutex
	pendingWrite  *pendingHandoff
	pendingNotify chan struct{}

	mu sync.Mutex // 串行 dispatch 下的双保险，保护重建/查询与 Close 交错
}

// pendingHandoff 一条等待用户确认的终端写入（manual 审批档位）。
type pendingHandoff struct {
	sessionID string
	data      string
	decided   chan bool
}

// ExecParams 执行参数（面板 exec 与 Agent 的 remote_exec 共用）。
type ExecParams struct {
	ServerID      string
	Command       string
	Intent        string
	MaxLineLength int
	TimeoutSec    int
}

// NewService 构造服务并加载全部配置。dataDir 与 OpsCopilot 桌面模式的数据目录
// 约定一致：sessions.json、command_whitelist.json、file_access.json、config.json
// 都在该目录下。out 为协议行输出目标（生产为 os.Stdout，测试可注入缓冲）。
func NewService(dataDir string, out io.Writer) (*Service, error) {
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
	s.out = newLineEmitter(out)
	s.tcSvc = terminalcontrol.NewService(func(a terminalcontrol.Audit) {
		s.audit(auditRecord{
			Time:   time.Now().Unix(),
			Tool:   "terminal_control",
			Server: a.SessionID,
			Detail: a.Action + "@" + a.Phase,
			OK:     true,
		})
	})
	s.terminals = newTerminalManager(s.emitEvent, s.tcSvc)
	s.transfers = newTransferManager(s.emitEvent)
	s.lastTouch.Store(time.Now().Unix())
	s.startBridge()
	return s, nil
}

// touch 记录一次活动（任何请求或工具调用），推迟空闲退出。
func (s *Service) touch() {
	s.lastTouch.Store(time.Now().Unix())
}

// idleExitAfter 无活动且无占用多久后退出（下次请求自动重启，实现零常驻）。
var idleExitAfter = 15 * time.Minute

// idleCheck 达到空闲条件返回 true：无近期活动，且没有活跃 SSH 连接、
// 终端会话与在途传输。
func (s *Service) idleCheck() bool {
	if time.Since(time.Unix(s.lastTouch.Load(), 0)) < idleExitAfter {
		return false
	}
	s.mu.Lock()
	mgr := s.ops
	s.mu.Unlock()
	if mgr != nil && mgr.ConnectionCount() > 0 {
		return false
	}
	if s.terminals != nil && s.terminals.activeCount() > 0 {
		return false
	}
	s.pendingMu.Lock()
	busy := s.pendingWrite != nil
	s.pendingMu.Unlock()
	if busy {
		return false
	}
	return true
}

// startBridge 启动 MCP 桥监听并落盘发现文件。端口被占（重复实例或冲突）
// 时降级：面板功能不受影响，仅 Agent 工具不可用。
func (s *Service) startBridge() {
	token, err := ensureBridgeToken(s.dataDir)
	if err != nil {
		slog.Warn("[ops-sidecar] MCP 桥 token 生成失败，Agent 工具不可用", "error", err)
		return
	}
	port := resolveBridgePort()
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		slog.Warn("[ops-sidecar] MCP 桥端口不可用，服务降级运行（面板可用）", "port", port, "error", err)
		return
	}
	info := BridgeInfo{Port: port, Token: token, PID: os.Getpid()}
	if err := writeBridgeInfo(s.dataDir, info); err != nil {
		slog.Warn("[ops-sidecar] 服务发现文件写入失败，Agent 工具不可用", "error", err)
		_ = listener.Close()
		return
	}
	s.bridgeListener = listener
	gateway := &mcpGateway{svc: s, token: token}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go gateway.serveConn(conn)
		}
	}()
	slog.Info("[ops-sidecar] MCP 桥已就绪", "port", port)
}

// emitEvent 供终端 pump 与传输 goroutine 下发异步事件。
func (s *Service) emitEvent(event string, params any) {
	s.out.line(Event{Event: event, Params: params})
}

// errServerNotFound 服务器节点解析失败的哨兵（节点缺失或非服务器节点）。
var errServerNotFound = errors.New("服务器未找到")

// resolveServer 按树节点 ID 解析出当前内核与主机地址。串行路径专用：
// 持锁保证内核新鲜度（树变更重建）与快照读取的原子性。
func (s *Service) resolveServer(serverID string) (*ops.Manager, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureOpsFreshLocked(); err != nil {
		return nil, "", err
	}
	node := findNode(s.store.Snapshot(), serverID)
	if node == nil || node.Type != connectionstore.KindConnection || node.Config == nil {
		return nil, "", errServerNotFound
	}
	return s.ops, node.Config.Host, nil
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
	if s.bridgeListener != nil {
		_ = s.bridgeListener.Close()
		s.bridgeListener = nil
	}
	removeBridgeInfo(s.dataDir)
	if s.terminals != nil {
		s.terminals.closeAll()
	}
	if s.transfers != nil {
		s.transfers.cancelAll()
	}
	s.resolvePending(-1)
	if s.ops != nil {
		s.ops.Shutdown()
		s.ops = nil
	}
}

// resolvePending 结算待审批写入：approved>0 同意、<0 拒绝、0 保持等待。
func (s *Service) resolvePending(approved int) {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	if s.pendingWrite == nil || approved == 0 {
		return
	}
	if approved > 0 {
		s.pendingWrite.decided <- true
	} else {
		s.pendingWrite.decided <- false
	}
	s.pendingWrite = nil
}

// handle 分发单个请求。返回 exit=true 表示客户端请求退出，主循环应优雅结束。
func (s *Service) handle(req Request) (resp Response, exit bool) {
	defer func() {
		if r := recover(); r != nil {
			resp = errorResponse(req.ID, CodeInternal, fmt.Sprintf("内部错误: %v", r), "")
			exit = false
		}
	}()
	s.touch()

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
	case MethodTransferUpload:
		return s.handleTransfer(req, transferKindUpload)
	case MethodTransferDownload:
		return s.handleTransfer(req, transferKindDownload)
	case MethodTransferCancel:
		return s.handleTransferCancel(req)
	case MethodTerminalOpen:
		return s.handleTerminalOpen(req)
	case MethodTerminalWrite:
		return s.handleTerminalWrite(req)
	case MethodTerminalReady:
		return s.handleTerminalSimple(req, false)
	case MethodTerminalResize:
		return s.handleTerminalResize(req)
	case MethodTerminalClose:
		return s.handleTerminalSimple(req, true)
	case MethodTerminalApproveHandoff:
		return s.handleTerminalApproveHandoff(req)
	case MethodSettingsGet:
		return s.handleSettingsGet(req)
	case MethodSettingsSet:
		return s.handleSettingsSet(req)
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
	s.mu.Unlock()

	result, err := s.execByNode(node, ExecParams{
		ServerID:      params.ServerID,
		Command:       params.Command,
		Intent:        params.Intent,
		MaxLineLength: params.MaxLineLength,
		TimeoutSec:    params.TimeoutSec,
	})
	if err != nil {
		if errors.Is(err, errServerNotFound) {
			return errorResponse(req.ID, CodeNotFound, "服务器未找到", "请先在连接面板中添加该服务器"), false
		}
		return errorResponse(req.ID, classifyExecError(err.Error()), err.Error(), ""), false
	}
	return okResponse(req.ID, result), false
}

// execByNode 执行语义单点：内核新鲜度 + 白名单预检（结构化拒绝文案与面板
// 一致）+ 超时/行长归一。面板 exec 与 Agent 的 remote_exec 共用。
func (s *Service) execByNode(node *connectionstore.Node, params ExecParams) (*ops.ExecResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureOpsFreshLocked(); err != nil {
		return nil, err
	}
	if node == nil || node.Type != connectionstore.KindConnection || node.Config == nil {
		return nil, errServerNotFound
	}
	host := node.Config.Host
	if check := s.whitelist.Check(params.Command, host); !check.Allowed {
		return nil, fmt.Errorf("%s（可在 OpsCopilot 白名单配置中为该 IP 段放行对应命令）", check.Reason)
	}
	timeout := s.execTimeout
	if params.TimeoutSec > 0 {
		timeout = time.Duration(params.TimeoutSec) * time.Second
	}
	maxLineLength := params.MaxLineLength
	if maxLineLength <= 0 {
		maxLineLength = 500
	}
	return s.ops.Exec(context.Background(), host, params.Command, ops.ExecOptions{
		MaxLineLength: maxLineLength,
		Timeout:       timeout,
		Intent:        params.Intent,
	})
}

// handleTransfer 启动异步传输任务（上传/下载），即刻返回任务 ID；
// 进度与结果经 transfer.progress / transfer.done 事件下发。
func (s *Service) handleTransfer(req Request, kind string) (Response, bool) {
	var params struct {
		ServerID   string `json:"serverId"`
		LocalPath  string `json:"localPath"`
		RemotePath string `json:"remotePath"`
		MaxBytes   int    `json:"maxBytes"`
		Backup     bool   `json:"backup"`
		Mkdir      bool   `json:"mkdir"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return invalidParams(req.ID, "params 解析失败: "+err.Error()), false
	}
	if params.ServerID == "" || params.LocalPath == "" || params.RemotePath == "" {
		return invalidParams(req.ID, "serverId、localPath 与 remotePath 必填"), false
	}
	mgr, host, err := s.resolveServer(params.ServerID)
	if err != nil {
		return s.serverResolveError(req.ID, err), false
	}
	id, err := s.transfers.start(kind, mgr, host, params.LocalPath, params.RemotePath, params.MaxBytes, params.Backup, params.Mkdir)
	if err != nil {
		return errorResponse(req.ID, CodeInternal, err.Error(), ""), false
	}
	return okResponse(req.ID, map[string]any{"transferId": id}), false
}

// handleTransferCancel 取消在途传输任务。
func (s *Service) handleTransferCancel(req Request) (Response, bool) {
	var params struct {
		TransferID string `json:"transferId"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.TransferID == "" {
		return invalidParams(req.ID, "transferId 必填"), false
	}
	if err := s.transfers.cancel(params.TransferID); err != nil {
		return errorResponse(req.ID, CodeNotFound, err.Error(), ""), false
	}
	return okResponse(req.ID, nil), false
}

// handleTerminalOpen 打开（或按 sessionId 重连）交互终端会话。
func (s *Service) handleTerminalOpen(req Request) (Response, bool) {
	var params struct {
		ServerID  string `json:"serverId"`
		SessionID string `json:"sessionId"`
		Cols      int    `json:"cols"`
		Rows      int    `json:"rows"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return invalidParams(req.ID, "params 解析失败: "+err.Error()), false
	}
	if params.ServerID == "" {
		return invalidParams(req.ID, "serverId 必填"), false
	}
	mgr, host, err := s.resolveServer(params.ServerID)
	if err != nil {
		return s.serverResolveError(req.ID, err), false
	}
	// 惰性连接（幂等）：命中已有连接时刷新 LastActive，避免会话使用期间
	// 被空闲回收器断开
	if _, err := mgr.Connect(host); err != nil {
		return errorResponse(req.ID, classifyExecError(err.Error()), err.Error(), ""), false
	}
	conn := mgr.GetConnection(host)
	if conn == nil {
		return errorResponse(req.ID, CodeInternal, "连接已建立但不可用", ""), false
	}
	data, err := s.terminals.open(conn, params.SessionID, params.Cols, params.Rows)
	if err != nil {
		code := CodeInternal
		if errors.Is(err, errTerminalBusy) {
			code = CodeTerminalBusy
		} else if strings.Contains(err.Error(), "PTY") {
			code = CodeConnectFailed
		}
		return errorResponse(req.ID, code, err.Error(), ""), false
	}
	return okResponse(req.ID, data), false
}

// handleTerminalWrite 向终端写入按键数据。
func (s *Service) handleTerminalWrite(req Request) (Response, bool) {
	var params struct {
		SessionID string `json:"sessionId"`
		Data      string `json:"data"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return invalidParams(req.ID, "params 解析失败: "+err.Error()), false
	}
	if params.SessionID == "" {
		return invalidParams(req.ID, "sessionId 必填"), false
	}
	if err := s.terminals.write(params.SessionID, params.Data); err != nil {
		return terminalStateError(req.ID, err), false
	}
	return okResponse(req.ID, nil), false
}

// handleTerminalSimple 处理 terminal/ready（冲刷首段输出）与 terminal/close
// （关闭会话）。
func (s *Service) handleTerminalSimple(req Request, isClose bool) (Response, bool) {
	var params struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.SessionID == "" {
		return invalidParams(req.ID, "sessionId 必填"), false
	}
	var err error
	if isClose {
		err = s.terminals.close(params.SessionID)
	} else {
		err = s.terminals.ready(params.SessionID)
	}
	if err != nil {
		return terminalStateError(req.ID, err), false
	}
	return okResponse(req.ID, nil), false
}

// handleTerminalResize 同步终端尺寸到远端。
func (s *Service) handleTerminalResize(req Request) (Response, bool) {
	var params struct {
		SessionID string `json:"sessionId"`
		Cols      int    `json:"cols"`
		Rows      int    `json:"rows"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.SessionID == "" {
		return invalidParams(req.ID, "sessionId 必填"), false
	}
	if params.Cols <= 0 || params.Rows <= 0 {
		return invalidParams(req.ID, "cols 与 rows 必须为正"), false
	}
	if err := s.terminals.resize(params.SessionID, params.Cols, params.Rows); err != nil {
		return terminalStateError(req.ID, err), false
	}
	return okResponse(req.ID, nil), false
}

// handleTerminalApproveHandoff 用户裁决待确认的终端写入（handoff 状态条）。
func (s *Service) handleTerminalApproveHandoff(req Request) (Response, bool) {
	var params struct {
		SessionID string `json:"sessionId"`
		Approved  bool   `json:"approved"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.SessionID == "" {
		return invalidParams(req.ID, "sessionId 必填"), false
	}
	s.pendingMu.Lock()
	pending := s.pendingWrite
	if pending != nil && pending.sessionID == params.SessionID {
		s.pendingWrite = nil
	} else {
		pending = nil
	}
	s.pendingMu.Unlock()
	if pending == nil {
		return errorResponse(req.ID, CodeNotFound, "没有等待确认的终端写入", ""), false
	}
	pending.decided <- params.Approved
	return okResponse(req.ID, nil), false
}

// terminalWriteForAgent Agent 写终端：manual 档位经 Desktop 审批（挂起等待
// handoff 状态条裁决），auto 直接放行；全程审计。
func (s *Service) terminalWriteForAgent(sessionID, data string) (map[string]any, error) {
	if sessionID == "" || data == "" {
		return nil, errors.New("session_id 与 data 必填")
	}
	mode := s.approvalMode()
	if mode == "manual" {
		s.pendingMu.Lock()
		if s.pendingWrite != nil {
			s.pendingMu.Unlock()
			return nil, errors.New("已有等待用户确认的终端写入，请稍后重试")
		}
		pending := &pendingHandoff{sessionID: sessionID, data: data, decided: make(chan bool, 1)}
		s.pendingWrite = pending
		s.pendingMu.Unlock()
		s.emitEvent(EventTerminalHandoff, map[string]any{
			"sessionId": sessionID,
			"data":      data,
		})
		select {
		case approved := <-pending.decided:
			if !approved {
				return nil, errors.New("用户拒绝了本次终端写入")
			}
		case <-time.After(2 * time.Minute):
			s.pendingMu.Lock()
			if s.pendingWrite == pending {
				s.pendingWrite = nil
			}
			s.pendingMu.Unlock()
			return nil, errors.New("审批等待超时（2 分钟），写入已取消")
		}
	}
	if err := s.terminals.write(sessionID, data); err != nil {
		return nil, err
	}
	s.audit(auditRecord{
		Time:   time.Now().Unix(),
		Tool:   "terminal_write",
		Server: sessionID,
		Detail: fmt.Sprintf("%d 字节（%s 档位）", len(data), mode),
		OK:     true,
	})
	return map[string]any{"session_id": sessionID, "written": len(data), "mode": mode}, nil
}

// handleSettingsGet 读取 Agent 工具相关设置（审批档位）。
func (s *Service) handleSettingsGet(req Request) (Response, bool) {
	return okResponse(req.ID, map[string]any{"approvalMode": s.approvalMode()}), false
}

// handleSettingsSet 更新审批档位（Desktop 设置项驱动）。
func (s *Service) handleSettingsSet(req Request) (Response, bool) {
	var params struct {
		ApprovalMode string `json:"approvalMode"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.ApprovalMode == "" {
		return invalidParams(req.ID, "approvalMode 必填"), false
	}
	if err := s.setApprovalMode(params.ApprovalMode); err != nil {
		return errorResponse(req.ID, CodeInvalidParams, err.Error(), ""), false
	}
	return okResponse(req.ID, nil), false
}

// serverResolveError 把服务器解析失败映射为结构化错误响应。
func (s *Service) serverResolveError(id string, err error) Response {
	if errors.Is(err, errServerNotFound) {
		return errorResponse(id, CodeNotFound, err.Error(), "请先在连接面板中添加该服务器")
	}
	return errorResponse(id, CodeInternal, err.Error(), "")
}

// terminalStateError 把终端会话状态错误映射为结构化错误响应。
func terminalStateError(id string, err error) Response {
	switch {
	case errors.Is(err, errTerminalUnknown):
		return errorResponse(id, CodeNotFound, err.Error(), "")
	case errors.Is(err, errTerminalDead):
		return errorResponse(id, CodeConnectFailed, err.Error(), "请重新打开终端")
	default:
		return errorResponse(id, CodeInternal, err.Error(), "")
	}
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

// Run 启动 stdio 主循环：每行一个请求、串行处理；空闲超时（无活动且无
// 占用）自动退出实现零常驻，下次请求由调用方按代次自动重启。数据目录由
// OPSCOPILOT_DATA_DIR 注入，缺省退回 exe 同目录（与 OpsCopilot 桌面 CLI
// 的路径语义一致）。
func Run() int {
	svc, err := NewService(resolveDataDir(), os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ops-sidecar] 初始化失败: %v\n", err)
		return 1
	}

	lines := make(chan []byte)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			line := bytes.TrimSpace(scanner.Bytes())
			if len(line) == 0 {
				continue
			}
			lines <- append([]byte(nil), line...)
		}
		close(lines)
	}()

	idle := time.NewTicker(time.Minute)
	defer idle.Stop()
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				svc.Close()
				return 0
			}
			var req Request
			if err := json.Unmarshal(line, &req); err != nil {
				svc.out.line(errorResponse("", CodeInvalidParams, "请求不是合法 JSON: "+err.Error(), ""))
				continue
			}
			resp, exit := svc.handle(req)
			svc.out.line(resp)
			if exit {
				svc.Close()
				return 0
			}
		case <-idle.C:
			if svc.idleCheck() {
				slog.Info("[ops-sidecar] 空闲超时，自动退出（下次使用自动重启）")
				svc.Close()
				return 0
			}
		}
	}
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

// ResolveDataDir 数据目录解析的导出入口（--mcp 代理与 service 共用）。
func ResolveDataDir() string {
	return resolveDataDir()
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
