// Package shell 定义 Shell 组件的核心接口和类型
package shell

import (
	"context"
	"fmt"
	"io"
	"sync"

	"opscopilot/pkg/bridge"
	"opscopilot/pkg/session"
	"opscopilot/pkg/sshclient"
)

// DefaultTerminalSize 默认终端大小
const (
	DefaultCols = 120
	DefaultRows = 40
)

// ManagerImpl Shell 管理器实现
type ManagerImpl struct {
	mu          sync.RWMutex
	sessionMgr  *session.Manager
	bus         *bridge.BridgeImpl
	onConnect   []func(sessionID string, config ConnectConfig)
	onDisconnect []func(sessionID string, reason string)
}

// ManagerOption 管理器选项
type ManagerOption func(*ManagerImpl)

// NewManager 创建 Shell 管理器
func NewManager(opts ...ManagerOption) *ManagerImpl {
	m := &ManagerImpl{
		sessionMgr: session.NewManager(),
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// WithBus 设置事件总线
func WithBus(bus *bridge.BridgeImpl) ManagerOption {
	return func(m *ManagerImpl) {
		m.bus = bus
	}
}

// WithSessionManager 设置会话管理器
func WithSessionManager(sm *session.Manager) ManagerOption {
	return func(m *ManagerImpl) {
		m.sessionMgr = sm
	}
}

// Connect 建立新的 SSH 会话
func (m *ManagerImpl) Connect(ctx context.Context, config ConnectConfig) (Session, error) {
	// 创建 SSH 客户端配置
	clientConfig := &sshclient.ConnectConfig{
		Name:     config.Name,
		Host:     config.Host,
		Port:     config.Port,
		User:     config.User,
		Password: config.Password,
	}

	// 设置堡垒机配置
	if config.Bastion != nil {
		clientConfig.Bastion = &sshclient.ConnectConfig{
			Host:     config.Bastion.Host,
			Port:     config.Bastion.Port,
			User:     config.Bastion.User,
			Password: config.Bastion.Password,
		}
	}

	// 连接 SSH
	client, err := sshclient.NewClient(clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH client: %w", err)
	}

	// 创建 Shell 会话
	sshSession, stdin, _, err := client.StartShell(DefaultCols, DefaultRows)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to start shell: %w", err)
	}

	// 添加到会话管理器
	sessionID := m.sessionMgr.Add(client, stdin, sshSession)

	// 发布连接事件
	if m.bus != nil {
		_ = m.bus.PublishShellEvent(ctx, bridge.EventShellConnect, sessionID, bridge.ShellConnectPayload{
			Config: bridge.ConnectConfigPayload{
				ID:   config.ID,
				Name: config.Name,
				Host: config.Host,
				Port: config.Port,
				User: config.User,
			},
		})
	}

	// 调用连接回调
	m.mu.RLock()
	callbacks := make([]func(string, ConnectConfig), len(m.onConnect))
	copy(callbacks, m.onConnect)
	m.mu.RUnlock()

	for _, cb := range callbacks {
		cb(sessionID, config)
	}

	return &sessionWrapper{
		id:        sessionID,
		session:   &session.Session{ID: sessionID, Client: client, Stdin: stdin, SSHSession: sshSession},
		manager:   m,
		host:      config.Host,
		user:      config.User,
	}, nil
}

// Get 获取指定 ID 的会话
func (m *ManagerImpl) Get(id string) (Session, bool) {
	sess, ok := m.sessionMgr.Get(id)
	if !ok {
		return nil, false
	}
	return &sessionWrapper{
		id:      id,
		session: sess,
		manager: m,
	}, true
}

// List 列出所有活跃会话
func (m *ManagerImpl) List() []Session {
	sessions := m.sessionMgr.List()
	result := make([]Session, len(sessions))
	for i, s := range sessions {
		result[i] = &sessionWrapper{
			id:      s.ID,
			session: s,
			manager: m,
		}
	}
	return result
}

// Disconnect 断开指定会话
func (m *ManagerImpl) Disconnect(id string) error {
	m.sessionMgr.Remove(id)

	// 发布断开事件
	if m.bus != nil {
		_ = m.bus.PublishShellEvent(context.Background(), bridge.EventShellDisconnect, id, bridge.ShellDisconnectPayload{
			Reason: "user_disconnect",
		})
	}

	// 调用断开回调
	m.mu.RLock()
	callbacks := make([]func(string, string), len(m.onDisconnect))
	copy(callbacks, m.onDisconnect)
	m.mu.RUnlock()

	for _, cb := range callbacks {
		cb(id, "user_disconnect")
	}

	return nil
}

// DisconnectAll 断开所有会话
func (m *ManagerImpl) DisconnectAll() error {
	sessions := m.sessionMgr.List()
	for _, s := range sessions {
		m.Disconnect(s.ID)
	}
	return nil
}

// Broadcast 向多个会话广播数据
func (m *ManagerImpl) Broadcast(ids []string, data string) error {
	m.sessionMgr.Broadcast(ids, data)
	return nil
}

// SendCommand 发送命令
func (m *ManagerImpl) SendCommand(sessionID string, command string) error {
	sess, ok := m.sessionMgr.Get(sessionID)
	if !ok || sess.Stdin == nil {
		return fmt.Errorf("session not found or not ready: %s", sessionID)
	}

	_, err := sess.Stdin.Write([]byte(command))
	return err
}

// OnConnect 注册连接回调
func (m *ManagerImpl) OnConnect(callback func(sessionID string, config ConnectConfig)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onConnect = append(m.onConnect, callback)
}

// OnDisconnect 注册断开回调
func (m *ManagerImpl) OnDisconnect(callback func(sessionID string, reason string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onDisconnect = append(m.onDisconnect, callback)
}

// GetSessionManager 获取底层会话管理器（用于兼容）
func (m *ManagerImpl) GetSessionManager() *session.Manager {
	return m.sessionMgr
}

// sessionWrapper 会话包装器
type sessionWrapper struct {
	id      string
	session *session.Session
	manager *ManagerImpl
	host    string
	user    string
}

// ID 返回会话 ID
func (s *sessionWrapper) ID() string {
	return s.id
}

// Info 返回会话信息
func (s *sessionWrapper) Info() SessionInfo {
	return SessionInfo{
		ID:     s.id,
		Host:   s.host,
		User:   s.user,
		Active: true,
	}
}

// Send 发送数据
func (s *sessionWrapper) Send(data string) error {
	if s.session.Stdin == nil {
		return fmt.Errorf("session stdin not available")
	}
	_, err := s.session.Stdin.Write([]byte(data))
	return err
}

// Resize 调整大小
func (s *sessionWrapper) Resize(cols, rows int) error {
	return s.manager.sessionMgr.Resize(s.id, cols, rows)
}

// Close 关闭会话
func (s *sessionWrapper) Close() error {
	return s.manager.Disconnect(s.id)
}

// Stdin 返回标准输入
func (s *sessionWrapper) Stdin() io.Writer {
	return s.session.Stdin
}
