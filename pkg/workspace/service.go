// Package workspace coordinates presentation, not SSH state or credentials.
// A connected resource is usable by automation only after the UI acknowledges it.
package workspace

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

var ErrUnavailable = errors.New("工作台未就绪或页面已更换，请打开主窗口后重试")
var ErrStale = errors.New("工作台请求已失效")

type Target struct {
	TabID        string `json:"tabId"`
	ConnectionID string `json:"connectionId"`
}
type Tab struct {
	Target
	Title  string `json:"title"`
	Status string `json:"status"`
	Ready  bool   `json:"ready"`
}
type Snapshot struct {
	Tabs   []Tab  `json:"tabs"`
	Active Target `json:"active"`
}

// Public connection metadata is a whitelist, never a redacted credential struct.
type Endpoint struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	User string `json:"user"`
}
type Connection struct {
	ID    string     `json:"id"`
	Name  string     `json:"name"`
	Route []Endpoint `json:"route"`
}
type Inventory struct {
	Snapshot
	Connections []Connection `json:"connections"`
	ToolTarget  Target       `json:"toolTarget"`
}
type Presentation struct {
	ID     string `json:"id"`
	Action string `json:"action"`
	Tab    Tab    `json:"tab"`
}
type pending struct {
	command Presentation
	done    chan error
}

// The most recently attached desktop page is the presentation controller.
// Old pages cannot publish snapshots or acknowledge the replacement's commands.
type Service struct {
	mu       sync.Mutex
	client   string
	changed  chan struct{}
	seen     time.Time
	snapshot Snapshot
	pending  map[string]*pending
}

func (s *Service) Attach() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.changed != nil {
		close(s.changed)
	}
	s.changed = make(chan struct{})
	s.client = uuid.NewString()
	s.snapshot = Snapshot{Tabs: []Tab{}}
	s.pending = make(map[string]*pending)
	s.seen = time.Now()
	return s.client
}
func (s *Service) Detach(client string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if client == "" || client != s.client {
		return false
	}
	close(s.changed)
	s.changed = nil
	s.client = ""
	s.pending = nil
	s.snapshot = Snapshot{Tabs: []Tab{}}
	return true
}
func (s *Service) valid(client string) bool {
	return client != "" && client == s.client && time.Since(s.seen) < 5*time.Second
}
func (s *Service) Current() (string, Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.valid(s.client) {
		return "", Snapshot{}, ErrUnavailable
	}
	view := s.snapshot
	view.Tabs = append([]Tab{}, view.Tabs...)
	return s.client, view, nil
}
func (s *Service) Check(client string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.valid(client) {
		return ErrUnavailable
	}
	return nil
}
func (s *Service) Pull(client string, view Snapshot) ([]Presentation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if client == "" || client != s.client {
		return nil, ErrStale
	}
	if len(view.Tabs) > 256 {
		return nil, errors.New("工作台页签过多")
	}
	ids := map[string]bool{}
	for _, tab := range view.Tabs {
		if tab.TabID == "" || len(tab.TabID) > 128 || len(tab.ConnectionID) > 128 || len(tab.Title) > 512 || ids[tab.TabID] {
			return nil, errors.New("工作台快照无效")
		}
		ids[tab.TabID] = true
	}
	s.seen = time.Now()
	view.Tabs = append([]Tab{}, view.Tabs...)
	s.snapshot = view
	commands := make([]Presentation, 0, len(s.pending))
	for _, p := range s.pending {
		commands = append(commands, p.command)
	}
	return commands, nil
}
func (s *Service) Acknowledge(client, id string, success bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.pending[id]
	if !s.valid(client) || p == nil {
		return ErrStale
	}
	if success {
		found := false
		for _, tab := range s.snapshot.Tabs {
			if tab.Target == p.command.Tab.Target && tab.Ready && tab.Status == "connected" {
				found = true
			}
		}
		if !found || s.snapshot.Active != p.command.Tab.Target {
			return errors.New("目标终端尚未呈现或实际连接已变化")
		}
	}
	delete(s.pending, id)
	var err error
	if !success {
		err = errors.New("页面未能呈现目标终端")
	}
	p.done <- err
	return nil
}
func (s *Service) Present(ctx context.Context, client, action string, tab Tab) error {
	if action != "open" && action != "select" {
		return errors.New("未知工作台操作")
	}
	s.mu.Lock()
	if ctx.Err() != nil || !s.valid(client) {
		s.mu.Unlock()
		return ErrUnavailable
	}
	p := &pending{command: Presentation{ID: uuid.NewString(), Action: action, Tab: tab}, done: make(chan error, 1)}
	s.pending[p.command.ID] = p
	changed := s.changed
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		if s.pending[p.command.ID] == p {
			delete(s.pending, p.command.ID)
		}
		s.mu.Unlock()
	}()
	timer := time.NewTimer(12 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-changed:
		return ErrUnavailable
	case <-timer.C:
		return errors.New("等待页签呈现超时，请确认主窗口仍在运行")
	case err := <-p.done:
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
}
