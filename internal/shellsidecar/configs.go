package shellsidecar

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"opscopilot/pkg/remote"
	"opscopilot/pkg/sessionmanager"
)

// ConfigService 管理已保存的连接配置：sidecar 自有数据（JSON 文件），
// 与宿主/平台无耦合；凭据落盘沿用既有决策（允许凭据落文件系统）。
// 复用 pkg/sessionmanager（与终端应用同一套树形结构与 Upsert 语义）。
type ConfigService struct {
	mu   sync.Mutex
	mgr  *sessionmanager.Manager
	path string
}

func NewConfigService(dataDir string) (*ConfigService, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建数据目录失败: %w", err)
	}
	return NewConfigServiceWithPath(filepath.Join(dataDir, "saved-connections.json"))
}

func NewConfigServiceWithPath(path string) (*ConfigService, error) {
	mgr := sessionmanager.NewManagerWithPath(path)
	if err := mgr.Load(); err != nil {
		return nil, fmt.Errorf("读取连接配置失败: %w", err)
	}
	return &ConfigService{mgr: mgr, path: path}, nil
}

// SavedSession 是配置树的一个节点（会话或文件夹），JSON 形态与终端应用一致。
type SavedSession struct {
	ID       string                `json:"id"`
	Name     string                `json:"name"`
	Type     string                `json:"type"`
	Children []SavedSession        `json:"children,omitempty"`
	Config   *remote.ConnectConfig `json:"config,omitempty"`
}

func convertTree(sessions []*sessionmanager.Session) []SavedSession {
	out := make([]SavedSession, 0, len(sessions))
	for _, s := range sessions {
		node := SavedSession{ID: s.ID, Name: s.Name, Type: string(s.Type)}
		if s.Config != nil {
			cfg := *s.Config
			node.Config = &cfg
		}
		if len(s.Children) > 0 {
			node.Children = convertTree(s.Children)
		}
		out = append(out, node)
	}
	return out
}

// List 返回配置树（含文件夹）。
func (s *ConfigService) List() ([]SavedSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.mgr.Load(); err != nil {
		return nil, err
	}
	return convertTree(s.mgr.GetSessions()), nil
}

// SaveInput 是保存/更新的入参；endpoint 相同（host/port/user/protocol）视为同一条，
// Upsert 更新而不是重复创建（与终端应用语义一致）。
type SaveInput struct {
	Name         string                `json:"name"`
	Protocol     string                `json:"protocol,omitempty"`
	Host         string                `json:"host"`
	Port         int                   `json:"port"`
	User         string                `json:"user"`
	Password     string                `json:"password"`
	HostKey      string                `json:"host_key,omitempty"`
	RootPassword string                `json:"rootPassword,omitempty"`
	Bastion      *remote.ConnectConfig `json:"bastion,omitempty"`
	Group        string                `json:"group,omitempty"`
}

func (s *ConfigService) Save(in SaveInput) (string, error) {
	if in.Host == "" || in.User == "" {
		return "", fmt.Errorf("host 和 user 不能为空")
	}
	if in.Port <= 0 {
		in.Port = 22
	}
	cfg := remote.ConnectConfig{
		Name: in.Name, Host: in.Host, Port: in.Port, User: in.User,
		Password: in.Password, RootPassword: in.RootPassword,
		HostKey:  in.HostKey,
		Bastion:  in.Bastion,
		Protocol: in.Protocol,
	}
	if cfg.Protocol == "" {
		cfg.Protocol = remote.ProtocolSSH
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.mgr.Upsert(cfg, in.Group); err != nil {
		return "", err
	}
	return s.findID(in.Host, in.Port, in.User)
}

// Update 更新一条已保存连接（含 group 移动）。config 为完整连接配置。
func (s *ConfigService) Update(id string, config remote.ConnectConfig, group string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.mgr.UpdateSession(id, config, group); err != nil {
		return err
	}
	return nil
}

// CreateFolder 在根级新建一个空文件夹。
func (s *ConfigService) CreateFolder(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.mgr.CreateFolder(name); err != nil {
		return err
	}
	return nil
}

// Delete 删除一条已保存配置（按 ID）。
func (s *ConfigService) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.mgr.DeleteSession(id); err != nil {
		return err
	}
	return nil
}

// Rename 改显示名。
func (s *ConfigService) Rename(id, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.mgr.RenameSession(id, name); err != nil {
		return err
	}
	return nil
}

func (s *ConfigService) findID(host string, port int, user string) (string, error) {
	var id string
	var walk func([]*sessionmanager.Session)
	walk = func(sessions []*sessionmanager.Session) {
		for _, s := range sessions {
			if c := s.Config; c != nil && c.Host == host && c.Port == port && c.User == user {
				id = s.ID
				return
			}
			walk(s.Children)
		}
	}
	walk(s.mgr.GetSessions())
	if id == "" {
		return "", fmt.Errorf("保存后未找到配置")
	}
	return id, nil
}
