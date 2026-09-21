package main

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"opscopilot/pkg/connectionstore"
	"opscopilot/pkg/remote"
)

// 本文件覆盖会话树"出参"方向的边界契约——GetConnectionTree 曾直接返回
// connectionstore.Node（Config 为 remote.ConnectConfig，root_password /
// host_key 下划线 tag），前端按驼峰读取永远拿到空串：
//
//   - 编辑弹窗里 root 密码显示为空，用户只改个名字保存，整体替换把磁盘上
//     已存的 root 密码清掉；
//   - 双击树节点连接时，下划线键与入参侧驼峰 tag 不匹配被静默丢弃，
//     sudo 提权与固定主机密钥校验（ssh.FixedHostKey）都不生效。
//
// 纯 Go 调用（不经 JSON）察觉不到这类接缝，所以这里的测试全部走
// json.Marshal → json.Unmarshal 的线格式往返。

// marshalTree 把 Wails 出参树按线格式序列化，模拟 Wails 把返回值交给前端。
func marshalTree(t *testing.T, nodes []*SessionTreeNode) []byte {
	t.Helper()
	data, err := json.Marshal(nodes)
	if err != nil {
		t.Fatalf("marshal tree: %v", err)
	}
	return data
}

// wireNode 是前端视角的树节点：字段名取自 frontend-shell 的 SessionNode /
// ConnectionConfig 类型（驼峰），完全不看 Go 侧结构体。
type wireNode struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Type     string     `json:"type"`
	Children []wireNode `json:"children,omitempty"`
	Config   *struct {
		Protocol     string    `json:"protocol,omitempty"`
		Host         string    `json:"host"`
		Port         int       `json:"port"`
		User         string    `json:"user"`
		Password     string    `json:"password"`
		RootPassword string    `json:"rootPassword"`
		HostKey      string    `json:"hostKey,omitempty"`
		Bastion      *struct { //nolint:revive // 与外层同构，仅为读取线格式
			Host     string `json:"host"`
			Port     int    `json:"port"`
			User     string `json:"user"`
			Password string `json:"password"`
		} `json:"bastion"`
	} `json:"config,omitempty"`
}

func parseWireTree(t *testing.T, data []byte) []wireNode {
	t.Helper()
	var out []wireNode
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal wire tree: %v\njson: %s", err, data)
	}
	return out
}

func findWireSession(t *testing.T, nodes []wireNode, id string) *wireNode {
	t.Helper()
	for i := range nodes {
		if nodes[i].ID == id {
			return &nodes[i]
		}
		for j := range nodes[i].Children {
			if found := findWireSession(t, []wireNode{nodes[i].Children[j]}, id); found != nil {
				return found
			}
		}
	}
	return nil
}

// seedConnection 用持久化形态（下划线）写入一条带完整敏感字段的连接，
// 等价于历史版本或 Teams 插件落下的数据。
func seedConnection(t *testing.T, savedMgr *connectionstore.Store) string {
	t.Helper()
	cfg := remote.ConnectConfig{
		Name: "db-1", Host: "10.0.0.9", Port: 22, User: "ops",
		Password: "login-pw", RootPassword: "root-secret",
		HostKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExample",
		Bastion: &remote.ConnectConfig{
			Name: "jump", Host: "10.0.0.254", Port: 2222, User: "jump", Password: "jump-pw",
		},
	}
	if _, err := savedMgr.CreateConnection(cfg, ""); err != nil {
		t.Fatalf("seed create: %v", err)
	}
	return savedMgr.Snapshot()[0].ID
}

// TestGetConnectionTree_WireRoundTrip 端到端覆盖"树读回 → 前端可读"：
// 出参线格式里 rootPassword / hostKey 必须以驼峰键出现且带出真实值。
func TestGetConnectionTree_WireRoundTrip(t *testing.T) {
	workDir := t.TempDir()
	app, savedMgr := newSavedSessionTestApp(t, workDir)
	id := seedConnection(t, savedMgr)

	tree, err := app.GetConnectionTree()
	if err != nil {
		t.Fatalf("GetConnectionTree: %v", err)
	}
	node := findWireSession(t, parseWireTree(t, marshalTree(t, tree)), id)
	if node == nil || node.Config == nil {
		t.Fatalf("连接 %s 未出现在出参树中", id)
	}
	cfg := node.Config
	if cfg.RootPassword != "root-secret" {
		t.Errorf("线格式 rootPassword 丢失: got %q", cfg.RootPassword)
	}
	if cfg.HostKey != "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExample" {
		t.Errorf("线格式 hostKey 丢失: got %q", cfg.HostKey)
	}
	if cfg.Password != "login-pw" {
		t.Errorf("登录密码丢失: got %q", cfg.Password)
	}
	if cfg.Bastion == nil || cfg.Bastion.Host != "10.0.0.254" || cfg.Bastion.Password != "jump-pw" {
		t.Errorf("跳板机配置在出参中丢失: %+v", cfg.Bastion)
	}
	// 下划线键不得再出现（出现即说明出参仍是持久化形态）。
	raw := string(marshalTree(t, tree))
	if strings.Contains(raw, "root_password") || strings.Contains(raw, "host_key") {
		t.Errorf("出参树仍含下划线键: %s", raw)
	}
}

