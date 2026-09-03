package sessionmanager

import (
	"encoding/json"
	"fmt"
	"opscopilot/pkg/filetxn"
	"opscopilot/pkg/remote"
	"strings"
	"sync"

	"github.com/google/uuid"
)

type SessionType string

const (
	TypeFolder  SessionType = "folder"
	TypeSession SessionType = "session"
)

type Session struct {
	ID       string                `json:"id"`
	Name     string                `json:"name"` // Display name; defaults to Host
	Type     SessionType           `json:"type"`
	Children []*Session            `json:"children,omitempty"` // For folders
	Config   *remote.ConnectConfig `json:"config,omitempty"`   // For sessions
}

type Manager struct {
	filePath            string
	Sessions            []*Session
	mu                  sync.RWMutex
	baseline            []byte
	PreserveCredentials bool
}

func NewManager() *Manager {
	return &Manager{
		filePath: "sessions.json",
		Sessions: []*Session{},
	}
}

// NewManagerWithPath 创建指定文件路径的 session 管理器
func NewManagerWithPath(filePath string) *Manager {
	return &Manager{
		filePath: filePath,
		Sessions: []*Session{},
	}
}

func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.reload()
}
func (m *Manager) reload() error {
	data, err := filetxn.Read(m.filePath)
	if err != nil {
		return err
	}
	next := []*Session{}
	if len(data) > 0 {
		if err = json.Unmarshal(data, &next); err != nil {
			return err
		}
	}
	m.Sessions = next
	m.baseline = append([]byte(nil), data...)
	return nil
}
func (m *Manager) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, err := json.MarshalIndent(m.Sessions, "", "  ")
	if err != nil {
		return err
	}
	merged, err := filetxn.Merge(m.filePath, m.baseline, data)
	if err != nil {
		return err
	}
	m.baseline = merged
	return json.Unmarshal(merged, &m.Sessions)
}
func (m *Manager) saveLocked() error {
	data, err := json.MarshalIndent(m.Sessions, "", "  ")
	if err != nil {
		return err
	}
	if err = filetxn.Write(m.filePath, data); err != nil {
		return err
	}
	m.baseline = data
	return nil
}
func (m *Manager) beginMutation() (func(), error) {
	release, err := filetxn.Lock(m.filePath)
	if err != nil {
		return nil, err
	}
	if err = m.reload(); err != nil {
		release()
		return nil, err
	}
	return release, nil
}

func (m *Manager) GetSessions() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	data, _ := json.Marshal(m.Sessions)
	var out []*Session
	_ = json.Unmarshal(data, &out)
	return out
}

func (m *Manager) DeleteSession(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	release, err := m.beginMutation()
	if err != nil {
		return err
	}
	defer release()

	// Recursive deletion helper
	var deleteNode func(nodes []*Session) []*Session
	deleteNode = func(nodes []*Session) []*Session {
		var result []*Session
		for _, node := range nodes {
			if node.ID == id {
				continue // Skip (delete)
			}
			if node.Type == TypeFolder {
				node.Children = deleteNode(node.Children)
			}
			result = append(result, node)
		}
		return result
	}

	m.Sessions = deleteNode(m.Sessions)
	return m.saveLocked()
}

func (m *Manager) RenameSession(id, newName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	release, err := m.beginMutation()
	if err != nil {
		return err
	}
	defer release()

	var renameNode func(nodes []*Session) bool
	renameNode = func(nodes []*Session) bool {
		for _, node := range nodes {
			if node.ID == id {
				node.Name = newName
				if node.Config != nil {
					node.Config.Name = newName
				}
				return true
			}
			if node.Type == TypeFolder {
				if renameNode(node.Children) {
					return true
				}
			}
		}
		return false
	}

	if renameNode(m.Sessions) {
		return m.saveLocked()
	}
	return fmt.Errorf("session not found")
}

