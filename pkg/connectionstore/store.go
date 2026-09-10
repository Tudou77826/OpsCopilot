// Package connectionstore 持久化"已保存连接"的树形结构。
//
// 数据模型是文件夹与连接的混合树，落盘为单个 JSON 文件（默认 sessions.json）。
// 三条不变量贯穿本包的全部实现：
//
//  1. 树结构是归属的唯一真相。ConnectConfig.Group 只是写时派生的镜像，
//     供 pkg/core/ops 等外部消费者展示用，任何判断都不得读它。
//  2. 文件夹按 ID 寻址，不按名字。因此重命名文件夹不会让任何引用失配。
//  3. 嵌套深度不限，移动操作做环检测，禁止把文件夹移入自身或其后代。
//
// JSON 线格式与历史 sessions.json 完全一致（type/children/config，type 取值
// folder/session），旧文件零迁移可读。
package connectionstore

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"opscopilot/internal/atomicfile"
	"opscopilot/pkg/remote"

	"github.com/google/uuid"
)

// Kind 是树节点的种类。
type Kind string

const (
	KindFolder Kind = "folder"
	// KindConnection 的取值沿用历史 JSON 里的 "session"，以保证旧文件零迁移。
	KindConnection Kind = "session"
)

// Node 是持久化树的一个节点：文件夹（KindFolder）或已保存连接（KindConnection）。
// 两种节点互斥：文件夹有 Children 无 Config，连接有 Config 无 Children。
type Node struct {
	ID       string                `json:"id"`
	Name     string                `json:"name"` // 显示名，默认取 Host
	Type     Kind                  `json:"type"`
	Children []*Node               `json:"children,omitempty"` // 仅文件夹
	Config   *remote.ConnectConfig `json:"config,omitempty"`   // 仅连接
}

// 领域错误。文案面向用户，故用中文。
var (
	ErrNotFound          = errors.New("节点不存在")
	ErrNotFolder         = errors.New("目标不是文件夹")
	ErrNotConnection     = errors.New("该节点不是连接")
	ErrParentNotFound    = errors.New("父文件夹不存在")
	ErrCycle             = errors.New("不能把文件夹移动到它自己或它的子文件夹内")
	ErrEmptyName         = errors.New("名称不能为空")
	ErrDuplicateFolder   = errors.New("同一层已存在同名文件夹")
	ErrDuplicateEndpoint = errors.New("已存在相同协议、主机和端口的连接")
)

// Store 管理一棵连接树及其落盘。
type Store struct {
	mu        sync.RWMutex
	filePath  string
	nodes     []*Node
	lastSaved []byte // 上一次落盘的内容，用于跳过无变更写入
}

func NewStore() *Store {
	return &Store{filePath: "sessions.json", nodes: []*Node{}}
}

// NewStoreWithPath 创建使用指定文件路径的 Store。
func NewStoreWithPath(filePath string) *Store {
	return &Store{filePath: filePath, nodes: []*Node{}}
}

// Load 从磁盘读取连接树。
//
// 读取后会执行一次对账（reconcile）：修复历史数据中的结构不一致，并按树结构
// 重建 config.group 镜像。若对账产生了变更，会先把原始文件备份为
// sessions.json.bak-<时间戳> 再落盘，保证用户可回退。
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := os.ReadFile(s.filePath)
	missing := os.IsNotExist(err)
	if err != nil && !missing {
		return fmt.Errorf("读取 %s 失败: %w", s.filePath, err)
	}

	var nodes []*Node
	if !missing && len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &nodes); err != nil {
			return fmt.Errorf("解析 %s 失败: %w", s.filePath, err)
		}
	}

	repairedTree, repaired := reconcile(nodes)
	s.nodes = repairedTree

	encoded, err := s.encodeLocked()
	if err != nil {
		return err
	}

	if !missing && repaired {
		if err := s.backupLocked(); err != nil {
			return fmt.Errorf("备份 %s 失败: %w", s.filePath, err)
		}
	}
	if missing || repaired {
		if err := atomicfile.Write(s.filePath, encoded, 0o644); err != nil {
			return fmt.Errorf("写入 %s 失败: %w", s.filePath, err)
		}
	}
	s.lastSaved = encoded
	return nil
}

