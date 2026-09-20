package shellsidecar

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"opscopilot/pkg/connectionstore"
	"opscopilot/pkg/remote"
)

// ConfigService 管理已保存的连接树：sidecar 自有数据（JSON 文件），与宿主/平台
// 无耦合。复用 pkg/connectionstore，因此文件夹嵌套、ID 寻址、端点去重等语义
// 与桌面端完全一致——两端共享同一份实现，不各自维护一套。
type ConfigService struct {
	mu  sync.Mutex
	mgr *connectionstore.Store
}

func NewConfigService(dataDir string) (*ConfigService, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建数据目录失败: %w", err)
	}
	return NewConfigServiceWithPath(filepath.Join(dataDir, "saved-connections.json"))
}

// NewConfigServiceWithPath 以指定文件构造配置服务。桌面模式（sidecar 读写桌面应用
// 自己的 sessions.json）必须指定路径，而不是在 dataDir 下另起一个文件。
func NewConfigServiceWithPath(path string) (*ConfigService, error) {
	mgr := connectionstore.NewStoreWithPath(path)
	if err := mgr.Load(); err != nil {
		return nil, fmt.Errorf("读取连接配置失败: %w", err)
	}
	return &ConfigService{mgr: mgr}, nil
}

// SavedNode 是配置树的一个节点（文件夹或连接），JSON 形态与桌面端一致。
// 出参用 remote.ConnectConfig（下划线 tag），前端读取时做 root_password →
// rootPassword 的归一化，这与桌面端 Wails 出参的处理方式相同。
type SavedNode struct {
	ID       string                `json:"id"`
	Name     string                `json:"name"`
	Type     string                `json:"type"`
	Children []SavedNode           `json:"children,omitempty"`
	Config   *remote.ConnectConfig `json:"config,omitempty"`
}

// ConnectionInput 是 RPC 入参侧的连接配置。
//
// 字段名不是任意选的：唯一客户端是插件的 business.ts，它发出的 payload 里
// 名字与密码相关字段用的是下划线风格（host_key / root_password，见
// plugins/teams-opscopilot/src/business.ts 与 connections.ts）。名字写错不会报错，
// 只会静默丢掉该字段——保存时会把已存的主机密钥/root 密码清空，所以这里必须与
// 客户端逐字一致。其余字段（name/host/port/user/password/protocol/group）都是单词。
type ConnectionInput struct {
	Name         string           `json:"name"`
	Protocol     string           `json:"protocol,omitempty"`
	Host         string           `json:"host"`
	Port         int              `json:"port"`
	User         string           `json:"user"`
	Password     string           `json:"password"`
	RootPassword string           `json:"root_password,omitempty"`
	HostKey      string           `json:"host_key,omitempty"`
	Group        string           `json:"group,omitempty"`
	Bastion      *ConnectionInput `json:"bastion,omitempty"`
}

// toRemote 逐字段转换到持久化结构，Bastion 递归。
func (in ConnectionInput) toRemote() remote.ConnectConfig {
	out := remote.ConnectConfig{
		Name:         in.Name,
		Protocol:     in.Protocol,
		Host:         in.Host,
		Port:         in.Port,
		User:         in.User,
		Password:     in.Password,
		RootPassword: in.RootPassword,
		HostKey:      in.HostKey,
		Group:        in.Group,
	}
	if in.Bastion != nil {
		bastion := in.Bastion.toRemote()
		out.Bastion = &bastion
	}
	return out
}

func convertTree(nodes []*connectionstore.Node) []SavedNode {
	out := make([]SavedNode, 0, len(nodes))
	for _, n := range nodes {
		node := SavedNode{ID: n.ID, Name: n.Name, Type: string(n.Type)}
		if n.Config != nil {
			cfg := *n.Config
			node.Config = &cfg
		}
		if len(n.Children) > 0 {
			node.Children = convertTree(n.Children)
		}
		out = append(out, node)
	}
	return out
}

// List 返回配置树（含任意层级的文件夹）。
func (s *ConfigService) List() ([]SavedNode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 共享模式下文件可能被桌面端改过：先按 filetxn 重新读一次，再给出快照。
	if err := s.mgr.Load(); err != nil {
		return nil, err
	}
	return convertTree(s.mgr.Snapshot()), nil
}

// Save 是"连接时自动落库"的入口：按端点 ensure，返回节点 ID。
//
// 语义与桌面端连接钩子一致（connectionstore.EnsureConnectionByEndpoint）：
// 端点已存在时只合入凭据，不改动显示名与位置——连接重放携带的旧名字/旧分组
// 不得覆盖用户在树里的改名与调整；不存在时才按 in.Group（支持 "A/B" 多层）
// 名字路径解析落位文件夹。留空落在根。
func (s *ConfigService) Save(in ConnectionInput) (string, error) {
	if in.Host == "" || in.User == "" {
		return "", fmt.Errorf("host 和 user 不能为空")
	}
	if in.Port <= 0 {
		in.Port = 22
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	node, err := s.mgr.EnsureConnectionByEndpoint(in.toRemote(), in.Group)
	if err != nil {
		return "", err
	}
	return node.ID, nil
}

// CreateConnection 新建一条连接（不建立会话），返回节点 ID。
func (s *ConfigService) CreateConnection(in ConnectionInput, parentID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	node, err := s.mgr.CreateConnection(in.toRemote(), parentID)
	if err != nil {
		return "", err
	}
	return node.ID, nil
}

// UpdateConnection 更新连接配置，节点位置不变。
func (s *ConfigService) UpdateConnection(id string, in ConnectionInput) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mgr.UpdateConnection(id, in.toRemote())
}

// CreateFolder 在 parentID 下新建文件夹，返回新文件夹 ID。
func (s *ConfigService) CreateFolder(name, parentID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	node, err := s.mgr.CreateFolder(name, parentID)
	if err != nil {
		return "", err
	}
	return node.ID, nil
}

// MoveNode 移动节点到 newParentID 下的第 index 位。
func (s *ConfigService) MoveNode(id, newParentID string, index int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mgr.MoveNode(id, newParentID, index)
}

// ReorderNodes 重排某层子节点。
func (s *ConfigService) ReorderNodes(parentID string, orderedIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mgr.ReorderNodes(parentID, orderedIDs)
}

// Delete 删除节点；删除文件夹会连带删除其整棵子树。
func (s *ConfigService) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mgr.DeleteNode(id)
}

// Rename 改显示名。
func (s *ConfigService) Rename(id, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mgr.RenameNode(id, name)
}

// Duplicate 复制一条连接，返回副本节点 ID。
func (s *ConfigService) Duplicate(id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	node, err := s.mgr.DuplicateConnection(id)
	if err != nil {
		return "", err
	}
	return node.ID, nil
}
