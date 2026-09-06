package sessionmanager

import (
	"opscopilot/pkg/sshclient"
	"os"
	"path/filepath"
	"testing"
)

func TestNewManager(t *testing.T) {
	m := NewManager()
	if m.Sessions == nil {
		t.Error("Sessions should be initialized")
	}
	if m.filePath != "sessions.json" {
		t.Errorf("Expected default filePath 'sessions.json', got %s", m.filePath)
	}
}

func TestUpsertSession(t *testing.T) {
	// Setup temporary file
	tmpFile := filepath.Join(os.TempDir(), "test_sessions.json")
	defer os.Remove(tmpFile)

	m := NewManager()
	m.filePath = tmpFile

	config := sshclient.ConnectConfig{
		Host: "192.168.1.1",
		User: "root",
		Port: 22,
	}

	// Test 1: Add new session (Root)
	err := m.Upsert(config, "")
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	if len(m.Sessions) != 1 {
		t.Fatalf("Expected 1 session, got %d", len(m.Sessions))
	}
	if m.Sessions[0].Name != "192.168.1.1" {
		t.Errorf("Expected name '192.168.1.1', got %s", m.Sessions[0].Name)
	}
	if m.Sessions[0].Type != TypeSession {
		t.Errorf("Expected type 'session', got %s", m.Sessions[0].Type)
	}

	// Test 2: Update existing session
	config.User = "admin"
	err = m.Upsert(config, "")
	if err != nil {
		t.Fatalf("Upsert update failed: %v", err)
	}
	if len(m.Sessions) != 1 {
		t.Fatalf("Expected 1 session after update, got %d", len(m.Sessions))
	}
	if m.Sessions[0].Config.User != "admin" {
		t.Errorf("Expected user 'admin', got %s", m.Sessions[0].Config.User)
	}

	// Test 3: Move to Group
	err = m.Upsert(config, "Prod")
	if err != nil {
		t.Fatalf("Upsert move failed: %v", err)
	}

	// Should have 1 folder in root, and session inside it
	if len(m.Sessions) != 1 {
		t.Fatalf("Expected 1 node (folder) in root, got %d", len(m.Sessions))
	}
	folder := m.Sessions[0]
	if folder.Type != TypeFolder || folder.Name != "Prod" {
		t.Errorf("Expected folder 'Prod', got %s (%s)", folder.Name, folder.Type)
	}
	if len(folder.Children) != 1 {
		t.Fatalf("Expected 1 child in folder, got %d", len(folder.Children))
	}
	if folder.Children[0].Config.Host != "192.168.1.1" {
		t.Errorf("Expected session in folder")
	}
}

func TestDeleteSession(t *testing.T) {
	tmpFile := filepath.Join(os.TempDir(), "test_sessions_delete.json")
	defer os.Remove(tmpFile)

	m := NewManager()
	m.filePath = tmpFile

	config := sshclient.ConnectConfig{Host: "1.1.1.1"}
	m.Upsert(config, "")

	id := m.Sessions[0].ID

	err := m.DeleteSession(id)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if len(m.Sessions) != 0 {
		t.Errorf("Expected 0 sessions, got %d", len(m.Sessions))
	}
}

func TestRenameSession(t *testing.T) {
	tmpFile := filepath.Join(os.TempDir(), "test_sessions_rename.json")
	defer os.Remove(tmpFile)

	m := NewManager()
	m.filePath = tmpFile

	config := sshclient.ConnectConfig{Host: "1.1.1.1"}
	m.Upsert(config, "")

	id := m.Sessions[0].ID

	err := m.RenameSession(id, "NewName")
	if err != nil {
		t.Fatalf("Rename failed: %v", err)
	}

	if m.Sessions[0].Name != "NewName" {
		t.Errorf("Expected name 'NewName', got %s", m.Sessions[0].Name)
	}
	if m.Sessions[0].Config.Name != "NewName" {
		t.Errorf("Expected config name 'NewName', got %s", m.Sessions[0].Config.Name)
	}
}