// Save 把当前树落盘。内容与上次写入完全相同时跳过写盘。
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked()
}

// Snapshot 返回树的深拷贝。
//
// 返回深拷贝而非内部切片，是为了让调用方（含 Wails 的 JSON 序列化）在锁外
// 安全使用，不会与并发写操作产生数据竞争。
func (s *Store) Snapshot() []*Node {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Node, 0, len(s.nodes))
	for _, n := range s.nodes {
		out = append(out, cloneNode(n))
	}
	return out
}

// FindByEndpoint 按 (协议, 主机, 端口) 查找连接，返回深拷贝；未找到返回 nil。
// Protocol 为空按 SSH 处理，与 remote.Dial 的归一化一致。
func (s *Store) FindByEndpoint(protocol, host string, port int) *Node {
	s.mu.RLock()
	defer s.mu.RUnlock()
	target := &remote.ConnectConfig{Protocol: protocol, Host: host, Port: port}
	return cloneNode(findEndpoint(s.nodes, target, ""))
}

// CreateFolder 在 parentID 指定的文件夹下新建空文件夹；parentID 为空表示根。
func (s *Store) CreateFolder(name, parentID string) (*Node, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrEmptyName
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	siblings, err := s.childrenOfLocked(parentID)
	if err != nil {
		return nil, err
	}
	if findChildFolderByName(*siblings, name, "") != nil {
		return nil, fmt.Errorf("%w: %q", ErrDuplicateFolder, name)
	}

	folder := &Node{ID: uuid.New().String(), Name: name, Type: KindFolder}
	*siblings = append(*siblings, folder)
	return cloneNode(folder), s.saveLocked()
}

