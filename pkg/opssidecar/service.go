package opssidecar

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
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
	"opscopilot/pkg/secretstore"
	"opscopilot/pkg/terminalcontrol"
)

// Service 常驻运维服务。
//
// 连接树（connectionstore.Store）与执行内核（ops.Manager）是两个实例、指向同一份
// sessions.json：树编辑走 Store 并落盘，exec/共享连接走 Manager。树变更后标记
// dirty，使用前按 dirty/mtime 重建；mtime 检查兜住 OpsCopilot GUI 的外部编辑。
// 交互终端不进共享池：每会话一条独立连接（Manager.ConnectIndependent）。
type Service struct {
	dataDir string

	store     *connectionstore.Store
	whitelist *security.WhitelistManager

	ops                *ops.Manager
	opsSessionsModTime time.Time
	opsDirty           bool
	execTimeout        time.Duration

	// out 是 stdout 的唯一写入者：响应（串行路径）与事件（终端 pump、传输
	// goroutine）经它整行落盘。
	out       *lineEmitter
	terminals *terminalManager
	ft        *ftHub
	tcSvc     *terminalcontrol.Service
	secrets   secretstore.SecretStore
	projects  *projectPool

	// MCP 桥（随机端口 + 内存态 token；Desktop 经 bridge/info 取凭据 env 直传）
	bridgeMu   sync.Mutex
	bridgeAddr string
	bridgeTok  string
	bridgeLsn  net.Listener

	auditMu   sync.Mutex
	lastTouch atomic.Int64

	// 手工审批：requestID 并发的待确认终端写入。
	pendingMu    sync.Mutex
	pendingWrite map[string]*pendingHandoff

	mu sync.Mutex
}

// pendingHandoff 一条等待用户确认的终端写入（manual 审批档位）。
type pendingHandoff struct {
	requestID string
	hint      string
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
// 约定一致。out 为协议行输出目标（生产 os.Stdout，测试注入缓冲）。
func NewService(dataDir string, out io.Writer) (*Service, error) {
	s := &Service{dataDir: dataDir, pendingWrite: map[string]*pendingHandoff{}}

	s.store = connectionstore.NewStoreWithPath(filepath.Join(dataDir, "sessions.json"))
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
			Time: time.Now().Unix(), Tool: "terminal_control", Server: a.SessionID,
			Detail: a.Action + "@" + a.Phase, OK: true,
		})
	})
	s.terminals = newTerminalManager(s.emitEvent, s.tcSvc, s.dialTerminal)
	s.ft = newFtHub(s, s.emitEvent)
	s.secrets = secretstore.NewKeyringStore()
	s.projects = newProjectPool()
	s.lastTouch.Store(time.Now().Unix())
	s.startBridge()
	return s, nil
}

// dialTerminal 终端独立拨号（serverID = 树节点 ID；按节点配置精确拨）。
// 不能按 host 再拨：同 host 多节点（127.0.0.1 的 ssh/telnet 并存）时
// findSessionConfig 的 host-first 匹配会永远命中第一条。
func (s *Service) dialTerminal(serverID string) (*ops.Connection, error) {
	s.mu.Lock()
	node := findNode(s.store.Snapshot(), serverID)
	s.mu.Unlock()
	if node == nil || node.Type != connectionstore.KindConnection || node.Config == nil {
		return nil, errServerNotFound
	}
	return s.ops.ConnectIndependentNode(node)
}

// looksLikeHTTP 判断连接首 8 字节是否为 HTTP 请求行（方法 + 空格 + 路径）。
func looksLikeHTTP(head []byte) bool {
	if len(head) < 8 {
		return false
	}
	line := string(head)
	for _, m := range []string{"GET ", "POST", "HEAD", "PUT ", "DELE", "OPTI"} {
		if line[:4] == m {
			return true
		}
	}
	return false
}

// bufferedConn 用嗅探阶段建立的 bufio.Reader 包装连接（Peek 过的字节
// 不丢失，后续读经由缓冲层继续）。
type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.r.Read(p) }