func TestUpsertPreservesRenamedSessionName(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "sessions.json")
	m := NewManagerWithPath(tmpFile)

	config := sshclient.ConnectConfig{
		Host: "10.0.0.1",
		Port: 22,
		User: "root",
	}
	if err := m.Upsert(config, ""); err != nil {
		t.Fatalf("initial Upsert failed: %v", err)
	}

	id := m.Sessions[0].ID
	if err := m.RenameSession(id, "web-primary"); err != nil {
		t.Fatalf("Rename failed: %v", err)
	}

	// Simulate reconnecting with an older saved config whose Name was empty.
	if err := m.Upsert(config, ""); err != nil {
		t.Fatalf("reconnect Upsert failed: %v", err)
	}

	if len(m.Sessions) != 1 {
		t.Fatalf("Expected one session, got %d", len(m.Sessions))
	}
	if m.Sessions[0].ID != id {
		t.Errorf("Expected session ID %q to be reused, got %q", id, m.Sessions[0].ID)
	}
	if m.Sessions[0].Name != "web-primary" {
		t.Errorf("Expected renamed display name to be preserved, got %q", m.Sessions[0].Name)
	}
	if m.Sessions[0].Config.Name != "web-primary" {
		t.Errorf("Expected config name to stay in sync, got %q", m.Sessions[0].Config.Name)
	}
}

func TestUpsertUsesConfiguredDisplayName(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "sessions.json")
	m := NewManagerWithPath(tmpFile)

	config := sshclient.ConnectConfig{
		Name: "database-primary",
		Host: "10.0.0.2",
		Port: 22,
		User: "root",
	}
	if err := m.Upsert(config, ""); err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	if m.Sessions[0].Name != "database-primary" {
		t.Errorf("Expected configured display name, got %q", m.Sessions[0].Name)
	}
	if m.Sessions[0].Config.Name != "database-primary" {
		t.Errorf("Expected config name to be persisted, got %q", m.Sessions[0].Config.Name)
	}
}

func TestPersistence(t *testing.T) {
	tmpFile := filepath.Join(os.TempDir(), "test_sessions_persist.json")
	defer os.Remove(tmpFile)

	m1 := NewManager()
	m1.filePath = tmpFile
	m1.Upsert(sshclient.ConnectConfig{Host: "1.1.1.1"}, "GroupA")

	// Load with new manager
	m2 := NewManager()
	m2.filePath = tmpFile
	err := m2.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(m2.Sessions) != 1 {
		t.Fatalf("Expected 1 folder loaded, got %d", len(m2.Sessions))
	}
	if m2.Sessions[0].Name != "GroupA" {
		t.Errorf("Expected group 'GroupA', got %s", m2.Sessions[0].Name)
	}
	if len(m2.Sessions[0].Children) != 1 {
		t.Errorf("Expected 1 child in group")
	}
}

func TestRecursiveDelete(t *testing.T) {
	tmpFile := filepath.Join(os.TempDir(), "test_sessions_recursive_delete.json")
	defer os.Remove(tmpFile)

	m := NewManager()
	m.filePath = tmpFile

	// Create Group -> Session
	m.Upsert(sshclient.ConnectConfig{Host: "1.1.1.1"}, "GroupA")

	// Find Group ID
	groupID := m.Sessions[0].ID

	// Delete Group
	err := m.DeleteSession(groupID)
	if err != nil {
		t.Fatalf("Delete group failed: %v", err)
	}

	if len(m.Sessions) != 0 {
		t.Errorf("Expected root empty after group delete, got %d", len(m.Sessions))
	}
}

