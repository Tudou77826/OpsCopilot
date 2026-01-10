package session

import (
	"io"
	"opscopilot/pkg/sshclient"
	"sync"

	"github.com/google/uuid"
)

type Session struct {
	ID     string
	Client *sshclient.Client
	Stdin  io.WriteCloser
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

func (m *Manager) Add(client *sshclient.Client, stdin io.WriteCloser) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := uuid.New().String()
	m.sessions[id] = &Session{
		ID:     id,
		Client: client,
		Stdin:  stdin,
	}
	return id
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
		// Ensure resources are closed
		if sess.Client != nil {
			sess.Client.Close()
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
