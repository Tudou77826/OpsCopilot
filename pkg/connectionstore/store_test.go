package connectionstore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"opscopilot/pkg/remote"
)

func TestNewStoreDefaults(t *testing.T) {
	s := NewStore()
	if s.nodes == nil {
		t.Error("nodes 应被初始化")
	}
	if s.filePath != "sessions.json" {
		t.Errorf("默认 filePath 应为 sessions.json，实际 %s", s.filePath)
	}
}

// ── 基础增删改 ──────────────────────────────────────────────

func TestUpsertByEndpoint_AddUpdateMove(t *testing.T) {
	s := NewStoreWithPath(filepath.Join(t.TempDir(), "sessions.json"))

	cfg := remote.ConnectConfig{Host: "192.168.1.1", User: "root", Port: 22}
	if _, err := s.UpsertByEndpoint(cfg, ""); err != nil {
		t.Fatalf("首次写入失败: %v", err)
	}
	if len(s.nodes) != 1 {
		t.Fatalf("期望 1 个根节点，实际 %d", len(s.nodes))
	}
	if s.nodes[0].Name != "192.168.1.1" {
		t.Errorf("显示名应回退为主机地址，实际 %q", s.nodes[0].Name)
	}
	if s.nodes[0].Type != KindConnection {
		t.Errorf("类型应为连接，实际 %q", s.nodes[0].Type)
	}

	// 同端点再次写入 = 更新，不新增节点。
	cfg.User = "admin"
	if _, err := s.UpsertByEndpoint(cfg, ""); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if len(s.nodes) != 1 {
		t.Fatalf("更新后仍应只有 1 个节点，实际 %d", len(s.nodes))
	}
	if s.nodes[0].Config.User != "admin" {
		t.Errorf("配置未更新，user=%q", s.nodes[0].Config.User)
	}

	// 指定分组写入 = 移动。
	folderID, err := s.EnsureFolderByNamePath("Prod")
	if err != nil {
		t.Fatalf("建分组失败: %v", err)
	}
	if _, err := s.UpsertByEndpoint(cfg, folderID); err != nil {
		t.Fatalf("移入分组失败: %v", err)
	}
	if len(s.nodes) != 1 {
		t.Fatalf("根下应只剩分组节点，实际 %d", len(s.nodes))
	}
	folder := s.nodes[0]
	if folder.Type != KindFolder || folder.Name != "Prod" {
		t.Fatalf("期望分组 Prod，实际 %s(%s)", folder.Name, folder.Type)
	}
	if len(folder.Children) != 1 || folder.Children[0].Config.Host != "192.168.1.1" {
		t.Fatalf("连接未落入分组")
	}
}