// --- 会话移动/编辑业务流程防护用例 ---
// 覆盖用户操作流：拖拽移动会话、编辑保存失败不留脏状态。
// 背景（Issue #70/#71）：UpdateSession 曾"先摘节点后校验"，失败时内存树被污染，
// 前端 5 秒轮询把坏树当最新数据渲染，用户看到"会话消失"。

// seedMoveTestTree 构造标准测试树并落盘：
//   生产(folder) ── web-1(session, 10.0.0.1)
//   测试(folder) ── (空)
//   db-1(session, 10.0.0.2, 根目录)
func seedMoveTestTree(t *testing.T, filePath string) *Manager {
	t.Helper()
	m := NewManagerWithPath(filePath)
	m.Sessions = []*Session{
		{ID: "f-prod", Name: "生产", Type: TypeFolder, Children: []*Session{
			{ID: "s-web1", Name: "web-1", Type: TypeSession, Config: &sshclient.ConnectConfig{
				Host: "10.0.0.1", Port: 22, User: "root", Password: "p1", RootPassword: "rp1",
			}},
		}},
		{ID: "f-test", Name: "测试", Type: TypeFolder, Children: []*Session{}},
		{ID: "s-db1", Name: "db-1", Type: TypeSession, Config: &sshclient.ConnectConfig{
			Host: "10.0.0.2", Port: 22, User: "root",
		}},
	}
	if err := m.Save(); err != nil {
		t.Fatalf("seed save: %v", err)
	}
	return m
}

// findNodeByID 在树中递归查找节点。
func findNodeByID(nodes []*Session, id string) *Session {
	for _, node := range nodes {
		if node.ID == id {
			return node
		}
		if node.Type == TypeFolder {
			if found := findNodeByID(node.Children, id); found != nil {
				return found
			}
		}
	}
	return nil
}

// sessionLocation 返回 session 所在位置的描述（根目录 / 文件夹名）。
func sessionLocation(t *testing.T, m *Manager, id string) string {
	t.Helper()
	for _, node := range m.Sessions {
		if node.ID == id {
			return "<root>"
		}
		if node.Type == TypeFolder {
			if findNodeByID(node.Children, id) != nil {
				return node.Name
			}
		}
	}
	t.Fatalf("session %s not found in tree", id)
	return ""
}

// TestUpdateSession_MoveBetweenFolders 覆盖用户流程："把会话从文件夹 A 拖到文件夹 B"。
// 移动后必须恰好存在一个副本，位于目标文件夹内；重载磁盘后结果一致。
func TestUpdateSession_MoveBetweenFolders(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "sessions.json")
	m := seedMoveTestTree(t, tmpFile)

	web1 := findNodeByID(m.Sessions, "s-web1")
	if web1 == nil || web1.Config == nil {
		t.Fatalf("seed failed: s-web1 missing")
	}
	movedCfg := *web1.Config

	if err := m.UpdateSession("s-web1", movedCfg, "测试"); err != nil {
		t.Fatalf("move to 测试 failed: %v", err)
	}

	if loc := sessionLocation(t, m, "s-web1"); loc != "测试" {
		t.Errorf("session should be in 测试, got %q", loc)
	}
	if findNodeByID(m.Sessions, "s-web1").Config.Host != "10.0.0.1" {
		t.Errorf("config lost during move")
	}

	// 磁盘一致性：重新加载后位置不变。
	m2 := NewManagerWithPath(tmpFile)
	if err := m2.Load(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if loc := sessionLocation(t, m2, "s-web1"); loc != "测试" {
		t.Errorf("after reload session should be in 测试, got %q", loc)
	}
}

