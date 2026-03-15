package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"opscopilot/pkg/ai"
	"opscopilot/pkg/config"
	"opscopilot/pkg/llm"
	"opscopilot/pkg/recorder"
	"opscopilot/pkg/secretstore"
	"opscopilot/pkg/sessionmanager"
	"opscopilot/pkg/sshclient"
)

// Config MCP Server 配置
type Config struct {
	SessionsFile   string // sessions.json 路径
	RecordingsDir  string // 录制文件存储目录
	KnowledgeDir   string // 知识库目录（用于归档排查经验）
	WhitelistPath  string // 白名单配置文件路径（可选）
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
	llmChecker       *LLMChecker              // LLM 风险检查器（新）
	recorder         *recorder.Recorder       // 复用主程序录制器
	mcpRecorder      *MCPRecorderAdapter      // MCP 适配器
	aiService        *ai.AIService            // AI 服务（用于 get_hints）
	mu               sync.RWMutex
	stopChan         chan struct{}            // 停止清理 goroutine 的信号
}

// Connection SSH 连接
type Connection struct {
	Name         string
	Host         string            // 服务器 IP 地址（用于白名单匹配）
	Client       *sshclient.Client
	RootPassword string // 用于 sudo 提权
	ConnectedAt  time.Time
	LastActive   atomic.Int64 // Unix 纳秒时间戳，支持并发读写
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
		recorder:    recorder.NewRecorder(config.RecordingsDir),
		stopChan:    make(chan struct{}),
	}

	// 创建 MCP 适配器（传入知识库目录用于归档）
	s.mcpRecorder = NewMCPRecorderAdapter(s.recorder, config.KnowledgeDir)

	// 启动连接空闲超时清理 goroutine
	go s.startIdleConnectionCleaner()

	// 使用现有的 sessionmanager 加载 sessions.json
	s.sessionMgr = sessionmanager.NewManager()
	if config.SessionsFile != "" {
		// 如果指定了路径，需要修改 manager 的 filePath
		// sessionmanager 没有提供设置路径的方法，这里直接加载
		data, err := os.ReadFile(config.SessionsFile)
		if err != nil {
			if !os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "[MCP] Warning: Failed to read sessions file: %v\n", err)
			}
		} else {
			if err := json.Unmarshal(data, &s.sessionMgr.Sessions); err != nil {
				fmt.Fprintf(os.Stderr, "[MCP] Warning: Failed to parse sessions file: %v\n", err)
			}
		}
	} else {
		if err := s.sessionMgr.Load(); err != nil {
			fmt.Fprintf(os.Stderr, "[MCP] Warning: Failed to load sessions: %v\n", err)
		}
	}

	// 使用现有的 secretstore
	s.secretStore = secretstore.NewKeyringStore()

	// 初始化 AI 服务（复用 OpsCopilot 的配置）
	if err := s.initAIService(); err != nil {
		fmt.Fprintf(os.Stderr, "[MCP] Warning: Failed to init AI service: %v\n", err)
		// 不阻止启动，只是 get_hints 功能降级
	}

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

	return s, nil
}

// initAIService 初始化 AI 服务
func (s *Server) initAIService() error {
	// 1. 加载 config.json
	configMgr := config.NewManager()
	if err := configMgr.Load(); err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// 2. 检查 API Key 是否配置
	llmCfg := configMgr.Config.LLM
	if llmCfg.APIKey == "" {
		return fmt.Errorf("LLM API Key not configured")
	}

	// 3. 创建 LLM Provider
	fastProvider := llm.NewOpenAIProvider(llmCfg.APIKey, llmCfg.BaseURL, llmCfg.FastModel)
	complexProvider := llm.NewOpenAIProvider(llmCfg.APIKey, llmCfg.BaseURL, llmCfg.ComplexModel)

	// 4. 创建 AIService
	s.aiService = ai.NewAIService(fastProvider, complexProvider, configMgr)

	// 5. 初始化 LLM 检查器（使用 fast provider）
	s.llmChecker = NewLLMChecker(fastProvider)

	// 6. 设置空的事件发射器（MCP Server 不需要 UI 事件，避免 Wails runtime 调用）
	ai.SetEventEmitter(func(ctx context.Context, optionalData string, optionalData2 ...interface{}) {
		// No-op for MCP server
	})

	return nil
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