func TestDeleteNode(t *testing.T) {
	s := NewStoreWithPath(filepath.Join(t.TempDir(), "sessions.json"))
	node, err := s.UpsertByEndpoint(remote.ConnectConfig{Host: "1.1.1.1", Port: 22}, "")
	if err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	if err := s.DeleteNode(node.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if len(s.nodes) != 0 {
		t.Errorf("删除后根应为空，实际 %d", len(s.nodes))
	}

	if err := s.DeleteNode("no-such-id"); !errors.Is(err, ErrNotFound) {
		t.Errorf("删除不存在的节点应返回 ErrNotFound，实际 %v", err)
	}
}

func TestRenameNode_SyncsConnectionConfigName(t *testing.T) {
	s := NewStoreWithPath(filepath.Join(t.TempDir(), "sessions.json"))
	node, _ := s.UpsertByEndpoint(remote.ConnectConfig{Host: "1.1.1.1", Port: 22}, "")

	if err := s.RenameNode(node.ID, "NewName"); err != nil {
		t.Fatalf("重命名失败: %v", err)
	}
	if s.nodes[0].Name != "NewName" {
		t.Errorf("显示名未更新: %q", s.nodes[0].Name)
	}
	if s.nodes[0].Config.Name != "NewName" {
		t.Errorf("连接配置名未同步: %q", s.nodes[0].Config.Name)
	}

	if err := s.RenameNode(node.ID, "   "); !errors.Is(err, ErrEmptyName) {
		t.Errorf("空名应被拒绝，实际 %v", err)
	}
}

func TestUpsertPreservesRenamedSessionName(t *testing.T) {
	s := NewStoreWithPath(filepath.Join(t.TempDir(), "sessions.json"))

	cfg := remote.ConnectConfig{Host: "10.0.0.1", Port: 22, User: "root"}
	if _, err := s.UpsertByEndpoint(cfg, ""); err != nil {
		t.Fatalf("首次写入失败: %v", err)
	}
	id := s.nodes[0].ID

	if err := s.RenameNode(id, "web-primary"); err != nil {
		t.Fatalf("重命名失败: %v", err)
	}

	// 模拟用"名字仍为空"的旧配置重新连接。
	if _, err := s.UpsertByEndpoint(cfg, ""); err != nil {
		t.Fatalf("重连写入失败: %v", err)
	}

	if len(s.nodes) != 1 {
		t.Fatalf("期望 1 个节点，实际 %d", len(s.nodes))
	}
	if s.nodes[0].ID != id {
		t.Errorf("应复用原 ID %q，实际 %q", id, s.nodes[0].ID)
	}
	if s.nodes[0].Name != "web-primary" {
		t.Errorf("改过的显示名被覆盖: %q", s.nodes[0].Name)
	}
	if s.nodes[0].Config.Name != "web-primary" {
		t.Errorf("连接配置名未保持一致: %q", s.nodes[0].Config.Name)
	}
}

func TestUpsertUsesConfiguredDisplayName(t *testing.T) {
	s := NewStoreWithPath(filepath.Join(t.TempDir(), "sessions.json"))

	cfg := remote.ConnectConfig{Name: "database-primary", Host: "10.0.0.2", Port: 22, User: "root"}
	if _, err := s.UpsertByEndpoint(cfg, ""); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if s.nodes[0].Name != "database-primary" || s.nodes[0].Config.Name != "database-primary" {
		t.Errorf("应使用配置里的显示名，实际 node=%q config=%q",
			s.nodes[0].Name, s.nodes[0].Config.Name)
	}
}

func TestPersistenceRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")

	s1 := NewStoreWithPath(path)
	folderID, err := s1.EnsureFolderByNamePath("GroupA")
	if err != nil {
		t.Fatalf("建分组失败: %v", err)
	}
	if _, err := s1.UpsertByEndpoint(remote.ConnectConfig{Host: "1.1.1.1", Port: 22}, folderID); err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	s2 := NewStoreWithPath(path)
	if err := s2.Load(); err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if len(s2.nodes) != 1 || s2.nodes[0].Name != "GroupA" {
		t.Fatalf("分组未持久化: %+v", s2.nodes)
	}
	if len(s2.nodes[0].Children) != 1 {
		t.Fatalf("分组内的连接未持久化")
	}
}

