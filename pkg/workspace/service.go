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
var ErrInactive = errors.New("此页面不是当前轮次的操作页面")

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
	Tabs            []Tab  `json:"tabs"`
	Active          Target `json:"active"`
	ActiveWorkspace string `json:"activeWorkspace,omitempty"`
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

type page struct {
	key  string
	view Snapshot
	seen time.Time
}

// Pages register independently. Only explicit activation by an accepted
// assistant submission chooses the presentation controller for that turn.
type Service struct {
	mu       sync.Mutex
	client   string
	changed  chan struct{}
	seen     time.Time
	snapshot Snapshot
	pending  map[string]*pending
	pages    map[string]*page
}

func (s *Service) Attach() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Legacy single-view hosts explicitly replace their previous controller.
	s.pages = map[string]*page{}
	id := uuid.NewString()
	s.pages[id] = &page{seen: time.Now(), view: Snapshot{Tabs: []Tab{}}}
	s.activateLocked(id)
	return id
}

func (s *Service) activateLocked(client string) {
	if s.changed != nil {
		close(s.changed)
	}
	s.changed = make(chan struct{})
	s.client = client
	s.snapshot = s.pages[client].view
	s.pending = make(map[string]*pending)
	s.seen = s.pages[client].seen
}

// RegisterPage replaces only the same browser tab across reloads. A different
// native/WebView/browser window cannot steal an executing turn by attaching.
func (s *Service) RegisterPage(key string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if key == "" || len(key) > 128 {
		return "", false, errors.New("页面标识无效")
	}
	if s.pages == nil {
		s.pages = map[string]*page{}
	}
	replaced := false
	for id, p := range s.pages {
		if p.key == key {
			replaced = replaced || id == s.client
			delete(s.pages, id)
		} else if id != s.client && time.Since(p.seen) > time.Minute {
			delete(s.pages, id)
		}
	}
	if len(s.pages) >= 32 {
		return "", false, errors.New("打开的应用页面过多，请关闭不用的页面")
	}
	id := uuid.NewString()
	s.pages[id] = &page{key: key, seen: time.Now(), view: Snapshot{Tabs: []Tab{}}}
	if s.client == "" || replaced {
		s.activateLocked(id)
	}
	return id, replaced, nil
}
func (s *Service) Activate(client string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.pages[client]
	if p == nil || time.Since(p.seen) >= 5*time.Second {
		return ErrStale
	}
	if s.client != client {
		s.activateLocked(client)
	}
	return nil
}
func (s *Service) Registered(client string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.pages[client]
	if client == "" || p == nil || time.Since(p.seen) >= 5*time.Second {
		return ErrStale
	}
	return nil
}
func (s *Service) Detach(client string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if client == "" || s.pages[client] == nil {
		return false
	}
	delete(s.pages, client)
	if client != s.client {
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
		if s.pages[client] != nil {
			return ErrInactive
		}
		return ErrStale
	}
	return nil
}
func (s *Service) Pull(client string, view Snapshot) ([]Presentation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if client == "" || s.pages[client] == nil {
		return nil, ErrStale
	}
	if len(view.Tabs) > 256 || len(view.ActiveWorkspace) > 128 {
		return nil, errors.New("工作台页签过多")
	}
	ids := map[string]bool{}
	for _, tab := range view.Tabs {
		if tab.TabID == "" || len(tab.TabID) > 128 || len(tab.ConnectionID) > 128 || len(tab.Title) > 512 || ids[tab.TabID] {
			return nil, errors.New("工作台快照无效")
		}
		ids[tab.TabID] = true
	}
	view.Tabs = append([]Tab{}, view.Tabs...)
	p := s.pages[client]
	p.seen = time.Now()
	p.view = view
	if client != s.client {
		return []Presentation{}, nil
	}
	s.seen = p.seen
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