// Upsert adds or updates a session.
// If groupName is provided, it puts it in that folder.
// Naming rule: Use the configured display name, falling back to Host.
// sameEndpoint 判断两个连接配置是否指向同一远程端点。
// key 为 (Host, Port, Protocol) 三元组。Protocol 空值归一化为 SSH,
// 保证老数据(无 Protocol 字段)与新数据比较一致。
// 用于 Upsert/UpdateSession 的去重,允许同 host 不同协议/端口共存。
func sameEndpoint(a, b *remote.ConnectConfig) bool {
	if a == nil || b == nil {
		return false
	}
	ap := a.Protocol
	if ap == "" {
		ap = remote.ProtocolSSH
	}
	bp := b.Protocol
	if bp == "" {
		bp = remote.ProtocolSSH
	}
	return a.Host == b.Host && a.Port == b.Port && ap == bp
}

// Upsert adds or updates a session.
// If groupName is provided, it puts it in that folder.
// Naming rule: Use the configured display name, falling back to Host.
//
// 去重 key 为 (Host, Port, Protocol) 三元组:同一 host 不同协议/端口可共存,
// 例如 192.168.1.1:22(SSH) 与 192.168.1.1:23(Telnet) 不冲突。
// Protocol 空值在比较前归一化为 SSH,保证一致。
func (m *Manager) Upsert(config remote.ConnectConfig, groupName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	release, err := m.beginMutation()
	if err != nil {
		return err
	}
	defer release()

	// 归一化 Protocol:空值视为 SSH,保证去重比较一致。
	if config.Protocol == "" {
		config.Protocol = remote.ProtocolSSH
	}

	// Step 1: Remove existing session with same (Host, Port, Protocol) from anywhere in the tree.
	// This handles "Update" (by removing old and adding new) and "Move" (if group changed).
	var removed *Session
	var removeByHost func(nodes []*Session) []*Session
	removeByHost = func(nodes []*Session) []*Session {
		var result []*Session
		for _, node := range nodes {
			if node.Type == TypeSession && node.Config != nil && sameEndpoint(node.Config, &config) {
				removed = node
				continue // Remove
			}
			if node.Type == TypeFolder {
				node.Children = removeByHost(node.Children)
			}
			result = append(result, node)
		}
		return result
	}
	m.Sessions = removeByHost(m.Sessions)

	if m.PreserveCredentials && removed != nil {
		preserveCredentials(&config, removed.Config)
	}
	// Connections opened from the saved-session tree carry the stored config.
	// Preserve the node's display name when that config still has the old/empty name.
	targetName := config.Name
	if removed != nil && removed.Name != "" {
		oldConfigName := ""
		if removed.Config != nil {
			oldConfigName = removed.Config.Name
		}
		if targetName == "" || targetName == oldConfigName {
			targetName = removed.Name
		}
	}
	if targetName == "" {
		targetName = config.Host
	}
	config.Name = targetName

	// Step 2: Create new node
	newNode := &Session{
		ID:     "",
		Name:   targetName,
		Type:   TypeSession,
		Config: &config,
	}
	if removed != nil {
		newNode.ID = removed.ID
	}
	if newNode.ID == "" {
		newNode.ID = uuid.New().String()
	}

	// Step 3: Add to target
	if groupName != "" {
		// Find folder
		found := false
		for _, node := range m.Sessions {
			if node.Type == TypeFolder && node.Name == groupName {
				node.Children = append(node.Children, newNode)
				found = true
				break
			}
		}
		if !found {
			// Create new folder
			newFolder := &Session{
				ID:       uuid.New().String(),
				Name:     groupName,
				Type:     TypeFolder,
				Children: []*Session{newNode},
			}
			m.Sessions = append(m.Sessions, newFolder)
		}
	} else {
		// Add to root
		m.Sessions = append(m.Sessions, newNode)
	}

	return m.saveLocked()
}

