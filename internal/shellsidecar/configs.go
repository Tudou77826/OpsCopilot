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
	mgr := connectionstore.NewStoreWithPath(filepath.Join(dataDir, "saved-connections.json"))
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

// ConnectionInput 是 RPC 入参侧的连接配置，JSON tag 为驼峰，与前端 TS 类型一致。
//
// 不能直接用 remote.ConnectConfig 接 RPC 入参：它的 JSON tag 是下划线风格
// （root_password），前端发来的 rootPassword 会被静默丢弃，导致整体替换保存时
// 清空已存的 root 密码。桌面端 Wails 边界存在同样的坑，两端用同一套转换约定。
type ConnectionInput struct {
	Name         string           `json:"name"`
	Protocol     string           `json:"protocol,omitempty"`
	Host         string           `json:"host"`
	Port         int              `json:"port"`
	User         string           `json:"user"`
	Password     string           `json:"password"`
	RootPassword string           `json:"rootPassword,omitempty"`
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
	return convertTree(s.mgr.Snapshot()), nil
}

// Save 是"连接时自动落库"的入口：按端点 upsert，返回节点 ID。
//
// in.Group 是便于输入的分组路径（支持 "A/B" 多层），会解析为文件夹 ID；
// 留空则落在根。这是唯一按名字定位文件夹的入口，仅供该便捷场景使用。
func (s *ConfigService) Save(in ConnectionInput) (string, error) {
	if in.Host == "" || in.User == "" {
		return "", fmt.Errorf("host 和 user 不能为空")
	}
	if in.Port <= 0 {
		in.Port = 22
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	parentID, err := s.mgr.EnsureFolderByNamePath(in.Group)
	if err != nil {
		return "", err
	}
	node, err := s.mgr.UpsertByEndpoint(in.toRemote(), parentID)
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
