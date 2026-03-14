package mcpserver

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

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
	MaxTotalBytes  int
	MaxLineLength  int
	HeadLines      int
	IdleTimeoutMin int
}

// Server MCP Server 实现 - 复用 OpsCopilot 现有能力
type Server struct {
	config        *Config
	sessionMgr    *sessionmanager.Manager  // 复用现有的 session 管理器
	secretStore   secretstore.SecretStore  // 复用现有的密码存储
	connections   map[string]*Connection   // 活跃的 SSH 连接
	checker       *CommandChecker
	recorder      *recorder.Recorder       // 复用主程序录制器
	mcpRecorder   *MCPRecorderAdapter      // MCP 适配器
	mu            sync.RWMutex
}

// Connection SSH 连接
type Connection struct {
	Name         string
	Client       *sshclient.Client
	RootPassword string // 用于 sudo 提权
	ConnectedAt  time.Time
	LastActive   time.Time
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
		config.IdleTimeoutMin = 30
	}

	s := &Server{
		config:      config,
		connections: make(map[string]*Connection),
		checker:     NewCommandChecker(),
		recorder:    recorder.NewRecorder(config.RecordingsDir),
	}

	// 创建 MCP 适配器（传入知识库目录用于归档）
	s.mcpRecorder = NewMCPRecorderAdapter(s.recorder, config.KnowledgeDir)

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

	return s, nil
}

// GetAvailableServers 获取所有可用的服务器配置
func (s *Server) GetAvailableServers() []*sessionmanager.Session {
	return s.sessionMgr.GetSessions()
}

// Shutdown 关闭服务器
func (s *Server) Shutdown() {
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
