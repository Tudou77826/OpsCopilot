package session

import (
	"opscopilot/pkg/sshclient"
	"sync"
	"testing"
)

// MockClient 模拟 sshclient.Client
type MockClient struct{}

func (m *MockClient) Close() error { return nil }

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
	sessionID := manager.Add(&sshclient.Client{}, &MockWriter{}, nil)
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
		id := manager.Add(&sshclient.Client{}, w, nil)
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
