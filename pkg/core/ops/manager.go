package ops

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/sftp"
	"opscopilot/pkg/core/security"
	"opscopilot/pkg/remote"
	"opscopilot/pkg/secretstore"
	"opscopilot/pkg/sessionmanager"
)

// Config 运维内核配置
type Config struct {
	SessionsFile   string // sessions.json 路径
	WhitelistPath  string // 白名单配置文件路径
	FilePath       string // 文件访问控制配置路径
	MaxTotalBytes  int
	MaxLineLength  int
	HeadLines      int
	IdleTimeoutMin int
}

// Manager 运维内核管理器 —— GUI 和 CLI 共享的统一运维能力入口
// 所有非交互式访问（CLI、未来任何协议）都必须通过本管理器执行，
// 它内置 security 闸门（命令白名单 + 文件访问控制），调用方无法绕过。
type Manager struct {
	config           *Config
	sessionMgr       *sessionmanager.Manager
	secretStore      secretstore.SecretStore
	connections      map[string]*Connection
	whitelistManager *security.WhitelistManager
	fileChecker      *security.FileAccessChecker
	mu               sync.RWMutex
	stopChan         chan struct{}
}

// Connection 远程连接(协议无关)。
//
// 重构后持有 remote.Connection 接口,可承载 SSH/Telnet 等多协议。
// 命令执行、健康检查、关闭等能力通过 Conn 直接调用(各协议实现等价语义);
// SFTP 等协议特有能力通过类型断言 remote.SFTPCapable 查询。
type Connection struct {
	Name          string
	Host          string // 服务器 IP 地址（用于白名单匹配）
	Protocol      string // 协议标识(remote.ProtocolSSH / remote.ProtocolTelnet)
	Conn          remote.Connection
	RootPassword  string // 用于 sudo 提权
	ConnectedAt   time.Time
	LastActive    atomic.Int64 // Unix 纳秒时间戳，支持并发读写
	sftpMu        sync.Mutex   // 保护 SFTP 客户端的并发访问
	sftpClient    *sftp.Client // 缓存的 SFTP 客户端
	sftpTested    bool         // 是否已检测过 SFTP 可用性
	sftpAvailable bool         // SFTP 是否可用
}

// NewManager 创建运维内核管理器
func NewManager(config *Config) (*Manager, error) {
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

	m := &Manager{
		config:      config,
		connections: make(map[string]*Connection),
		stopChan:    make(chan struct{}),
	}

	// 启动连接空闲超时清理 goroutine
	go m.startIdleConnectionCleaner()

	// 使用现有的 sessionmanager 加载 sessions.json
	sessionsPath := config.SessionsFile
	if sessionsPath == "" {
		sessionsPath = "sessions.json"
	}
	m.sessionMgr = sessionmanager.NewManagerWithPath(sessionsPath)
	if err := m.sessionMgr.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "[ops] Warning: Failed to load sessions: %v\n", err)
	}

	// 使用现有的 secretstore
	m.secretStore = secretstore.NewKeyringStore()

	// 初始化白名单管理器
	whitelistPath := "command_whitelist.json"
	if config.WhitelistPath != "" {
		whitelistPath = config.WhitelistPath
	}
	whitelistMgr, err := security.NewWhitelistManager(whitelistPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ops] Warning: Failed to load whitelist config: %v, using defaults\n", err)
		whitelistMgr, _ = security.NewWhitelistManager("")
		_ = whitelistMgr.UpdateConfig(security.DefaultWhitelistConfig())
	}
	m.whitelistManager = whitelistMgr

	// 初始化文件访问检查器
	fileAccessPath := "file_access.json"
	if config.FilePath != "" {
		fileAccessPath = config.FilePath
	}
	fileChecker, err := security.NewFileAccessChecker(fileAccessPath)
	if err != nil {
		return nil, fmt.Errorf("加载文件访问配置失败: %w", err)
	}
	m.fileChecker = fileChecker

	return m, nil
}

// GetAvailableServers 获取所有可用的服务器配置
func (m *Manager) GetAvailableServers() []*sessionmanager.Session {
	return m.sessionMgr.GetSessions()
}

// Shutdown 关闭管理器
func (m *Manager) Shutdown() {
	close(m.stopChan)

	m.mu.Lock()
	defer m.mu.Unlock()

	for name, conn := range m.connections {
		closeSFTP(conn)
		if conn.Conn != nil {
			conn.Conn.Close()
			fmt.Fprintf(os.Stderr, "[ops] Closed connection to %s\n", name)
		}
	}
}

// startIdleConnectionCleaner 定期检查并断开空闲连接
func (m *Manager) startIdleConnectionCleaner() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopChan:
			return
		case <-ticker.C:
			m.cleanIdleConnections()
		}
	}
}

// cleanIdleConnections 清理空闲超时的连接
func (m *Manager) cleanIdleConnections() {
	m.mu.Lock()
	defer m.mu.Unlock()

	idleTimeout := time.Duration(m.config.IdleTimeoutMin) * time.Minute
	now := time.Now()

	for name, conn := range m.connections {
		lastActive := time.Unix(0, conn.LastActive.Load())
		idleDuration := now.Sub(lastActive)
		if idleDuration > idleTimeout {
			closeSFTP(conn)
			if conn.Conn != nil {
				conn.Conn.Close()
			}
			delete(m.connections, name)
			fmt.Fprintf(os.Stderr, "[ops] Disconnected idle server '%s' (idle for %v)\n", name, idleDuration.Round(time.Second))
		}
	}
}