func TestDeleteFolderRemovesWholeSubtree(t *testing.T) {
	s := NewStoreWithPath(filepath.Join(t.TempDir(), "sessions.json"))

	outerID, _ := s.EnsureFolderByNamePath("生产")
	innerID, _ := s.EnsureFolderByNamePath("生产/华东")
	if _, err := s.UpsertByEndpoint(remote.ConnectConfig{Host: "1.1.1.1", Port: 22}, innerID); err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	if err := s.DeleteNode(outerID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if len(s.nodes) != 0 {
		t.Errorf("删除外层文件夹后根应为空，实际 %d", len(s.nodes))
	}
}

// ── 多层嵌套文件夹 ──────────────────────────────────────────

func TestCreateFolder_NestedAndSiblingDuplicateRejected(t *testing.T) {
	s := NewStoreWithPath(filepath.Join(t.TempDir(), "sessions.json"))

	a, err := s.CreateFolder("A", "")
	if err != nil {
		t.Fatalf("建 A 失败: %v", err)
	}
	b, err := s.CreateFolder("B", a.ID)
	if err != nil {
		t.Fatalf("在 A 下建 B 失败: %v", err)
	}
	c, err := s.CreateFolder("C", b.ID)
	if err != nil {
		t.Fatalf("在 B 下建 C 失败: %v", err)
	}

	// 三层结构：A > B > C，且 Group 镜像按直接父名派生。
	root := s.nodes[0]
	if root.ID != a.ID || len(root.Children) != 1 || root.Children[0].ID != b.ID {
		t.Fatalf("两层嵌套结构不正确")
	}
	if len(root.Children[0].Children) != 1 || root.Children[0].Children[0].ID != c.ID {
		t.Fatalf("三层嵌套结构不正确")
	}

	// 同层同名文件夹应被拒绝；不同层同名允许。
	if _, err := s.CreateFolder("B", a.ID); !errors.Is(err, ErrDuplicateFolder) {
		t.Errorf("同层重名应被拒绝，实际 %v", err)
	}
	if _, err := s.CreateFolder("B", c.ID); err != nil {
		t.Errorf("不同层同名应被允许，实际 %v", err)
	}
}

func TestCreateFolder_RejectsMissingOrNonFolderParent(t *testing.T) {
	s := NewStoreWithPath(filepath.Join(t.TempDir(), "sessions.json"))
	node, _ := s.UpsertByEndpoint(remote.ConnectConfig{Host: "1.1.1.1", Port: 22}, "")

	if _, err := s.CreateFolder("X", "no-such-id"); !errors.Is(err, ErrParentNotFound) {
		t.Errorf("父不存在应返回 ErrParentNotFound，实际 %v", err)
	}
	if _, err := s.CreateFolder("X", node.ID); !errors.Is(err, ErrNotFolder) {
		t.Errorf("父不是文件夹应返回 ErrNotFolder，实际 %v", err)
	}
}

func TestEnsureFolderByNamePath_IdempotentAndNested(t *testing.T) {
	s := NewStoreWithPath(filepath.Join(t.TempDir(), "sessions.json"))

	id1, err := s.EnsureFolderByNamePath("生产/华东/杭州")
	if err != nil {
		t.Fatalf("建路径失败: %v", err)
	}
	id2, err := s.EnsureFolderByNamePath("生产/华东/杭州")
	if err != nil {
		t.Fatalf("重复建路径失败: %v", err)
	}
	if id1 != id2 {
		t.Errorf("同名路径应幂等，实际 %s vs %s", id1, id2)
	}

	empty, err := s.EnsureFolderByNamePath("   ")
	if err != nil || empty != "" {
		t.Errorf("空路径应返回根（空 ID），实际 %q, %v", empty, err)
	}

	// 只应有一棵 生产 树。
	if len(s.nodes) != 1 || s.nodes[0].Name != "生产" {
		t.Fatalf("根下应只有 生产，实际 %+v", s.nodes)
	}
}

func TestRenameFolder_KeepsChildrenAndRejectsSiblingDuplicate(t *testing.T) {
	s := NewStoreWithPath(filepath.Join(t.TempDir(), "sessions.json"))

	prodID, _ := s.EnsureFolderByNamePath("生产")
	if _, err := s.EnsureFolderByNamePath("测试"); err != nil {
		t.Fatalf("建测试分组失败: %v", err)
	}
	node, _ := s.UpsertByEndpoint(remote.ConnectConfig{Host: "10.0.0.1", Port: 22}, prodID)

	if err := s.RenameNode(prodID, "生产环境"); err != nil {
		t.Fatalf("重命名失败: %v", err)
	}
	// 子节点归属由结构决定，重命名后依然在同一个文件夹里（这是 ID 寻址的关键收益）。
	if loc := findNodeByID(s.nodes, node.ID); loc == nil {
		t.Fatalf("重命名后子节点丢失")
	}
	if s.nodes[0].Name != "生产环境" || len(s.nodes[0].Children) != 1 {
		t.Fatalf("重命名后结构与子节点应保持，实际 %+v", s.nodes[0])
	}
	// Group 镜像随父名更新。
	if got := s.nodes[0].Children[0].Config.Group; got != "生产环境" {
		t.Errorf("重命名文件夹后 Group 镜像未跟随，实际 %q", got)
	}

	if err := s.RenameNode(prodID, "测试"); !errors.Is(err, ErrDuplicateFolder) {
		t.Errorf("与兄弟重名应被拒绝，实际 %v", err)
	}
}

// ── 移动与排序 ──────────────────────────────────────────────

func TestMoveNode_BetweenFoldersAndRoot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	s := seedTestTree(t, path)

	if err := s.MoveNode("s-web1", "f-test", 0); err != nil {
		t.Fatalf("移动到测试分组失败: %v", err)
	}
	if loc := nodeLocation(t, s, "s-web1"); loc != "测试" {
		t.Errorf("应位于 测试，实际 %q", loc)
	}
	if findNodeByID(s.nodes, "s-web1").Config.Host != "10.0.0.1" {
		t.Errorf("移动过程中配置丢失")
	}

	// 落盘一致：重载后位置不变。
	s2 := NewStoreWithPath(path)
	if err := s2.Load(); err != nil {
		t.Fatalf("重载失败: %v", err)
	}
	if loc := nodeLocation(t, s2, "s-web1"); loc != "测试" {
		t.Errorf("重载后应位于 测试，实际 %q", loc)
	}

	if err := s2.MoveNode("s-web1", "", 0); err != nil {
		t.Fatalf("移出到根失败: %v", err)
	}
	if loc := nodeLocation(t, s2, "s-web1"); loc != "<root>" {
		t.Errorf("应位于根，实际 %q", loc)
	}
}

