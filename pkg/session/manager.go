package session

import (
	"io"
	"sync"

	"github.com/google/uuid"
	"opscopilot/pkg/remote"
)

// Session 表示一个活跃的远程连接会话。
//
// 重构后只持有 remote.Connection 接口(协议无关),不再感知 *sshclient.Client
// 或 *ssh.Session。Resize 能力由 Conn 自身提供(SSH 实现内部转发到
// session.WindowChange),故无需单独的 SSHSession 字段。
type Session struct {
	ID    string
	Conn  remote.Connection
	Stdin io.WriteCloser
}

type Manager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
	}
}

// Add 注册一个新会话,返回生成的 sessionID。
func (m *Manager) Add(conn remote.Connection, stdin io.WriteCloser) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := uuid.New().String()
	m.sessions[id] = &Session{
		ID:    id,
		Conn:  conn,
		Stdin: stdin,
	}
	return id
}

// AddWithID 以指定 ID 注册会话(用于重连复用同一 sessionID)。
func (m *Manager) AddWithID(id string, conn remote.Connection, stdin io.WriteCloser) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.sessions[id] = &Session{
		ID:    id,
		Conn:  conn,
		Stdin: stdin,
	}
}

func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sess, ok := m.sessions[id]
	return sess, ok
}

func (m *Manager) Remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if sess, ok := m.sessions[id]; ok {
		// 关闭底层连接资源
		if sess.Conn != nil {
			sess.Conn.Close()
		}
		delete(m.sessions, id)
	}
}

func (m *Manager) List() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		list = append(list, s)
	}
	return list
}

func (m *Manager) Broadcast(ids []string, data string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var wg sync.WaitGroup
	payload := []byte(data)

	for _, id := range ids {
		if sess, ok := m.sessions[id]; ok && sess.Stdin != nil {
			wg.Add(1)
			go func(w io.Writer, sid string) {
				defer wg.Done()
				w.Write(payload)
			}(sess.Stdin, id)
		}
	}
	wg.Wait()
}

// Resize 调整指定会话的远端终端尺寸。
// 委托给 Conn.Resize(各协议映射到本协议的尺寸通知机制:
// SSH -> session.WindowChange,Telnet -> IAC SB NAWS)。
// 会话不存在或 Conn 为空时静默忽略,与历史行为一致。
func (m *Manager) Resize(id string, cols, rows int) error {
	m.mu.RLock()
	sess, ok := m.sessions[id]
	m.mu.RUnlock()

	if !ok {
		return nil // Session not found, ignore silently
	}

	if sess.Conn == nil {
		return nil // No connection, ignore
	}

	return sess.Conn.Resize(cols, rows)
}