// TestUpdateSession_SameFolderStaysPut 覆盖用户流程："把会话拖到它自己所在的文件夹"。
// 结果必须是原地不动、单副本——绝不能被弹到根目录（Issue #70 的可见症状）。
func TestUpdateSession_SameFolderStaysPut(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "sessions.json")
	m := seedMoveTestTree(t, tmpFile)

	web1 := findNodeByID(m.Sessions, "s-web1")
	movedCfg := *web1.Config
	movedCfg.Group = "生产"

	if err := m.UpdateSession("s-web1", movedCfg, "生产"); err != nil {
		t.Fatalf("same-folder update failed: %v", err)
	}

	if loc := sessionLocation(t, m, "s-web1"); loc != "生产" {
		t.Errorf("session should stay in 生产, got %q", loc)
	}
	// 全树恰好一个 s-web1，不允许移动产生副本。
	count := 0
	var countNode func(nodes []*Session)
	countNode = func(nodes []*Session) {
		for _, n := range nodes {
			if n.ID == "s-web1" {
				count++
			}
			if n.Type == TypeFolder {
				countNode(n.Children)
			}
		}
	}
	countNode(m.Sessions)
	if count != 1 {
		t.Errorf("expected exactly 1 copy of s-web1, got %d", count)
	}
}

// TestUpdateSession_MoveToRoot 覆盖用户流程："把会话拖到树空白处移出分组"。
func TestUpdateSession_MoveToRoot(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "sessions.json")
	m := seedMoveTestTree(t, tmpFile)

	web1 := findNodeByID(m.Sessions, "s-web1")
	movedCfg := *web1.Config
	movedCfg.Group = ""

	if err := m.UpdateSession("s-web1", movedCfg, ""); err != nil {
		t.Fatalf("ungroup failed: %v", err)
	}

	if loc := sessionLocation(t, m, "s-web1"); loc != "<root>" {
		t.Errorf("session should be at root after ungroup, got %q", loc)
	}
}

// TestUpdateSession_DuplicateErrorLeavesStateIntact 防护编辑流程的数据一致性：
// 改主机地址撞上已有会话时必须报错，且内存树与磁盘文件都不能有任何变化——
// 前端轮询渲染的是内存树，脏状态会让会话从列表里"消失"。
func TestUpdateSession_DuplicateErrorLeavesStateIntact(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "sessions.json")
	m := seedMoveTestTree(t, tmpFile)

	before, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("read before: %v", err)
	}
	beforeTree := string(before)

	// 把 web-1 的主机改成与 db-1 相同 → 重名冲突。
	web1 := findNodeByID(m.Sessions, "s-web1")
	conflictCfg := *web1.Config
	conflictCfg.Host = "10.0.0.2"

	err = m.UpdateSession("s-web1", conflictCfg, "生产")
	if err == nil {
		t.Fatalf("expected duplicate-host error, got nil")
	}

	// 内存树必须完好：web-1 仍在生产文件夹、配置仍是原主机。
	if loc := sessionLocation(t, m, "s-web1"); loc != "生产" {
		t.Errorf("session vanished from 生产 after failed update, now at %q", loc)
	}
	if got := findNodeByID(m.Sessions, "s-web1").Config.Host; got != "10.0.0.1" {
		t.Errorf("in-memory config mutated by failed update: host=%q", got)
	}
	// 磁盘文件必须逐字节不变。
	after, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(after) != beforeTree {
		t.Errorf("sessions.json changed despite failed update")
	}
}

// TestUpdateSession_NotFoundLeavesStateIntact 防护异常路径：更新不存在的会话
// 必须报错且不留任何状态变化。
func TestUpdateSession_NotFoundLeavesStateIntact(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "sessions.json")
	m := seedMoveTestTree(t, tmpFile)

	before, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("read before: %v", err)
	}

	err = m.UpdateSession("no-such-id", sshclient.ConnectConfig{Host: "9.9.9.9", Port: 22}, "")
	if err == nil {
		t.Fatalf("expected session-not-found error, got nil")
	}

	after, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(after) != string(before) {
		t.Errorf("sessions.json changed despite not-found error")
	}
	if len(m.Sessions) != 3 {
		t.Errorf("in-memory tree mutated by not-found error: %d top-level nodes", len(m.Sessions))
	}
}
