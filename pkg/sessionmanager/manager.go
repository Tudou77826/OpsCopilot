package sessionmanager

import (
	"encoding/json"
	"fmt"
	"opscopilot/pkg/remote"
	"os"
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
	ID       string                   `json:"id"`
	Name     string                   `json:"name"` // Display name (IP)
	Type     SessionType              `json:"type"`
	Children []*Session               `json:"children,omitempty"` // For folders
	Config   *remote.ConnectConfig `json:"config,omitempty"`   // For sessions
}

type Manager struct {
	filePath string
	Sessions []*Session
	mu       sync.RWMutex
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

	data, err := os.ReadFile(m.filePath)
	if os.IsNotExist(err) {
		m.Sessions = []*Session{}
		return m.Save()
	}
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &m.Sessions)
}

func (m *Manager) Save() error {
	// Assumes lock is held by caller or public method
	data, err := json.MarshalIndent(m.Sessions, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.filePath, data, 0644)
}

func (m *Manager) GetSessions() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Sessions
}

func (m *Manager) DeleteSession(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

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
	return m.Save()
}

func (m *Manager) RenameSession(id, newName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var renameNode func(nodes []*Session) bool
	renameNode = func(nodes []*Session) bool {
		for _, node := range nodes {
			if node.ID == id {
				node.Name = newName
				if node.Config != nil {
					// We only change the display name of the node,
					// but maybe we should also sync it to config?
					// The Requirement says "Name uses IP".
					// If user renames, they might want a custom alias.
					// So we allow renaming the Node.Name.
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
		return m.Save()
	}
	return fmt.Errorf("session not found")
}

// Upsert adds or updates a session.
// If groupName is provided, it puts it in that folder.
// Naming rule: Use IP (Host) as Name.
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
// Naming rule: Use IP (Host) as Name.
//
// 去重 key 为 (Host, Port, Protocol) 三元组:同一 host 不同协议/端口可共存,
// 例如 192.168.1.1:22(SSH) 与 192.168.1.1:23(Telnet) 不冲突。
// Protocol 空值在比较前归一化为 SSH,保证一致。
func (m *Manager) Upsert(config remote.ConnectConfig, groupName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 归一化 Protocol:空值视为 SSH,保证去重比较一致。
	if config.Protocol == "" {
		config.Protocol = remote.ProtocolSSH
	}

	targetName := config.Host
	if targetName == "" {
		targetName = config.Name // Fallback
	}

	// Step 1: Remove existing session with same (Host, Port, Protocol) from anywhere in the tree.
	// This handles "Update" (by removing old and adding new) and "Move" (if group changed).
	var removedID string
	var removeByHost func(nodes []*Session) []*Session
	removeByHost = func(nodes []*Session) []*Session {
		var result []*Session
		for _, node := range nodes {
			if node.Type == TypeSession && node.Config != nil && sameEndpoint(node.Config, &config) {
				removedID = node.ID // Reuse ID
				continue            // Remove
			}
			if node.Type == TypeFolder {
				node.Children = removeByHost(node.Children)
			}
			result = append(result, node)
		}
		return result
	}
	m.Sessions = removeByHost(m.Sessions)

	// Step 2: Create new node
	newNode := &Session{
		ID:     removedID,
		Name:   targetName,
		Type:   TypeSession,
		Config: &config,
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

	return m.Save()
}

func (m *Manager) UpdateSession(id string, config remote.ConnectConfig, groupName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

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

	return m.Save()
}

// CreateFolder creates an empty folder at root level.
func (m *Manager) CreateFolder(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("folder name cannot be empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

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
	return m.Save()
}
