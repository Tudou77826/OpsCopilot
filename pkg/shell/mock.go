// Package shell 定义 Shell 组件的核心接口和类型
package shell

import (
	"context"
	"io"
	"sync"
)

// MockSession Mock 会话实现
type MockSession struct {
	mu       sync.RWMutex
	id       string
	info     SessionInfo
	closed   bool
	dataSent []string
	stdin    *mockWriter
}

type mockWriter struct {
	data []byte
}

func (w *mockWriter) Write(p []byte) (n int, err error) {
	w.data = append(w.data, p...)
	return len(p), nil
}

// NewMockSession 创建 Mock 会话
func NewMockSession(id string, info SessionInfo) *MockSession {
	return &MockSession{
		id:    id,
		info:  info,
		stdin: &mockWriter{},
	}
}

// ID 返回会话 ID
func (s *MockSession) ID() string {
	return s.id
}

// Info 返回会话信息
func (s *MockSession) Info() SessionInfo {
	return s.info
}

// Send 发送数据
func (s *MockSession) Send(data string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dataSent = append(s.dataSent, data)
	return nil
}

// Resize 调整大小
func (s *MockSession) Resize(cols, rows int) error {
	return nil
}

// Close 关闭会话
func (s *MockSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

// Stdin 返回标准输入
func (s *MockSession) Stdin() io.Writer {
	return s.stdin
}

// IsClosed 检查是否关闭
func (s *MockSession) IsClosed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.closed
}

// GetDataSent 获取已发送的数据
func (s *MockSession) GetDataSent() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dataSent
}

// MockManager Mock 会话管理器
type MockManager struct {
	mu       sync.RWMutex
	sessions map[string]*MockSession
	connectError error
}

// NewMockManager 创建 Mock 管理器
func NewMockManager() *MockManager {
	return &MockManager{
		sessions: make(map[string]*MockSession),
	}
}

// Connect 建立连接
func (m *MockManager) Connect(ctx context.Context, config ConnectConfig) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.connectError != nil {
		return nil, m.connectError
	}

	info := SessionInfo{
		ID:     config.ID,
		Host:   config.Host,
		User:   config.User,
		Active: true,
	}

	session := NewMockSession(config.ID, info)
	m.sessions[config.ID] = session
	return session, nil
}

// Get 获取会话
func (m *MockManager) Get(id string) (Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sess, ok := m.sessions[id]
	if !ok {
		return nil, false
	}
	return sess, true
}

// List 列出所有会话
func (m *MockManager) List() []Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]Session, 0, len(m.sessions))
	for _, sess := range m.sessions {
		result = append(result, sess)
	}
	return result
}

// Disconnect 断开会话
func (m *MockManager) Disconnect(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if sess, ok := m.sessions[id]; ok {
		sess.Close()
		delete(m.sessions, id)
	}
	return nil
}

// DisconnectAll 断开所有会话
func (m *MockManager) DisconnectAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, sess := range m.sessions {
		sess.Close()
	}
	m.sessions = make(map[string]*MockSession)
	return nil
}

// Broadcast 广播数据
func (m *MockManager) Broadcast(ids []string, data string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, id := range ids {
		if sess, ok := m.sessions[id]; ok {
			sess.Send(data)
		}
	}
	return nil
}

// SetConnectError 设置连接错误（用于测试）
func (m *MockManager) SetConnectError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connectError = err
}

// SessionCount 返回会话数量
func (m *MockManager) SessionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}