func TestMoveNode_SameFolderStaysPutAndKeepsSingleCopy(t *testing.T) {
	s := seedTestTree(t, filepath.Join(t.TempDir(), "sessions.json"))

	if err := s.MoveNode("s-web1", "f-prod", 0); err != nil {
		t.Fatalf("原地移动失败: %v", err)
	}
	if loc := nodeLocation(t, s, "s-web1"); loc != "生产" {
		t.Errorf("应留在 生产，实际 %q", loc)
	}
	if n := countByID(s.nodes, "s-web1"); n != 1 {
		t.Errorf("原地移动不应产生副本，实际 %d 个", n)
	}
}

func TestMoveNode_RejectsCycle(t *testing.T) {
	s := NewStoreWithPath(filepath.Join(t.TempDir(), "sessions.json"))

	outer, _ := s.CreateFolder("outer", "")
	middle, _ := s.CreateFolder("middle", outer.ID)
	inner, _ := s.CreateFolder("inner", middle.ID)

	cases := []struct {
		name        string
		id          string
		newParentID string
	}{
		{"移到自身", outer.ID, outer.ID},
		{"移到直接子级", outer.ID, middle.ID},
		{"移到孙级", outer.ID, inner.ID},
		{"中间层移到孙级", middle.ID, inner.ID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := s.MoveNode(tc.id, tc.newParentID, 0); !errors.Is(err, ErrCycle) {
				t.Errorf("应返回 ErrCycle，实际 %v", err)
			}
		})
	}

	// 反向（子级移到祖先）是合法的。
	if err := s.MoveNode(inner.ID, "", 0); err != nil {
		t.Errorf("子级移到根应允许，实际 %v", err)
	}
}

func TestMoveNode_IndexClampedAndOrderPersisted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	s := NewStoreWithPath(path)

	for _, name := range []string{"c1", "c2", "c3"} {
		if _, err := s.CreateConnection(remote.ConnectConfig{Host: name, Port: 22}, ""); err != nil {
			t.Fatalf("建连接 %s 失败: %v", name, err)
		}
	}
	// 把最后一个挪到最前。
	if err := s.MoveNode(s.nodes[2].ID, "", 0); err != nil {
		t.Fatalf("排序移动失败: %v", err)
	}
	got := []string{s.nodes[0].Name, s.nodes[1].Name, s.nodes[2].Name}
	want := []string{"c3", "c1", "c2"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("顺序应为 %v，实际 %v", want, got)
		}
	}

	// 越界索引按追加处理。
	if err := s.MoveNode(s.nodes[0].ID, "", 999); err != nil {
		t.Fatalf("越界索引应被钳制，实际 %v", err)
	}
	if s.nodes[2].Name != "c3" {
		t.Errorf("越界索引应追加到末尾，实际 %v", []string{s.nodes[0].Name, s.nodes[1].Name, s.nodes[2].Name})
	}

	// 顺序必须持久化：重载后保持，不得被任何字母序重排覆盖。
	s2 := NewStoreWithPath(path)
	if err := s2.Load(); err != nil {
		t.Fatalf("重载失败: %v", err)
	}
	if s2.nodes[2].Name != "c3" {
		t.Errorf("重载后顺序丢失: %v", []string{s2.nodes[0].Name, s2.nodes[1].Name, s2.nodes[2].Name})
	}
}