// audit 追加一条审计记录到 ops-audit.jsonl。
func (s *Service) audit(rec auditRecord) {
	s.auditMu.Lock()
	defer s.auditMu.Unlock()
	file, err := os.OpenFile(filepath.Join(s.dataDir, auditFileName), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	if raw, err := json.Marshal(rec); err == nil {
		_, _ = file.Write(append(raw, 10))
	}
}

func (s *Service) emitEvent(event string, params any) {
	s.out.line(Event{Event: event, Params: params})
}

var errServerNotFound = errors.New("服务器未找到")

// resolveServer 按树节点 ID 解析当前内核与主机（FT/MCP 等按 host 复用
// 共享池配置的路径专用；终端独立拨号走 ConnectIndependentNode）。
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

func (s *Service) ensureOpsFreshLocked() error {
	if !s.opsDirty && s.opsSessionsModTime.Equal(sessionsModTime(s.dataDir)) {
		return nil
	}
	return s.rebuildOpsLocked()
}

func (s *Service) Close() {
	s.bridgeMu.Lock()
	if s.bridgeLsn != nil {
		_ = s.bridgeLsn.Close()
		s.bridgeLsn = nil
	}
	s.bridgeMu.Unlock()
	removeBridgeInfo(s.dataDir)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.terminals != nil {
		s.terminals.closeAll()
	}
	if s.ft != nil {
		s.ft.cancelAll()
	}
	if s.projects != nil {
		s.projects.closeAll()
	}
	s.rejectAllPendingLocked()
	if s.ops != nil {
		s.ops.Shutdown()
		s.ops = nil
	}
}

func (s *Service) rejectAllPendingLocked() {
	s.pendingMu.Lock()
	pending := s.pendingWrite
	s.pendingWrite = map[string]*pendingHandoff{}
	s.pendingMu.Unlock()
	for _, p := range pending {
		p.decided <- false
	}
}

// touch 记录一次活动，推迟空闲退出。
func (s *Service) touch() { s.lastTouch.Store(time.Now().Unix()) }

var idleExitAfter = 15 * time.Minute

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
	busy := len(s.pendingWrite) > 0
	s.pendingMu.Unlock()
	return !busy
}

// startBridge MCP 桥：随机回环端口 + 每次启动随机 token（内存态）。
// Desktop 经 bridge/info 取地址与 token，通过 mcpServers env 直传给 --mcp
// 代理；发现文件仅作代理的拉起兜底（service 空闲退出后代理重启它）。
func (s *Service) startBridge() {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		slog.Warn("[ops-sidecar] MCP 桥 token 生成失败，Agent 工具不可用", "error", err)
		return
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		slog.Warn("[ops-sidecar] MCP 桥端口不可用，服务降级运行（面板可用）", "error", err)
		return
	}
	info := BridgeInfo{
		Address: listener.Addr().String(),
		Token:   hex.EncodeToString(key),
		PID:     os.Getpid(),
	}
	if err := writeBridgeInfo(s.dataDir, info); err != nil {
		slog.Warn("[ops-sidecar] 服务发现文件写入失败，Agent 工具拉起兜底不可用", "error", err)
		_ = listener.Close()
		return
	}
	s.bridgeMu.Lock()
	s.bridgeLsn = listener
	s.bridgeAddr = info.Address
	s.bridgeTok = info.Token
	s.bridgeMu.Unlock()
	gateway := &mcpGateway{svc: s, token: info.Token}
	httpSrv := &http.Server{Handler: http.HandlerFunc(gateway.handleHTTP)}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				// 协议嗅探：HTTP 请求（POST/GET/OPTIONS）走 MCP HTTP 端点，
				// 其余（NDJSON 信封）走 stdio 代理的 TCP 桥（遗留兼容）。
				// Peek 不消费字节——HTTP 分支把带缓冲的连接交给 http.Server，
				// 避免请求行被啃掉前 8 字节。
				reader := bufio.NewReader(conn)
				head, err := reader.Peek(8)
				if err != nil || !looksLikeHTTP(head) {
					gateway.serveConn(conn)
					return
				}
				_ = httpSrv.Serve(&oneConnListener{
					conn: &bufferedConn{Conn: conn, r: reader},
					done: make(chan struct{}),
				})
			}()
		}
	}()
	slog.Info("[ops-sidecar] MCP 桥已就绪", "address", info.Address)
}

