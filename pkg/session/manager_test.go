package session

import (
	"context"
	"io"
	"sync"
	"testing"

	"opscopilot/pkg/remote"
)

// mockConn 是 remote.Connection 的测试桩,不依赖任何真实协议实现。
type mockConn struct {
	mu       sync.Mutex
	resized  bool
	closed   bool
	lastCols int
	lastRows int
}

func (m *mockConn) StartShell(cols, rows int) (io.WriteCloser, io.Reader, error) {
	return nil, nil, nil
}
func (m *mockConn) Resize(cols, rows int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resized = true
	m.lastCols = cols
	m.lastRows = rows
	return nil
}
func (m *mockConn) Run(ctx context.Context, cmd string) (string, error) { return "", nil }
func (m *mockConn) Healthy() bool                                       { return true }
func (m *mockConn) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}
func (m *mockConn) Protocol() string { return remote.ProtocolSSH }

// 编译期断言:确保 mockConn 实现 remote.Connection。
var _ remote.Connection = (*mockConn)(nil)

type MockWriter struct {
	data []byte
	mu   sync.Mutex
}

func (w *MockWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.data = append(w.data, p...)
	return len(p), nil
}

func (w *MockWriter) Close() error { return nil }

func TestSessionManager(t *testing.T) {
	manager := NewManager()

	// Test Add
	sessionID := manager.Add(&mockConn{}, &MockWriter{})
	if sessionID == "" {
		t.Error("Expected session ID, got empty string")
	}

	// Test Get
	sess, ok := manager.Get(sessionID)
	if !ok {
		t.Error("Expected session to exist")
	}
	if sess.ID != sessionID {
		t.Errorf("Expected session ID %s, got %s", sessionID, sess.ID)
	}
	if sess.Conn == nil {
		t.Error("Expected session Conn to be set")
	}

	// Test Resize 委托到 Conn.Resize
	if err := manager.Resize(sessionID, 100, 40); err != nil {
		t.Errorf("Resize returned error: %v", err)
	}
	mc := sess.Conn.(*mockConn)
	mc.mu.Lock()
	if !mc.resized || mc.lastCols != 100 || mc.lastRows != 40 {
		t.Errorf("Resize not delegated correctly: resized=%v cols=%d rows=%d", mc.resized, mc.lastCols, mc.lastRows)
	}
	mc.mu.Unlock()

	// Test List
	list := manager.List()
	if len(list) != 1 {
		t.Errorf("Expected 1 session, got %d", len(list))
	}

	// Test Remove
	manager.Remove(sessionID)
	_, ok = manager.Get(sessionID)
	if ok {
		t.Error("Expected session to be removed")
	}
}

func TestBroadcast(t *testing.T) {
	manager := NewManager()

	// Create 3 sessions
	writers := []*MockWriter{
		{}, {}, {},
	}

	ids := []string{}
	for _, w := range writers {
		id := manager.Add(&mockConn{}, w)
		ids = append(ids, id)
	}

	// Broadcast to all
	msg := "echo hello\n"
	manager.Broadcast(ids, msg)

	// Verify all writers received the message
	for i, w := range writers {
		w.mu.Lock()
		got := string(w.data)
		w.mu.Unlock()

		if got != msg {
			t.Errorf("Session %d: expected %q, got %q", i, msg, got)
		}
	}
}