func TestReorderNodes_ValidatesPermutation(t *testing.T) {
	s := NewStoreWithPath(filepath.Join(t.TempDir(), "sessions.json"))
	a, _ := s.CreateConnection(remote.ConnectConfig{Host: "a", Port: 22}, "")
	b, _ := s.CreateConnection(remote.ConnectConfig{Host: "b", Port: 22}, "")

	if err := s.ReorderNodes("", []string{b.ID, a.ID}); err != nil {
		t.Fatalf("合法排序失败: %v", err)
	}
	if s.nodes[0].ID != b.ID {
		t.Errorf("排序未生效")
	}

	if err := s.ReorderNodes("", []string{a.ID}); err == nil {
		t.Errorf("数量不匹配应被拒绝")
	}
	if err := s.ReorderNodes("", []string{a.ID, a.ID}); err == nil {
		t.Errorf("重复 ID 应被拒绝")
	}
	if err := s.ReorderNodes("", []string{a.ID, "ghost"}); err == nil {
		t.Errorf("未知 ID 应被拒绝")
	}
}

// ── 连接去重与复制 ──────────────────────────────────────────

func TestCreateConnection_RejectsDuplicateEndpoint(t *testing.T) {
	s := NewStoreWithPath(filepath.Join(t.TempDir(), "sessions.json"))
	if _, err := s.CreateConnection(remote.ConnectConfig{Host: "1.1.1.1", Port: 22}, ""); err != nil {
		t.Fatalf("首次创建失败: %v", err)
	}
	if _, err := s.CreateConnection(remote.ConnectConfig{Host: "1.1.1.1", Port: 22}, ""); !errors.Is(err, ErrDuplicateEndpoint) {
		t.Errorf("同端点应被拒绝，实际 %v", err)
	}
	// 同主机不同协议/端口可共存。
	if _, err := s.CreateConnection(remote.ConnectConfig{Host: "1.1.1.1", Port: 23, Protocol: remote.ProtocolTelnet}, ""); err != nil {
		t.Errorf("同主机不同协议应允许，实际 %v", err)
	}
	if _, err := s.CreateConnection(remote.ConnectConfig{Host: "1.1.1.1", Port: 2222}, ""); err != nil {
		t.Errorf("同主机不同端口应允许，实际 %v", err)
	}
}

func TestUpdateConnection_DuplicateErrorLeavesStateIntact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	s := seedTestTree(t, path)

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}

	web1 := findNodeByID(s.nodes, "s-web1")
	conflict := *web1.Config
	conflict.Host = "10.0.0.2" // 与 db-1 撞端点

	if err := s.UpdateConnection("s-web1", conflict); !errors.Is(err, ErrDuplicateEndpoint) {
		t.Fatalf("期望端点冲突错误，实际 %v", err)
	}

	if loc := nodeLocation(t, s, "s-web1"); loc != "生产" {
		t.Errorf("失败更新后节点位置被改动: %q", loc)
	}
	if got := findNodeByID(s.nodes, "s-web1").Config.Host; got != "10.0.0.1" {
		t.Errorf("失败更新改动了内存配置: host=%q", got)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Errorf("失败更新改动了磁盘文件")
	}
}

func TestUpdateConnection_NotFoundLeavesStateIntact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	s := seedTestTree(t, path)
	before, _ := os.ReadFile(path)

	if err := s.UpdateConnection("no-such-id", remote.ConnectConfig{Host: "9.9.9.9", Port: 22}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("期望 ErrNotFound，实际 %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Errorf("失败更新改动了磁盘文件")
	}
	if len(s.nodes) != 3 {
		t.Errorf("失败更新改动了内存树: %d 个根节点", len(s.nodes))
	}
}

