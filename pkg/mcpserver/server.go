package mcpserver

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/sftp"
	"opscopilot/pkg/secretstore"
	"opscopilot/pkg/sessionmanager"
	"opscopilot/pkg/sshclient"
)

// Config MCP Server 配置
type Config struct {
	ConfigDir      string // 配置文件基础目录（可执行文件所在目录）
	SessionsFile   string // sessions.json 路径
	RecordingsDir  string // 录制文件存储目录
	KnowledgeDir   string // 知识库目录（用于归档排查经验）
	WhitelistPath  string // 白名单配置文件路径（可选）
	FilePath       string // 文件访问控制配置路径（可选）
	MaxTotalBytes  int
	MaxLineLength  int
	HeadLines      int
	IdleTimeoutMin int
}

// Server MCP Server 实现 - 复用 OpsCopilot 现有能力
type Server struct {
	config           *Config
	sessionMgr       *sessionmanager.Manager  // 复用现有的 session 管理器
	secretStore      secretstore.SecretStore  // 复用现有的密码存储
	connections      map[string]*Connection   // 活跃的 SSH 连接
	checker          *CommandChecker          // 简单检查器（向后兼容）
	whitelistManager *WhitelistManager        // 白名单管理器（新）
	fileChecker      *FileAccessChecker       // 文件访问检查器（新）
	localStagingDir  string                   // 本地暂存目录
	mu               sync.RWMutex
	stopChan         chan struct{}            // 停止清理 goroutine 的信号
}

// Connection SSH 连接
type Connection struct {
	Name          string
	Host          string            // 服务器 IP 地址（用于白名单匹配）
	Client        *sshclient.Client
	RootPassword  string // 用于 sudo 提权
	ConnectedAt   time.Time
	LastActive    atomic.Int64 // Unix 纳秒时间戳，支持并发读写
	sftpMu        sync.Mutex   // 保护 SFTP 客户端的并发访问
	sftpClient    *sftp.Client // 缓存的 SFTP 客户端
	sftpTested    bool         // 是否已检测过 SFTP 可用性
	sftpAvailable bool         // SFTP 是否可用
}

// NewServer 创建 MCP Server
func NewServer(config *Config) (*Server, error) {
	// 设置默认值
	if config.MaxTotalBytes == 0 {
		config.MaxTotalBytes = 10240
	}
	if config.MaxLineLength == 0 {
		config.MaxLineLength = 500
	}
	if config.HeadLines == 0 {
		config.HeadLines = 5
	}
	if config.IdleTimeoutMin == 0 {
		config.IdleTimeoutMin = 15 // 默认 15 分钟空闲超时
	}

	s := &Server{
		config:      config,
		connections: make(map[string]*Connection),
		checker:     NewCommandChecker(),
		stopChan:    make(chan struct{}),
	}

	// 启动连接空闲超时清理 goroutine
	go s.startIdleConnectionCleaner()

	// 使用现有的 sessionmanager 加载 sessions.json
	sessionsPath := config.SessionsFile
	if sessionsPath == "" {
		sessionsPath = "sessions.json"
	}
	s.sessionMgr = sessionmanager.NewManagerWithPath(sessionsPath)
	if err := s.sessionMgr.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "[MCP] Warning: Failed to load sessions: %v\n", err)
	}

	// 使用现有的 secretstore
	s.secretStore = secretstore.NewKeyringStore()

	// 初始化白名单管理器
	whitelistPath := "command_whitelist.json"
	if config.WhitelistPath != "" {
		whitelistPath = config.WhitelistPath
	}
	whitelistMgr, err := NewWhitelistManager(whitelistPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[MCP] Warning: Failed to load whitelist config: %v, using defaults\n", err)
		// 使用默认配置
		whitelistMgr, _ = NewWhitelistManager("")
		whitelistMgr.config = DefaultWhitelistConfig()
	}
	s.whitelistManager = whitelistMgr

	// 初始化文件访问检查器
	fileAccessPath := "file_access.json"
	if config.FilePath != "" {
		fileAccessPath = config.FilePath
	}
	fileChecker, err := NewFileAccessChecker(fileAccessPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[MCP] Warning: Failed to load file access config: %v, using defaults\n", err)
		fileChecker, _ = NewFileAccessChecker("")
		fileChecker.config = DefaultFileAccessConfig()
	}
	s.fileChecker = fileChecker

	// 设置本地暂存目录并创建
	s.localStagingDir = "/tmp/opscopilot-mcp/"
	if err := EnsureLocalStagingDir(s.localStagingDir); err != nil {
		fmt.Fprintf(os.Stderr, "[MCP] Warning: Failed to create staging dir: %v\n", err)
	}

	return s, nil
}

// GetAvailableServers 获取所有可用的服务器配置
func (s *Server) GetAvailableServers() []*sessionmanager.Session {
	return s.sessionMgr.GetSessions()
}

// Shutdown 关闭服务器
func (s *Server) Shutdown() {
	// 停止清理 goroutine
	close(s.stopChan)

	s.mu.Lock()
	defer s.mu.Unlock()

	// 关闭所有连接
	for name, conn := range s.connections {
		closeSFTP(conn)
		if conn.Client != nil {
			conn.Client.Close()
			fmt.Fprintf(os.Stderr, "[MCP] Closed connection to %s\n", name)
		}
	}
}

// startIdleConnectionCleaner 定期检查并断开空闲连接
func (s *Server) startIdleConnectionCleaner() {
	// 每分钟检查一次
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.cleanIdleConnections()
		}
	}
}

// cleanIdleConnections 清理空闲超时的连接
func (s *Server) cleanIdleConnections() {
	s.mu.Lock()
	defer s.mu.Unlock()

	idleTimeout := time.Duration(s.config.IdleTimeoutMin) * time.Minute
	now := time.Now()

	for name, conn := range s.connections {
		lastActive := time.Unix(0, conn.LastActive.Load())
		idleDuration := now.Sub(lastActive)
		if idleDuration > idleTimeout {
			closeSFTP(conn)
			if conn.Client != nil {
				conn.Client.Close()
			}
			delete(s.connections, name)
			fmt.Fprintf(os.Stderr, "[MCP] Disconnected idle server '%s' (idle for %v)\n", name, idleDuration.Round(time.Second))
		}
	}
}

// JSONRPCRequest JSON-RPC 请求
type JSONRPCRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      interface{}            `json:"id"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params,omitempty"`
}

// JSONRPCResponse JSON-RPC 响应
type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

// RPCError RPC 错误
type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// SendResponse 发送响应
func (s *Server) SendResponse(id interface{}, result interface{}) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	s.sendJSON(resp)
}

// SendError 发送错误响应
func (s *Server) SendError(id interface{}, code int, message string) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: message,
		},
	}
	s.sendJSON(resp)
}

// SendErrorWithData 发送带数据的错误响应
func (s *Server) SendErrorWithData(id interface{}, code int, message string, data interface{}) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	s.sendJSON(resp)
}

// sendJSON 发送 JSON 响应
func (s *Server) sendJSON(v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[MCP] Failed to marshal response: %v\n", err)
		return
	}
	fmt.Println(string(data))
}
