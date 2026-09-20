package main

import (
	"path/filepath"
	"testing"

	"opscopilot/pkg/connectionstore"
)

// findFolderByName 在树快照里按名字找文件夹（不存在返回 nil）。
func findFolderByName(nodes []*connectionstore.Node, name string) *connectionstore.Node {
	for _, n := range nodes {
		if n.Type == connectionstore.KindFolder && n.Name == name {
			return n
		}
	}
	return nil
}

// TestConnectAutosaveRepro_ReconnectReplay 是用户报告问题的回归闸门，走真实链路
// App.autoSaveConnection（ConnectWithID / ReconnectSession / DuplicateTerminalSession
// 共用的自动落库钩子）。
//
// 症状一（改名被还原）：连接改名后重连，重放的旧配置把显示名改回旧名。
// 症状二（连接回到旧分组）：分组改名后批量连接/重连，旧配置按旧分组名
// 重建文件夹并把连接拽回去——"只有一个连接呆在新分组，其余都回到旧分组"。
//
// 旧实现（UpsertByEndpoint 按端点整节点重写 + 按名字解析分组）在本用例上
// 真实复现过两个症状，见提交历史。
func TestConnectAutosaveRepro_ReconnectReplay(t *testing.T) {
	workDir := t.TempDir()
	app, savedMgr := newSavedSessionTestApp(t, workDir)

	// 用户第一次连接：web1，保存到分组"分组A"（连接表单的保存到分组输入）。
	first := ConnectConfig{Name: "web1", Protocol: "ssh", Host: "10.0.0.1", Port: 22, User: "ops", Password: "pw", Group: "分组A"}
	app.autoSaveConnection(first)

	// 用户随后在会话树里：分组"分组A"改名"分组B"，连接"web1"改名"生产-web1"。
	folder := findFolderByName(savedMgr.Snapshot(), "分组A")
	if folder == nil {
		t.Fatalf("种子文件夹不存在")
	}
	if len(folder.Children) == 0 {
		t.Fatalf("种子连接不存在")
	}
	connID := folder.Children[0].ID
	if err := savedMgr.RenameNode(folder.ID, "分组B"); err != nil {
		t.Fatalf("重命名分组: %v", err)
	}
	if err := savedMgr.RenameNode(connID, "生产-web1"); err != nil {
		t.Fatalf("重命名连接: %v", err)
	}

	// 连接后来断开，用户点"重连"（或批量重连/复制标签）：重放连接时捕获的旧配置。
	app.autoSaveConnection(first)

	snap := savedMgr.Snapshot()
	if revived := findFolderByName(snap, "分组A"); revived != nil {
		t.Errorf("症状二复现：旧分组「分组A」被重连重放复活，其中连接数=%d", len(revived.Children))
	}
	folderB := findFolderByName(snap, "分组B")
	if folderB == nil {
		t.Fatalf("新分组「分组B」消失")
	}
	if len(folderB.Children) != 1 {
		t.Fatalf("症状二复现：连接被拽离新分组「分组B」，剩 %d 个", len(folderB.Children))
	}
	if folderB.Children[0].ID != connID {
		t.Errorf("连接节点被重建，ID 从 %q 变为 %q", connID, folderB.Children[0].ID)
	}
	if folderB.Children[0].Name != "生产-web1" {
		t.Errorf("症状一复现：改名被还原：got %q, want %q", folderB.Children[0].Name, "生产-web1")
	}

	// 落盘一致：等价于应用重启后看到的树。
	reloaded := connectionstore.NewStoreWithPath(filepath.Join(workDir, "sessions.json"))
	if err := reloaded.Load(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if revived := findFolderByName(reloaded.Snapshot(), "分组A"); revived != nil {
		t.Errorf("落盘后旧分组仍被复活")
	}
}