// TestTreeConnect_RootPasswordReachesDial 覆盖"从树连接"路径：前端把树节点
// 的 config 原样回传给 Connect，入参侧驼峰 tag 必须能接住出参里的
// rootPassword / hostKey（出参与入参共用同一线格式）。
func TestTreeConnect_RootPasswordReachesDial(t *testing.T) {
	workDir := t.TempDir()
	app, savedMgr := newSavedSessionTestApp(t, workDir)
	id := seedConnection(t, savedMgr)

	// 前端拿到的正是 GetConnectionTree 的线格式；解析回入参类型即
	// window.go.main.App.Connect(config) 的实际效果。
	tree, err := app.GetConnectionTree()
	if err != nil {
		t.Fatalf("GetConnectionTree: %v", err)
	}
	var configs []ConnectConfig
	for _, n := range tree {
		if n.ID == id && n.Config != nil {
			configs = append(configs, *n.Config)
		}
	}
	if len(configs) != 1 {
		t.Fatalf("未在树中找到连接 %s", id)
	}
	got := configs[0]
	if got.RootPassword != "root-secret" {
		t.Errorf("树连接载荷丢 rootPassword: got %q", got.RootPassword)
	}
	if got.HostKey != "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExample" {
		t.Errorf("树连接载荷丢 hostKey: got %q", got.HostKey)
	}
}

// TestUpdateSavedConnection_TreeEditRoundTrip 覆盖完整用户流程：
// "打开编辑弹窗（初始值来自树）→ 不动 root 密码只改名 → 保存 → 重启后仍在"。
// 修复前：树读回 rootPassword 为空 → 表单提交不含该字段 → 整体替换清空磁盘值。
func TestUpdateSavedConnection_TreeEditRoundTrip(t *testing.T) {
	workDir := t.TempDir()
	app, savedMgr := newSavedSessionTestApp(t, workDir)
	id := seedConnection(t, savedMgr)

	// 编辑弹窗的 initialConfig 即树节点的 config。
	tree, err := app.GetConnectionTree()
	if err != nil {
		t.Fatalf("GetConnectionTree: %v", err)
	}
	var draft ConnectConfig
	for _, n := range tree {
		if n.ID == id && n.Config != nil {
			draft = *n.Config
		}
	}
	if draft.RootPassword != "root-secret" {
		t.Fatalf("编辑弹窗初始值就丢了 rootPassword: got %q", draft.RootPassword)
	}

	// 用户只改显示名，其余字段（含 root 密码）按表单值原样回传。
	draft.Name = "db-1-renamed"
	if err := app.UpdateSavedConnection(id, draft); err != nil {
		t.Fatalf("UpdateSavedConnection: %v", err)
	}

	// 从磁盘重载 = 应用重启后再次读树。
	reloaded := connectionstore.NewStoreWithPath(filepath.Join(workDir, "sessions.json"))
	if err := reloaded.Load(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	saved := findSavedNode(reloaded.Snapshot(), id)
	if saved == nil || saved.Config == nil {
		t.Fatalf("更新后的会话在重载后丢失")
	}
	if saved.Config.RootPassword != "root-secret" {
		t.Errorf("改名保存后 root 密码被清空: got %q", saved.Config.RootPassword)
	}
	if saved.Config.HostKey == "" {
		t.Errorf("改名保存后主机密钥被清空")
	}
	if saved.Config.Name != "db-1-renamed" {
		t.Errorf("改名未生效: got %q", saved.Config.Name)
	}
}

// TestCreateSavedConnection_TreeWireRoundTrip 覆盖新建入口的出参契约：
// 返回的节点同样必须是驼峰形态（前端类型要求）。
func TestCreateSavedConnection_TreeWireRoundTrip(t *testing.T) {
	workDir := t.TempDir()
	app, _ := newSavedSessionTestApp(t, workDir)

	node, err := app.CreateSavedConnection(ConnectConfig{
		Name: "new-1", Host: "10.9.0.1", Port: 22, User: "ops",
		Password: "p1", RootPassword: "r1", HostKey: "key-1",
	}, "")
	if err != nil {
		t.Fatalf("CreateSavedConnection: %v", err)
	}
	if node.Config == nil || node.Config.RootPassword != "r1" || node.Config.HostKey != "key-1" {
		t.Fatalf("新建返回的节点丢了敏感字段: %+v", node.Config)
	}
	// 线格式验证（Wails 序列化后的形态）。
	raw := string(marshalTree(t, []*SessionTreeNode{node}))
	if !strings.Contains(raw, `"rootPassword":"r1"`) || !strings.Contains(raw, `"hostKey":"key-1"`) {
		t.Errorf("新建返回的线格式不含驼峰键: %s", raw)
	}
}

// TestTreeBindingContract 防护出参方向的方法签名（#71 修复只堵了入参侧，
// 出参侧长期没人看）。所有把树/节点交给前端的边界方法都必须返回
// SessionTreeNode，不得直接漏出 connectionstore.Node。
func TestTreeBindingContract(t *testing.T) {
	forbidden := reflect.TypeOf(connectionstore.Node{})
	sliceType := reflect.TypeOf([]*SessionTreeNode{})
	ptrType := reflect.TypeOf(&SessionTreeNode{})
	for _, name := range []string{"GetConnectionTree", "CreateSavedFolder", "CreateSavedConnection", "DuplicateSavedConnection"} {
		method, ok := reflect.TypeOf(&App{}).MethodByName(name)
		if !ok {
			t.Fatalf("App.%s 方法不存在", name)
		}
		found := false
		for i := 0; i < method.Type.NumOut(); i++ {
			out := method.Type.Out(i)
			if out == forbidden {
				t.Errorf("App.%s 不得直接返回 connectionstore.Node（下划线 config tag）", name)
			}
			if out == sliceType || out == ptrType {
				found = true
			}
		}
		if !found {
			t.Errorf("App.%s 必须返回 SessionTreeNode（值或切片），实际签名: %s", name, method.Type)
		}
	}
}