func TestDuplicateConnection_CreatesIndependentCopyInSameFolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	s := seedTestTree(t, path)

	web1 := findNodeByID(s.nodes, "s-web1")
	web1.Config.Bastion = &remote.ConnectConfig{Host: "10.0.0.254", Port: 22, User: "jump", Password: "jp"}
	if err := s.Save(); err != nil {
		t.Fatalf("seed 失败: %v", err)
	}

	dup, err := s.DuplicateConnection("s-web1")
	if err != nil {
		t.Fatalf("复制失败: %v", err)
	}

	folder := findNodeByID(s.nodes, "f-prod")
	idxSrc, idxCopy := -1, -1
	for i, n := range folder.Children {
		if n.ID == "s-web1" {
			idxSrc = i
		}
		if n.ID == dup.ID {
			idxCopy = i
		}
	}
	if idxSrc < 0 || idxCopy != idxSrc+1 {
		t.Fatalf("副本应紧跟源节点之后，src=%d copy=%d", idxSrc, idxCopy)
	}
	if dup.Name != "web-1-副本" {
		t.Errorf("副本名应为 web-1-副本，实际 %q", dup.Name)
	}
	cfg := findNodeByID(s.nodes, dup.ID).Config
	if cfg.Host != "10.0.0.1" || cfg.User != "root" || cfg.Password != "p1" || cfg.RootPassword != "rp1" {
		t.Errorf("副本配置不完整: %+v", cfg)
	}
	if cfg.Bastion == nil || cfg.Bastion.Host != "10.0.0.254" {
		t.Fatalf("副本丢失跳板机配置")
	}

	// 深拷贝：改副本跳板机不影响源节点。
	cfg.Bastion.Host = "9.9.9.9"
	if findNodeByID(s.nodes, "s-web1").Config.Bastion.Host != "10.0.0.254" {
		t.Errorf("副本与源共享跳板机状态")
	}

	// 落盘可重载。
	s2 := NewStoreWithPath(path)
	if err := s2.Load(); err != nil {
		t.Fatalf("重载失败: %v", err)
	}
	loaded := findNodeByID(s2.nodes, dup.ID)
	if loaded == nil || loaded.Config == nil || loaded.Config.Bastion == nil {
		t.Fatalf("副本未持久化")
	}
}

func TestDuplicateConnection_AllowsSameEndpoint(t *testing.T) {
	s := seedTestTree(t, filepath.Join(t.TempDir(), "sessions.json"))

	if _, err := s.DuplicateConnection("s-web1"); err != nil {
		t.Fatalf("同端点复制应放行，实际 %v", err)
	}
	if n := countByHost(s.nodes, "10.0.0.1"); n != 2 {
		t.Errorf("期望 2 条同主机连接，实际 %d", n)
	}
}

func TestDuplicateConnection_RootCopyStaysAtRoot(t *testing.T) {
	s := seedTestTree(t, filepath.Join(t.TempDir(), "sessions.json"))

	dup, err := s.DuplicateConnection("s-db1")
	if err != nil {
		t.Fatalf("复制失败: %v", err)
	}
	if loc := nodeLocation(t, s, dup.ID); loc != "<root>" {
		t.Errorf("根节点副本应留在根，实际 %q", loc)
	}
}

func TestDuplicateConnection_NotFoundLeavesStateIntact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	s := seedTestTree(t, path)
	before, _ := os.ReadFile(path)

	if _, err := s.DuplicateConnection("no-such-id"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("期望 ErrNotFound，实际 %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Errorf("失败复制改动了磁盘文件")
	}
}

// ── 快照隔离 ────────────────────────────────────────────────

func TestSnapshotIsDeepCopy(t *testing.T) {
	s := seedTestTree(t, filepath.Join(t.TempDir(), "sessions.json"))

	snap := s.Snapshot()
	web1 := findNodeByID(snap, "s-web1")
	web1.Name = "tampered"
	web1.Config.Host = "0.0.0.0"
	web1.Config.Bastion = &remote.ConnectConfig{Host: "attacker"}

	// 篡改快照不得影响内部状态。
	if got := findNodeByID(s.nodes, "s-web1").Name; got != "web-1" {
		t.Errorf("快照被篡改后影响了内部状态: name=%q", got)
	}
	if got := findNodeByID(s.nodes, "s-web1").Config.Host; got != "10.0.0.1" {
		t.Errorf("快照被篡改后影响了内部配置: host=%q", got)
	}
}