// bridgeInfo 桥凭据（Desktop 经 env 直传给代理）。
func (s *Service) bridgeInfo() (string, string) {
	s.bridgeMu.Lock()
	defer s.bridgeMu.Unlock()
	return s.bridgeAddr, s.bridgeTok
}

// handle 分发单个请求。返回 exit=true 表示客户端请求退出。
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
	case MethodReorderChildren:
		return s.handleReorderChildren(req)
	case MethodDeleteNode:
		return s.handleDeleteNode(req)
	case MethodExec:
		return s.handleExec(req)

	case MethodFTCheck, MethodFTList, MethodFTStat, MethodFTRemoteMkdir,
		MethodFTRemoteRemove, MethodFTRemoteRename, MethodFTReadFile, MethodFTWriteFile:
		return s.handleFTSync(req)
	case MethodFTUpload, MethodFTDownload, MethodFTUploadDir, MethodFTDownloadDir:
		return s.handleFTStart(req)
	case MethodFTCancel:
		return s.handleFTCancel(req)
	case MethodLocalList, MethodLocalMkdir, MethodLocalRemove,
		MethodLocalRename, MethodLocalCopy, MethodLocalStat:
		return s.handleLocal(req)

	case MethodTerminalOpen:
		return s.handleTerminalOpen(req)
	case MethodTerminalWrite:
		return s.handleTerminalSimple(req, "write")
	case MethodTerminalReady:
		return s.handleTerminalSimple(req, "ready")
	case MethodTerminalResize:
		return s.handleTerminalResize(req)
	case MethodTerminalClose:
		return s.handleTerminalSimple(req, "close")
	case MethodTerminalApproveHandoff:
		return s.handleTerminalApproveHandoff(req)

	case MethodSettingsGet:
		return okResponse(req.ID, map[string]any{"approvalMode": s.approvalMode()}), false
	case MethodSettingsSet:
		return s.handleSettingsSet(req)
	case MethodWhitelistGet:
		return okResponse(req.ID, s.whitelist.GetConfig()), false
	case MethodWhitelistSet:
		return s.handleWhitelistSet(req)
	case MethodWhitelistForIP:
		return s.handleWhitelistForIP(req)
	case MethodFileAccessGet:
		return s.handleFileAccessGet(req)
	case MethodFileAccessSet:
		return s.handleFileAccessSet(req)
	case MethodProjectConnectionsList:
		return s.handleProjectConnectionsList(req)
	case MethodProjectConnectionCreate:
		return s.handleProjectConnectionCreate(req)
	case MethodProjectConnectionUpdate:
		return s.handleProjectConnectionUpdate(req)
	case MethodProjectConnectionDelete:
		return s.handleProjectConnectionDelete(req)
	case MethodProjectConnectionTest:
		return s.handleProjectConnectionTest(req)
	case MethodProjectFSList:
		return s.handleProjectFSList(req)
	case MethodProjectFSStat:
		return s.handleProjectFSStat(req)
	case MethodProjectFSPreview:
		return s.handleProjectFSPreview(req)
	case MethodProjectFSListFiles:
		return s.handleProjectFSListFiles(req)
	case MethodBridgeInfo:
		addr, token := s.bridgeInfo()
		return okResponse(req.ID, map[string]any{"address": addr, "token": token}), false
	default:
		return errorResponse(req.ID, CodeMethodNotFound, "未知方法: "+req.Method, ""), false
	}
}

// ---- 连接树 ----