func (m *Manager) UpdateSession(id string, config remote.ConnectConfig, groupName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	release, err := m.beginMutation()
	if err != nil {
		return err
	}
	defer release()

	// 归一化 Protocol:空值视为 SSH,保证去重比较一致。
	if config.Protocol == "" {
		config.Protocol = remote.ProtocolSSH
	}

	var removed *Session
	var removeByID func(nodes []*Session) []*Session
	removeByID = func(nodes []*Session) []*Session {
		var result []*Session
		for _, node := range nodes {
			if node.ID == id {
				removed = node
				continue
			}
			if node.Type == TypeFolder {
				node.Children = removeByID(node.Children)
			}
			result = append(result, node)
		}
		return result
	}
	m.Sessions = removeByID(m.Sessions)
	if removed == nil {
		return fmt.Errorf("session not found")
	}

	if m.PreserveCredentials {
		preserveCredentials(&config, removed.Config)
	}
	config.Group = groupName

	// 重复检测:key 为 (Host, Port, Protocol) 三元组,允许同 host 不同协议共存。
	var hasDuplicateHost func(nodes []*Session) bool
	hasDuplicateHost = func(nodes []*Session) bool {
		for _, node := range nodes {
			if node.Type == TypeSession && node.Config != nil && sameEndpoint(node.Config, &config) && node.ID != id {
				return true
			}
			if node.Type == TypeFolder && hasDuplicateHost(node.Children) {
				return true
			}
		}
		return false
	}
	if hasDuplicateHost(m.Sessions) {
		return fmt.Errorf("a session with the same host already exists")
	}

	oldDisplayName := removed.Name
	oldHost := ""
	if removed.Config != nil {
		oldHost = removed.Config.Host
	}

	displayName := config.Name
	if displayName == "" {
		displayName = config.Host
	}
	if oldDisplayName != "" && oldHost != "" && oldDisplayName != oldHost {
		if config.Name == "" || config.Name == oldHost || config.Name == oldDisplayName {
			displayName = oldDisplayName
		}
	}
	config.Name = displayName

	newNode := &Session{
		ID:     id,
		Name:   displayName,
		Type:   TypeSession,
		Config: &config,
	}

	if groupName != "" {
		found := false
		for _, node := range m.Sessions {
			if node.Type == TypeFolder && node.Name == groupName {
				node.Children = append(node.Children, newNode)
				found = true
				break
			}
		}
		if !found {
			newFolder := &Session{
				ID:       uuid.New().String(),
				Name:     groupName,
				Type:     TypeFolder,
				Children: []*Session{newNode},
			}
			m.Sessions = append(m.Sessions, newFolder)
		}
	} else {
		m.Sessions = append(m.Sessions, newNode)
	}

	return m.saveLocked()
}

// CreateFolder creates an empty folder at root level.
func (m *Manager) CreateFolder(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("folder name cannot be empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	release, err := m.beginMutation()
	if err != nil {
		return err
	}
	defer release()

	for _, s := range m.Sessions {
		if s.Type == TypeFolder && s.Name == name {
			return fmt.Errorf("folder '%s' already exists", name)
		}
	}

	folder := &Session{
		ID:       uuid.New().String(),
		Name:     name,
		Type:     TypeFolder,
		Children: []*Session{},
	}
	m.Sessions = append(m.Sessions, folder)
	return m.saveLocked()
}

// Shared plugin edits never erase a stored secret because the browser cannot read it.
func preserveCredentials(next, old *remote.ConnectConfig) {
	if old == nil {
		return
	}
	// Never carry an old server's password to a different target or account.
	if !sameEndpoint(next, old) || next.User != old.User {
		return
	}
	if next.Password == "" {
		next.Password = old.Password
	}
	if next.RootPassword == "" {
		next.RootPassword = old.RootPassword
	}
	if next.Bastion == nil {
		next.Bastion = old.Bastion
	}
}