func TestFindByEndpoint_NormalizesProtocol(t *testing.T) {
	s := NewStoreWithPath(filepath.Join(t.TempDir(), "sessions.json"))
	if _, err := s.UpsertByEndpoint(remote.ConnectConfig{Host: "1.1.1.1", Port: 22}, ""); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	// 存储里 Protocol 已被归一化为 ssh，用空协议查询也应命中。
	if got := s.FindByEndpoint("", "1.1.1.1", 22); got == nil {
		t.Errorf("空协议查询应按 SSH 命中")
	}
	if got := s.FindByEndpoint(remote.ProtocolTelnet, "1.1.1.1", 22); got != nil {
		t.Errorf("不同协议不应命中")
	}
}

// ── 持久化：对账、备份、无变更跳过 ──────────────────────────

// legacySample 是真实 sessions.json 的形态：文件夹下的连接没有 group 字段、
// 根连接的 config.name 缺失、config 里带下划线的 root_password。
const legacySample = `[
  {
    "id": "f-demo",
    "name": "测试设备(演示)",
    "type": "folder",
    "children": [
      {
        "id": "s-core",
        "name": "core-sw-01",
        "type": "session",
        "config": {
          "name": "core-sw-01",
          "host": "10.0.0.11",
          "port": 22,
          "user": "admin",
          "password": "pw",
          "root_password": "",
          "bastion": null
        }
      }
    ]
  },
  {
    "id": "s-root",
    "name": "39.108.66.227",
    "type": "session",
    "config": {
      "host": "39.108.66.227",
      "port": 22,
      "user": "root",
      "password": "pw2",
      "root_password": "",
      "bastion": null
    }
  }
]`

func TestLoadRepairsLegacyFileAndBacksUp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")
	if err := os.WriteFile(path, []byte(legacySample), 0o644); err != nil {
		t.Fatalf("写入旧样本失败: %v", err)
	}

	s := NewStoreWithPath(path)
	if err := s.Load(); err != nil {
		t.Fatalf("加载旧文件失败: %v", err)
	}

	// 结构保持不变：文件夹与连接都还在，层级正确。
	if len(s.nodes) != 2 {
		t.Fatalf("期望 2 个根节点，实际 %d", len(s.nodes))
	}
	folder := findNodeByID(s.nodes, "f-demo")
	if folder == nil || folder.Type != KindFolder || len(folder.Children) != 1 {
		t.Fatalf("文件夹结构在迁移中损坏: %+v", folder)
	}
	// 用户可见的显示名与密码必须原样保留。
	core := findNodeByID(s.nodes, "s-core")
	if core.Config.Password != "pw" || core.Config.User != "admin" {
		t.Errorf("迁移改动了连接凭据: %+v", core.Config)
	}
	// 空 Children 归一化为 nil（序列化稳定）。
	if folder.Children == nil {
		t.Errorf("文件夹不应丢失 children")
	}

	// Group 镜像按结构重建。
	if got := core.Config.Group; got != "测试设备(演示)" {
		t.Errorf("文件夹内连接的 group 镜像应为直接父名，实际 %q", got)
	}
	if got := findNodeByID(s.nodes, "s-root").Config.Group; got != "" {
		t.Errorf("根连接的 group 镜像应为空，实际 %q", got)
	}
	// 缺失的 config.name 被补为显示名。
	if got := findNodeByID(s.nodes, "s-root").Config.Name; got != "39.108.66.227" {
		t.Errorf("config.name 应补为显示名，实际 %q", got)
	}

	// 修复前先备份，用户可回退。
	entries, _ := os.ReadDir(dir)
	backups := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "sessions.json.bak-") {
			backups++
			raw, _ := os.ReadFile(filepath.Join(dir, e.Name()))
			if string(raw) != legacySample {
				t.Errorf("备份内容不是修复前的原始文件")
			}
		}
	}
	if backups != 1 {
		t.Errorf("期望 1 个备份文件，实际 %d", backups)
	}
}

func TestLoadReconcileIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.json")
	os.WriteFile(path, []byte(legacySample), 0o644)

	first := NewStoreWithPath(path)
	if err := first.Load(); err != nil {
		t.Fatalf("首次加载失败: %v", err)
	}
	afterFirst, _ := os.ReadFile(path)

	second := NewStoreWithPath(path)
	if err := second.Load(); err != nil {
		t.Fatalf("二次加载失败: %v", err)
	}
	afterSecond, _ := os.ReadFile(path)

	if string(afterFirst) != string(afterSecond) {
		t.Errorf("对账不幂等：第二次加载改动了文件")
	}

	entries, _ := os.ReadDir(dir)
	backups := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "sessions.json.bak-") {
			backups++
		}
	}
	if backups != 1 {
		t.Errorf("已规范的文件不应再次触发备份，实际 %d 个备份", backups)
	}
}

func TestSaveSkipsWriteWhenUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	s := NewStoreWithPath(path)
	if _, err := s.CreateConnection(remote.ConnectConfig{Host: "1.1.1.1", Port: 22}, ""); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat 失败: %v", err)
	}

	// 无变更的 Save 不应触碰文件（mtime 不变）。
	if err := s.Save(); err != nil {
		t.Fatalf("Save 失败: %v", err)
	}
	after, _ := os.Stat(path)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("无变更时不应写盘，mtime 从 %v 变为 %v", before.ModTime(), after.ModTime())
	}
}

func TestLoadCreatesFileWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	s := NewStoreWithPath(path)
	if err := s.Load(); err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("缺失的文件应被创建: %v", err)
	}
}

// ── 测试辅助 ────────────────────────────────────────────────

// seedTestTree 构造标准测试树并落盘：
//
//	生产(folder) ── web-1(10.0.0.1)
//	测试(folder) ── (空)
//	db-1(10.0.0.2，根)
func seedTestTree(t *testing.T, path string) *Store {
	t.Helper()
	s := NewStoreWithPath(path)
	s.nodes = []*Node{
		{ID: "f-prod", Name: "生产", Type: KindFolder, Children: []*Node{
			{ID: "s-web1", Name: "web-1", Type: KindConnection, Config: &remote.ConnectConfig{
				Name: "web-1", Host: "10.0.0.1", Port: 22, User: "root", Password: "p1", RootPassword: "rp1",
			}},
		}},
		{ID: "f-test", Name: "测试", Type: KindFolder},
		{ID: "s-db1", Name: "db-1", Type: KindConnection, Config: &remote.ConnectConfig{
			Name: "db-1", Host: "10.0.0.2", Port: 22, User: "root",
		}},
	}
	if err := s.Save(); err != nil {
		t.Fatalf("seed 失败: %v", err)
	}
	return s
}

func findNodeByID(nodes []*Node, id string) *Node {
	for _, n := range nodes {
		if n.ID == id {
			return n
		}
		if n.Type == KindFolder {
			if found := findNodeByID(n.Children, id); found != nil {
				return found
			}
		}
	}
	return nil
}

func countByID(nodes []*Node, id string) int {
	count := 0
	for _, n := range nodes {
		if n.ID == id {
			count++
		}
		if n.Type == KindFolder {
			count += countByID(n.Children, id)
		}
	}
	return count
}

func countByHost(nodes []*Node, host string) int {
	count := 0
	for _, n := range nodes {
		if n.Type == KindConnection && n.Config != nil && n.Config.Host == host {
			count++
		}
		if n.Type == KindFolder {
			count += countByHost(n.Children, host)
		}
	}
	return count
}

// nodeLocation 返回节点所在位置的描述（根节点为 "<root>"，否则为所属文件夹名）。
func nodeLocation(t *testing.T, s *Store, id string) string {
	t.Helper()
	for _, n := range s.nodes {
		if n.ID == id {
			return "<root>"
		}
		if n.Type == KindFolder && findNodeByID(n.Children, id) != nil {
			return n.Name
		}
	}
	t.Fatalf("节点 %s 不在树中", id)
	return ""
}