func (s *Service) handleList(req Request) (Response, bool) {
	s.mu.Lock()
	nodes := s.store.Snapshot()
	s.mu.Unlock()
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
		Config   json.RawMessage `json:"config"`
		ParentID string          `json:"parentId"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params.Config) == 0 {
		return invalidParams(req.ID, "config 必填"), false
	}
	var cfg connectionNodeConfig
	if err := json.Unmarshal(params.Config, &cfg); err != nil || cfg.Host == "" || cfg.User == "" {
		return invalidParams(req.ID, "config.host 与 config.user 必填"), false
	}
	node, err := s.store.CreateConnection(cfg, params.ParentID)
	if err != nil {
		return errorResponse(req.ID, CodeInternal, err.Error(), ""), false
	}
	s.markDirty()
	return okResponse(req.ID, node), false
}

func (s *Service) handleUpdateConnection(req Request) (Response, bool) {
	var params struct {
		ID     string          `json:"id"`
		Config json.RawMessage `json:"config"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.ID == "" || len(params.Config) == 0 {
		return invalidParams(req.ID, "id 与 config 必填"), false
	}
	var cfg connectionNodeConfig
	if err := json.Unmarshal(params.Config, &cfg); err != nil {
		return invalidParams(req.ID, "config 解析失败"), false
	}
	if err := s.store.UpdateConnection(params.ID, cfg); err != nil {
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

func (s *Service) handleReorderChildren(req Request) (Response, bool) {
	var params struct {
		ParentID   string   `json:"parentId"`
		OrderedIDs []string `json:"orderedIds"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params.OrderedIDs) == 0 {
		return invalidParams(req.ID, "orderedIds 必填"), false
	}
	if err := s.store.ReorderNodes(params.ParentID, params.OrderedIDs); err != nil {
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

func (s *Service) markDirty() {
	s.mu.Lock()
	s.opsDirty = true
	s.mu.Unlock()
}

// ---- exec ----

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
			return errorResponse(req.ID, CodeNotFound, err.Error(), "请先在连接面板中添加该服务器"), false
		}
		return errorResponse(req.ID, classifyExecError(err.Error()), err.Error(), ""), false
	}
	s.audit(auditRecord{
		Time: time.Now().Unix(), Tool: "remote_exec", Server: params.ServerID,
		Command: params.Command, Detail: params.Intent, OK: true,
	})
	return okResponse(req.ID, result), false
}

// execByNode 执行语义单点：内核新鲜度 + 白名单预检 + 超时/行长归一。
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

// ---- 文件传输（FT 族 / Local 族；返回 ftResponse 信封字符串） ----

func (s *Service) handleFTSync(req Request) (Response, bool) {
	var params struct {
		SessionID string `json:"sessionId"`
		Path      string `json:"path"`
		NewPath   string `json:"newPath"`
		Content   string `json:"content"`
		MaxBytes  int    `json:"maxBytes"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.SessionID == "" {
		return invalidParams(req.ID, "sessionId 必填"), false
	}
	transport, _, err := s.ft.transportFor(params.SessionID)
	if err != nil {
		return okResponse(req.ID, ftErr(err)), false
	}
	ctx := context.Background()
	var envelope string
	switch req.Method {
	case MethodFTCheck:
		ok, message, err := transport.Check(ctx)
		switch {
		case err != nil:
			envelope = ftErr(err)
		case !ok:
			envelope = ftMessage(message)
		default:
			envelope = ftOK()
		}
	case MethodFTList:
		entries, err := transport.List(ctx, params.Path)
		if err != nil {
			envelope = ftErr(err)
		} else {
			envelope = ftEntries(entries)
		}
	case MethodFTStat:
		entry, err := transport.Stat(ctx, params.Path)
		if err != nil {
			envelope = ftErr(err)
		} else {
			envelope = ftEntry(&entry)
		}
	case MethodFTRemoteMkdir:
		if err := transport.Mkdir(ctx, params.Path); err != nil {
			envelope = ftErr(err)
		} else {
			envelope = ftOK()
		}
	case MethodFTRemoteRemove:
		if err := transport.Remove(ctx, params.Path, true); err != nil {
			envelope = ftErr(err)
		} else {
			envelope = ftOK()
		}
	case MethodFTRemoteRename:
		if err := transport.Rename(ctx, params.Path, params.NewPath); err != nil {
			envelope = ftErr(err)
		} else {
			envelope = ftOK()
		}
	case MethodFTReadFile:
		maxBytes := int64(params.MaxBytes)
		if maxBytes <= 0 {
			maxBytes = 10 << 20
		}
		data, err := transport.ReadFile(ctx, params.Path, maxBytes)
		if err != nil {
			envelope = ftErr(err)
		} else {
			envelope = string(mustMarshalJSON(map[string]any{"ok": true, "content": string(data)}))
		}
	case MethodFTWriteFile:
		if err := transport.WriteFile(ctx, params.Path, []byte(params.Content)); err != nil {
			envelope = ftErr(err)
		} else {
			envelope = ftOK()
		}
	}
	return okResponse(req.ID, envelope), false
}

func (s *Service) handleFTStart(req Request) (Response, bool) {
	var params struct {
		SessionID  string `json:"sessionId"`
		LocalPath  string `json:"localPath"`
		RemotePath string `json:"remotePath"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return invalidParams(req.ID, "params 解析失败: "+err.Error()), false
	}
	if params.SessionID == "" || params.LocalPath == "" || params.RemotePath == "" {
		return invalidParams(req.ID, "sessionId、localPath 与 remotePath 必填"), false
	}
	kind := map[string]string{
		MethodFTUpload:      "upload",
		MethodFTDownload:    "download",
		MethodFTUploadDir:   "upload",
		MethodFTDownloadDir: "download",
	}[req.Method]
	if strings.Contains(req.Method, "dir") {
		if kind == "upload" && !isDir(params.LocalPath) {
			return okResponse(req.ID, ftErr(errors.New("本地路径不是目录"))), false
		}
	} else if kind == "upload" {
		if _, err := os.Stat(params.LocalPath); err != nil {
			return okResponse(req.ID, ftErr(err)), false
		}
	}
	s.audit(auditRecord{
		Time: time.Now().Unix(), Tool: req.Method, Server: params.SessionID,
		Command: params.RemotePath, Detail: params.LocalPath, OK: true,
	})
	id := s.ft.start(params.SessionID, kind, params.LocalPath, params.RemotePath)
	return okResponse(req.ID, ftTaskEnvelope(id)), false
}

func (s *Service) handleFTCancel(req Request) (Response, bool) {
	var params struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.TaskID == "" {
		return invalidParams(req.ID, "taskId 必填"), false
	}
	return okResponse(req.ID, s.ft.cancel(params.TaskID)), false
}

func (s *Service) handleLocal(req Request) (Response, bool) {
	var params struct {
		Path    string `json:"path"`
		NewPath string `json:"newPath"`
		Dst     string `json:"dst"`
		Src     string `json:"src"`
	}
	_ = json.Unmarshal(req.Params, &params)
	var envelope string
	switch req.Method {
	case MethodLocalList:
		envelope = localList(params.Path)
	case MethodLocalMkdir:
		envelope = localMkdir(params.Path)
	case MethodLocalRemove:
		envelope = localRemove(params.Path)
	case MethodLocalRename:
		envelope = localRename(params.Path, params.NewPath)
	case MethodLocalCopy:
		envelope = localCopy(params.Src, params.Dst)
	case MethodLocalStat:
		envelope = localStat(params.Path)
	}
	return okResponse(req.ID, envelope), false
}

// ---- 终端 ----

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
	data, err := s.terminals.open(params.ServerID, params.SessionID, params.Cols, params.Rows)
	if err != nil {
		code := CodeInternal
		if errors.Is(err, errServerNotFound) {
			code = CodeNotFound
		} else if strings.Contains(err.Error(), "连接失败") || strings.Contains(err.Error(), "PTY") {
			code = CodeConnectFailed
		}
		return errorResponse(req.ID, code, err.Error(), ""), false
	}
	return okResponse(req.ID, data), false
}

func (s *Service) handleTerminalSimple(req Request, op string) (Response, bool) {
	var params struct {
		SessionID string `json:"sessionId"`
		Data      string `json:"data"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.SessionID == "" {
		return invalidParams(req.ID, "sessionId 必填"), false
	}
	var err error
	switch op {
	case "write":
		err = s.terminals.write(params.SessionID, params.Data)
	case "ready":
		err = s.terminals.ready(params.SessionID)
	case "close":
		err = s.terminals.close(params.SessionID)
	}
	if err != nil {
		return terminalStateError(req.ID, err), false
	}
	return okResponse(req.ID, nil), false
}

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

// handleTerminalApproveHandoff 用户裁决待确认的终端写入（requestId 并发）。
func (s *Service) handleTerminalApproveHandoff(req Request) (Response, bool) {
	var params struct {
		RequestID string `json:"requestId"`
		Approved  bool   `json:"approved"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.RequestID == "" {
		return invalidParams(req.ID, "requestId 必填"), false
	}
	s.pendingMu.Lock()
	pending := s.pendingWrite[params.RequestID]
	delete(s.pendingWrite, params.RequestID)
	s.pendingMu.Unlock()
	if pending == nil {
		return errorResponse(req.ID, CodeNotFound, "没有等待确认的终端写入", ""), false
	}
	pending.decided <- params.Approved
	return okResponse(req.ID, nil), false
}

// terminalWriteForAgent Agent 写终端：manual 档位发 handoff 事件等待裁决
// （requestId 并发，互不阻塞）；auto 直接放行；全程审计。
func (s *Service) terminalWriteForAgent(hint, sessionID, data string) (map[string]any, error) {
	if sessionID == "" || data == "" {
		return nil, errors.New("session_id 与 data 必填")
	}
	mode := s.approvalMode()
	if mode == "manual" {
		requestID := newRequestID()
		pending := &pendingHandoff{
			requestID: requestID, hint: hint, sessionID: sessionID,
			data: data, decided: make(chan bool, 1),
		}
		s.pendingMu.Lock()
		s.pendingWrite[requestID] = pending
		s.pendingMu.Unlock()
		s.emitEvent(EventTerminalHandoff, map[string]any{
			"requestId": requestID, "hint": hint,
			"sessionId": sessionID, "data": data,
		})
		select {
		case approved := <-pending.decided:
			if !approved {
				s.audit(auditRecord{Time: time.Now().Unix(), Tool: "terminal_write", Server: sessionID, Command: data, Detail: "用户拒绝", OK: false})
				return nil, errors.New("用户拒绝了本次终端写入")
			}
		case <-time.After(2 * time.Minute):
			s.pendingMu.Lock()
			delete(s.pendingWrite, requestID)
			s.pendingMu.Unlock()
			return nil, errors.New("审批等待超时（2 分钟），写入已取消")
		}
	}
	if err := s.terminals.write(sessionID, data); err != nil {
		return nil, err
	}
	s.audit(auditRecord{
		Time: time.Now().Unix(), Tool: "terminal_write", Server: sessionID,
		Command: data, Detail: mode + " 档位", OK: true,
	})
	return map[string]any{"session_id": sessionID, "written": len(data), "mode": mode}, nil
}

// ---- 设置与闸门配置 ----

func (s *Service) approvalMode() string {
	raw, err := os.ReadFile(filepath.Join(s.dataDir, "config.json"))
	if err == nil {
		var cfg struct {
			RemoteOps struct {
				ApprovalMode string `json:"approvalMode"`
			} `json:"remoteOps"`
		}
		if json.Unmarshal(raw, &cfg) == nil && cfg.RemoteOps.ApprovalMode == "auto" {
			return "auto"
		}
	}
	return "manual"
}

func (s *Service) setApprovalMode(mode string) error {
	if mode != "manual" && mode != "auto" {
		return fmt.Errorf("审批档位只支持 manual / auto")
	}
	return updateJSONSection(filepath.Join(s.dataDir, "config.json"), "remoteOps", map[string]any{"approvalMode": mode})
}

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

func (s *Service) handleWhitelistSet(req Request) (Response, bool) {
	var params struct {
		Config json.RawMessage `json:"config"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params.Config) == 0 {
		return invalidParams(req.ID, "config 必填"), false
	}
	var cfg security.WhitelistConfig
	if err := json.Unmarshal(params.Config, &cfg); err != nil {
		return invalidParams(req.ID, "config 解析失败: "+err.Error()), false
	}
	if err := s.whitelist.UpdateConfig(&cfg); err != nil {
		return errorResponse(req.ID, CodeInvalidParams, err.Error(), ""), false
	}
	s.markDirty()
	return okResponse(req.ID, nil), false
}

func (s *Service) handleWhitelistForIP(req Request) (Response, bool) {
	var params struct {
		IP string `json:"ip"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.IP == "" {
		return invalidParams(req.ID, "ip 必填"), false
	}
	return okResponse(req.ID, s.whitelist.GetPoliciesForIP(params.IP)), false
}

func (s *Service) handleFileAccessGet(req Request) (Response, bool) {
	checker, err := security.LoadFileAccessChecker(filepath.Join(s.dataDir, "file_access.json"))
	if err != nil {
		return errorResponse(req.ID, CodeInternal, err.Error(), ""), false
	}
	return okResponse(req.ID, checker.GetConfig()), false
}

func (s *Service) handleFileAccessSet(req Request) (Response, bool) {
	var params struct {
		Config json.RawMessage `json:"config"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || len(params.Config) == 0 {
		return invalidParams(req.ID, "config 必填"), false
	}
	checker, err := security.LoadFileAccessChecker(filepath.Join(s.dataDir, "file_access.json"))
	if err != nil {
		return errorResponse(req.ID, CodeInternal, err.Error(), ""), false
	}
	var cfg security.FileAccessConfig
	if err := json.Unmarshal(params.Config, &cfg); err != nil {
		return invalidParams(req.ID, "config 解析失败: "+err.Error()), false
	}
	if err := checker.UpdateConfig(&cfg); err != nil {
		return errorResponse(req.ID, CodeInvalidParams, err.Error(), ""), false
	}
	s.markDirty()
	return okResponse(req.ID, nil), false
}

// updateJSONSection 读改写整份 JSON 文件的指定小节，保留其余内容。
func updateJSONSection(path, section string, values map[string]any) error {
	root := map[string]json.RawMessage{}
	if raw, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(raw, &root); err != nil {
			return fmt.Errorf("%s 损坏: %w", filepath.Base(path), err)
		}
	}
	inner := map[string]any{}
	if existing, ok := root[section]; ok {
		_ = json.Unmarshal(existing, &inner)
	}
	for key, value := range values {
		inner[key] = value
	}
	updated, err := json.Marshal(inner)
	if err != nil {
		return err
	}
	root[section] = updated
	raw, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

// serverResolveError / terminalStateError：错误映射。
func (s *Service) serverResolveError(id string, err error) Response {
	if errors.Is(err, errServerNotFound) {
		return errorResponse(id, CodeNotFound, err.Error(), "请先在连接面板中添加该服务器")
	}
	return errorResponse(id, CodeInternal, err.Error(), "")
}

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

// ---- 主循环 ----

// Run 启动 stdio 主循环：每行一个请求、串行处理；空闲超时自动退出实现
// 零常驻，下次请求由调用方按代次自动重启。数据目录由 OPSCOPILOT_DATA_DIR
// 注入，缺省退回 exe 同目录。
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
func ResolveDataDir() string { return resolveDataDir() }

func okResponse(id string, data any) Response { return Response{ID: id, OK: true, Data: data} }

func errorResponse(id, code, message, hint string) Response {
	return Response{ID: id, OK: false, Error: &ErrorWire{Code: code, Message: message, Hint: hint}}
}

func invalidParams(id, message string) Response {
	return errorResponse(id, CodeInvalidParams, message, "")
}

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

// connectionNodeConfig 树节点 config 的解析载体（remote.ConnectConfig 的
// 镜像；密码等敏感字段由 PreserveCredentials 语义保留）。
type connectionNodeConfig = remote.ConnectConfig

// newRequestID 生成审批请求 ID。
func newRequestID() string {
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(key)
}