// EnsureFolderByNamePath 按 "A/B/C" 形式的路径逐层查找或创建文件夹，
// 返回最末一层文件夹的 ID；空路径返回 ""（表示根）。
//
// 同名文件夹在同一层只会有一个，因此该操作天然幂等。路径段以 "/" 分隔，
// 对应 UI 上"保存到分组"输入框的便捷语义（NameDialog 禁止名称含 "/"）。
func (s *Store) EnsureFolderByNamePath(path string) (string, error) {
	segments := splitGroupPath(path)
	if len(segments) == 0 {
		return "", nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	siblings := &s.nodes
	var current *Node
	for _, seg := range segments {
		found := findChildFolderByName(*siblings, seg, "")
		if found == nil {
			found = &Node{ID: uuid.New().String(), Name: seg, Type: KindFolder}
			*siblings = append(*siblings, found)
		}
		current = found
		siblings = &found.Children
	}
	return current.ID, s.saveLocked()
}

// CreateConnection 新建一条连接而不建立实际会话（"新建会话"入口）。
// 同一 endpoint 已存在时拒绝，避免用户看到两条指向同一主机的条目。
func (s *Store) CreateConnection(cfg remote.ConnectConfig, parentID string) (*Node, error) {
	cfg.Protocol = normalizedProtocol(cfg.Protocol)
	if strings.TrimSpace(cfg.Host) == "" {
		return nil, errors.New("主机地址不能为空")
	}
	if cfg.Port <= 0 {
		return nil, errors.New("端口必须大于 0")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	siblings, err := s.childrenOfLocked(parentID)
	if err != nil {
		return nil, err
	}
	if findEndpoint(s.nodes, &cfg, "") != nil {
		return nil, fmt.Errorf("%w: %s:%d", ErrDuplicateEndpoint, cfg.Host, cfg.Port)
	}

	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		name = cfg.Host
	}
	cfg.Name = name
	cfg.Group = ""

	node := &Node{ID: uuid.New().String(), Name: name, Type: KindConnection, Config: &cfg}
	*siblings = append(*siblings, node)
	return cloneNode(node), s.saveLocked()
}

// UpsertByEndpoint 是全树范围内的"按端点插入或更新"，供连接时自动落库使用。
//
// 与 CreateConnection 的差别：命中同端点旧节点时不报错，而是复用其 ID 与
// 已改过的显示名，把节点摘下来重新放到 parentID 下（因此连接时指定新分组
// 等价于移动）。
func (s *Store) UpsertByEndpoint(cfg remote.ConnectConfig, parentID string) (*Node, error) {
	cfg.Protocol = normalizedProtocol(cfg.Protocol)
	if strings.TrimSpace(cfg.Host) == "" {
		return nil, errors.New("主机地址不能为空")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	siblings, err := s.childrenOfLocked(parentID)
	if err != nil {
		return nil, err
	}

	var removed *Node
	removeByEndpoint(&s.nodes, &cfg, &removed)

	// 用户若在会话树里改过显示名，而本次连接携带的配置名仍为空或还是旧配置名，
	// 则保留改过的显示名，否则用户的改名会在每次连接后被覆盖。
	targetName := strings.TrimSpace(cfg.Name)
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
		targetName = cfg.Host
	}
	cfg.Name = targetName
	cfg.Group = ""

	id := uuid.New().String()
	if removed != nil {
		id = removed.ID
	}

	node := &Node{ID: id, Name: targetName, Type: KindConnection, Config: &cfg}
	*siblings = append(*siblings, node)
	return cloneNode(node), s.saveLocked()
}

// RenameNode 改显示名。重命名连接时同步 Config.Name 以保持一致。
func (s *Store) RenameNode(id, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrEmptyName
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	loc := locate(&s.nodes, id)
	if loc == nil {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}

	if loc.node.Type == KindFolder {
		if findChildFolderByName(*loc.siblings, name, id) != nil {
			return fmt.Errorf("%w: %q", ErrDuplicateFolder, name)
		}
	} else if loc.node.Config != nil {
		loc.node.Config.Name = name
	}
	loc.node.Name = name
	return s.saveLocked()
}

// UpdateConnection 更新一条连接的配置，位置不变（移动请用 MoveNode）。
//
// 与别的连接撞端点时拒绝；显示名沿用旧规则：调用方未显式改名时保留原名。
func (s *Store) UpdateConnection(id string, cfg remote.ConnectConfig) error {
	cfg.Protocol = normalizedProtocol(cfg.Protocol)
	if strings.TrimSpace(cfg.Host) == "" {
		return errors.New("主机地址不能为空")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	loc := locate(&s.nodes, id)
	if loc == nil {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if loc.node.Type != KindConnection {
		return ErrNotConnection
	}
	if findEndpoint(s.nodes, &cfg, id) != nil {
		return fmt.Errorf("%w: %s:%d", ErrDuplicateEndpoint, cfg.Host, cfg.Port)
	}

	oldName := loc.node.Name
	oldHost := ""
	if loc.node.Config != nil {
		oldHost = loc.node.Config.Host
	}

	rawName := strings.TrimSpace(cfg.Name)
	display := rawName
	if display == "" {
		display = cfg.Host
	}
	if oldName != "" && oldHost != "" && oldName != oldHost {
		if rawName == "" || rawName == oldHost || rawName == oldName {
			display = oldName
		}
	}

	cfg.Name = display
	cfg.Group = ""
	loc.node.Name = display
	loc.node.Config = &cfg
	return s.saveLocked()
}

// MoveNode 把节点移动到 newParentID 下（空表示根）的第 index 位。
//
// index 的语义是"摘除待移动节点之后、目标兄弟列表中的插入位置"，取值超出范围
// 时追加到末尾。调用方若按"插入到某兄弟之前"计算位置，需自行扣除被移动节点
// 自身造成的下标偏移。
func (s *Store) MoveNode(id, newParentID string, index int) error {
	if id == newParentID {
		return ErrCycle
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	loc := locate(&s.nodes, id)
	if loc == nil {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	// 环检测：文件夹不能移到自身的后代里，否则会形成脱离根节点的环。
	if loc.node.Type == KindFolder && isDescendant(loc.node, newParentID) {
		return ErrCycle
	}

	if _, err := s.childrenOfLocked(newParentID); err != nil {
		return err
	}

	// 先摘除，再重新取目标切片：同一父级内移动时两者是同一个切片，长度已变。
	node := loc.node
	removeAt(loc.siblings, loc.index)

	target, err := s.childrenOfLocked(newParentID)
	if err != nil {
		return err
	}
	if index < 0 || index > len(*target) {
		index = len(*target)
	}
	insertAt(target, index, node)

	return s.saveLocked()
}

// ReorderNodes 把 parentID 下的子节点按 orderedIDs 重新排序。
//
// 排序规则交由调用方决定：浏览器的 localeCompare('zh-CN') 能正确处理中文
// 拼音序，而 Go 侧没有等价的本地化比较，因此"按名称排序"由前端算出顺序后
// 调本方法落库。orderedIDs 必须是当前子节点集合的一个完整排列。
func (s *Store) ReorderNodes(parentID string, orderedIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	siblings, err := s.childrenOfLocked(parentID)
	if err != nil {
		return err
	}
	if len(orderedIDs) != len(*siblings) {
		return fmt.Errorf("排序列表有 %d 项，实际子节点 %d 个", len(orderedIDs), len(*siblings))
	}

	byID := make(map[string]*Node, len(*siblings))
	for _, n := range *siblings {
		byID[n.ID] = n
	}

	reordered := make([]*Node, 0, len(orderedIDs))
	seen := make(map[string]bool, len(orderedIDs))
	for _, nid := range orderedIDs {
		n, ok := byID[nid]
		if !ok {
			return fmt.Errorf("排序列表包含不属于该层的节点: %s", nid)
		}
		if seen[nid] {
			return fmt.Errorf("排序列表包含重复节点: %s", nid)
		}
		seen[nid] = true
		reordered = append(reordered, n)
	}

	*siblings = reordered
	return s.saveLocked()
}

// DeleteNode 删除节点。删除文件夹会连带删除其整棵子树。
func (s *Store) DeleteNode(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	loc := locate(&s.nodes, id)
	if loc == nil {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	removeAt(loc.siblings, loc.index)
	return s.saveLocked()
}

// DuplicateConnection 复制一条连接：新 ID、名称加"-副本"后缀、配置深拷贝
// （含跳板机），副本紧跟源节点之后，落回同一文件夹。
//
// 与 CreateConnection 不同，这里允许与源同端点——用户复制后通常马上改主机。
func (s *Store) DuplicateConnection(id string) (*Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	loc := locate(&s.nodes, id)
	if loc == nil {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if loc.node.Type != KindConnection || loc.node.Config == nil {
		return nil, ErrNotConnection
	}

	dup := &Node{
		ID:     uuid.New().String(),
		Name:   uniqueSiblingName(*loc.siblings, loc.node.Name+"-副本"),
		Type:   KindConnection,
		Config: cloneConfig(loc.node.Config),
	}
	insertAt(loc.siblings, loc.index+1, dup)
	return cloneNode(dup), s.saveLocked()
}

// --- 内部实现 ---

// location 描述一个节点在树中的位置。siblings 指向包含该节点的切片
// （可能是 &s.nodes，也可能是某个父节点的 Children 字段）。
type location struct {
	node     *Node
	siblings *[]*Node
	index    int
}

func locate(nodes *[]*Node, id string) *location {
	for i, n := range *nodes {
		if n.ID == id {
			return &location{node: n, siblings: nodes, index: i}
		}
		if n.Type == KindFolder && len(n.Children) > 0 {
			if loc := locate(&n.Children, id); loc != nil {
				return loc
			}
		}
	}
	return nil
}

func (s *Store) childrenOfLocked(parentID string) (*[]*Node, error) {
	if parentID == "" {
		return &s.nodes, nil
	}
	loc := locate(&s.nodes, parentID)
	if loc == nil {
		return nil, fmt.Errorf("%w: %s", ErrParentNotFound, parentID)
	}
	if loc.node.Type != KindFolder {
		return nil, ErrNotFolder
	}
	return &loc.node.Children, nil
}

// removeAt 用容量受限的切片表达式重建，避免底层数组继续引用被删除的子树。
func removeAt(siblings *[]*Node, index int) {
	list := *siblings
	*siblings = append(list[:index:index], list[index+1:]...)
}

func insertAt(siblings *[]*Node, index int, node *Node) {
	list := *siblings
	list = append(list, nil)
	copy(list[index+1:], list[index:])
	list[index] = node
	*siblings = list
}

func isDescendant(ancestor *Node, id string) bool {
	if id == "" {
		return false
	}
	for _, child := range ancestor.Children {
		if child.ID == id || isDescendant(child, id) {
			return true
		}
	}
	return false
}

func findChildFolderByName(siblings []*Node, name, excludeID string) *Node {
	for _, n := range siblings {
		if n.Type == KindFolder && n.Name == name && n.ID != excludeID {
			return n
		}
	}
	return nil
}

// findEndpoint 在树中查找与 target 同端点的连接；excludeID 用于放过自身。
func findEndpoint(nodes []*Node, target *remote.ConnectConfig, excludeID string) *Node {
	for _, n := range nodes {
		if n.Type == KindConnection && n.ID != excludeID && sameEndpoint(n.Config, target) {
			return n
		}
		if n.Type == KindFolder {
			if found := findEndpoint(n.Children, target, excludeID); found != nil {
				return found
			}
		}
	}
	return nil
}

func removeByEndpoint(nodes *[]*Node, target *remote.ConnectConfig, removed **Node) {
	list := *nodes
	out := make([]*Node, 0, len(list))
	for _, n := range list {
		if n.Type == KindConnection && sameEndpoint(n.Config, target) {
			*removed = n
			continue
		}
		if n.Type == KindFolder && len(n.Children) > 0 {
			removeByEndpoint(&n.Children, target, removed)
		}
		out = append(out, n)
	}
	*nodes = out
}

func uniqueSiblingName(siblings []*Node, base string) string {
	used := make(map[string]bool, len(siblings))
	for _, n := range siblings {
		used[n.Name] = true
	}
	if !used[base] {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s%d", base, i)
		if !used[candidate] {
			return candidate
		}
	}
}

func splitGroupPath(path string) []string {
	var out []string
	for _, seg := range strings.Split(path, "/") {
		if seg = strings.TrimSpace(seg); seg != "" {
			out = append(out, seg)
		}
	}
	return out
}

func normalizedProtocol(p string) string {
	if p == "" {
		return remote.ProtocolSSH
	}
	return p
}

// sameEndpoint 判断两个连接是否指向同一远程端点，键为 (协议, 主机, 端口)。
// 协议空值先归一化为 SSH，使老数据（无 Protocol 字段）与新数据可比。
func sameEndpoint(a, b *remote.ConnectConfig) bool {
	if a == nil || b == nil {
		return false
	}
	return a.Host == b.Host && a.Port == b.Port &&
		normalizedProtocol(a.Protocol) == normalizedProtocol(b.Protocol)
}

func cloneConfig(cfg *remote.ConnectConfig) *remote.ConnectConfig {
	if cfg == nil {
		return nil
	}
	out := *cfg
	out.Bastion = cloneConfig(cfg.Bastion)
	return &out
}

func cloneNode(n *Node) *Node {
	if n == nil {
		return nil
	}
	out := &Node{ID: n.ID, Name: n.Name, Type: n.Type, Config: cloneConfig(n.Config)}
	if len(n.Children) > 0 {
		out.Children = make([]*Node, 0, len(n.Children))
		for _, child := range n.Children {
			out.Children = append(out.Children, cloneNode(child))
		}
	}
	return out
}

// encodeLocked 序列化当前树。
//
// 序列化前先重建 config.group 镜像，使"结构是唯一真相"落到磁盘上：该字段对
// 外部消费者（pkg/core/ops 的服务器列表）可用，但永远由树位置决定。
func (s *Store) encodeLocked() ([]byte, error) {
	deriveGroupMirror(s.nodes, "")
	data, err := json.MarshalIndent(s.nodes, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("序列化连接树失败: %w", err)
	}
	return data, nil
}

func (s *Store) saveLocked() error {
	encoded, err := s.encodeLocked()
	if err != nil {
		return err
	}
	if bytes.Equal(encoded, s.lastSaved) {
		return nil // 无变更不写盘
	}
	if err := atomicfile.Write(s.filePath, encoded, 0o644); err != nil {
		return fmt.Errorf("写入 %s 失败: %w", s.filePath, err)
	}
	s.lastSaved = encoded
	return nil
}

// backupLocked 把当前磁盘内容备份为 sessions.json.bak-<时间戳>。
// 只在首次对账修复历史数据时调用，让用户可回退到修复前的状态。
func (s *Store) backupLocked() error {
	raw, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	stamp := time.Now().Format("20060102-150405")
	return os.WriteFile(s.filePath+".bak-"+stamp, raw, 0o644)
}

// deriveGroupMirror 按树结构重建 config.group：根节点为空，其余为直接父
// 文件夹名。它是给外部消费者的派生镜像，不参与本包的任何判断。
func deriveGroupMirror(nodes []*Node, parentFolderName string) {
	for _, n := range nodes {
		if n.Type == KindFolder {
			deriveGroupMirror(n.Children, n.Name)
			continue
		}
		if n.Config != nil {
			n.Config.Group = parentFolderName
		}
	}
}

// reconcile 让树结构成为唯一真相，并修复历史数据中的已知不一致。
// 幂等：对已经规范的数据返回 changed=false。
//
// 修复项：
//   - 丢弃 nil 节点（JSON 里的 null 元素）
//   - 补全缺失的 ID；重复 ID 重新分配（保留先出现的那个）
//   - 补全空名称（连接回退到 Host）
//   - 纠正互斥字段：文件夹不应有 Config，连接不应有 Children
//   - 未知/空的 type 按有无 Config 推断
//   - 空 Children 归一化为 nil，保证序列化结果稳定
//
// 注意：不在此处强制"同级文件夹不同名"。历史数据可能已有同名兄弟，自动改名
// 会让用户困惑；唯一性只在新建与重命名时校验。
func reconcile(nodes []*Node) ([]*Node, bool) {
	seen := make(map[string]bool, len(nodes))
	changed := false
	out := make([]*Node, 0, len(nodes))

	for _, n := range nodes {
		if n == nil {
			changed = true
			continue
		}
		if reconcileNode(n, seen) {
			changed = true
		}
		out = append(out, n)
	}
	return out, changed
}

func reconcileNode(n *Node, seen map[string]bool) bool {
	changed := false

	if strings.TrimSpace(n.ID) == "" || seen[n.ID] {
		n.ID = uuid.New().String()
		changed = true
	}
	seen[n.ID] = true

	if strings.TrimSpace(n.Name) == "" {
		if n.Config != nil && n.Config.Host != "" {
			n.Name = n.Config.Host
		} else {
			n.Name = "未命名"
		}
		changed = true
	}

	switch n.Type {
	case KindFolder:
		if n.Config != nil {
			n.Config = nil
			changed = true
		}
	case KindConnection:
		if len(n.Children) > 0 {
			n.Children = nil
			changed = true
		}
	default:
		if n.Config != nil {
			n.Type = KindConnection
		} else {
			n.Type = KindFolder
		}
		changed = true
	}

	if n.Type == KindFolder {
		if len(n.Children) == 0 {
			if n.Children != nil {
				n.Children = nil
				changed = true
			}
			return changed
		}
		children, childrenChanged := reconcile(n.Children)
		if childrenChanged {
			changed = true
		}
		n.Children = children
		return changed
	}

	// 连接的显示名与 Config.Name 必须一致：历史数据里存在 Config.Name 缺失的条目，
	// 留着空值会让后续"未改名"的判断失去参照。
	if n.Config != nil && strings.TrimSpace(n.Config.Name) == "" {
		n.Config.Name = n.Name
		changed = true
	}
	return changed
}
