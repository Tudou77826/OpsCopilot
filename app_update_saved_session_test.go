package main

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"opscopilot/pkg/connectionstore"
	"opscopilot/pkg/remote"
	"opscopilot/pkg/sshclient"
)

// newSavedSessionTestApp 构造仅填充已保存会话所需字段的 App（不经过 NewApp 重初始化），
// 用于覆盖"前端编辑连接 → Wails → Go 持久化"这条用户流程。
func newSavedSessionTestApp(t *testing.T, workDir string) (*App, *connectionstore.Store) {
	t.Helper()
	savedMgr := connectionstore.NewStoreWithPath(filepath.Join(workDir, "sessions.json"))
	if err := savedMgr.Load(); err != nil {
		t.Fatalf("saved sessions load: %v", err)
	}
	app := &App{savedSessionMgr: savedMgr}
	return app, savedMgr
}

// findSavedNode 在会话树中按 ID 递归查找节点。
func findSavedNode(nodes []*connectionstore.Node, id string) *connectionstore.Node {
	for _, node := range nodes {
		if node.ID == id {
			return node
		}
		if node.Type == connectionstore.KindFolder {
			if found := findSavedNode(node.Children, id); found != nil {
				return found
			}
		}
	}
	return nil
}

// TestUpdateSavedConnection_PreservesRootPassword 覆盖用户业务流程：
// "编辑一个已有连接的信息（不改 root 密码以外的字段）→ 保存 → 配置完整落盘"。
//
// 回归背景（Issue #71）：边界层曾直接用 remote.ConnectConfig
// （root_password 下划线 tag）反序列化前端驼峰 payload，rootPassword 被静默
// 丢弃，整体替换保存时每次都清空已存 root 密码。
func TestUpdateSavedConnection_PreservesRootPassword(t *testing.T) {
	workDir := t.TempDir()
	app, savedMgr := newSavedSessionTestApp(t, workDir)

	// 初始状态：用户之前连接时保存了 root 密码。
	if _, err := savedMgr.UpsertByEndpoint(sshclient.ConnectConfig{
		Name:         "db-1",
		Host:         "10.0.0.1",
		Port:         22,
		User:         "ops",
		Password:     "login-pw",
		RootPassword: "root-secret",
	}, ""); err != nil {
		t.Fatalf("seed upsert: %v", err)
	}
	id := savedMgr.Snapshot()[0].ID

	// 用户在编辑弹窗里改了名称、端口、用户名；root 密码原样随表单回传
	// （前端表单预填已存值，提交时以驼峰 rootPassword 字段回传）。
	if err := app.UpdateSavedConnection(id, ConnectConfig{
		Name:         "db-1-renamed",
		Protocol:     "ssh",
		Host:         "10.0.0.1",
		Port:         2222,
		User:         "ops2",
		Password:     "login-pw",
		RootPassword: "root-secret",
	}); err != nil {
		t.Fatalf("UpdateSavedConnection 返回错误: %v", err)
	}

	// 从磁盘重新加载验证（等价于应用重启后用户再次打开编辑弹窗看到的值）。
	reloaded := connectionstore.NewStoreWithPath(filepath.Join(workDir, "sessions.json"))
	if err := reloaded.Load(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	saved := findSavedNode(reloaded.Snapshot(), id)
	if saved == nil {
		t.Fatalf("更新后的会话 %s 在重载后不存在", id)
	}
	cfg := saved.Config
	if cfg == nil {
		t.Fatalf("会话 %s 没有配置", id)
	}
	if cfg.RootPassword != "root-secret" {
		t.Errorf("更新后 root 密码丢失: got %q, want %q", cfg.RootPassword, "root-secret")
	}
	if cfg.Port != 2222 || cfg.User != "ops2" || cfg.Name != "db-1-renamed" {
		t.Errorf("编辑的字段未持久化: port=%d user=%q name=%q", cfg.Port, cfg.User, cfg.Name)
	}
	if cfg.Password != "login-pw" {
		t.Errorf("登录密码丢失: got %q", cfg.Password)
	}
}

// TestUpdateSavedConnection_BastionRoundTrip 覆盖用户业务流程：
// "编辑带跳板机的连接 → 保存 → 跳板机配置完整落盘"。
func TestUpdateSavedConnection_BastionRoundTrip(t *testing.T) {
	workDir := t.TempDir()
	app, savedMgr := newSavedSessionTestApp(t, workDir)

	if _, err := savedMgr.UpsertByEndpoint(sshclient.ConnectConfig{
		Name: "core-1", Host: "10.1.0.1", Port: 22, User: "ops", Password: "pw",
	}, ""); err != nil {
		t.Fatalf("seed upsert: %v", err)
	}
	id := savedMgr.Snapshot()[0].ID

	if err := app.UpdateSavedConnection(id, ConnectConfig{
		Name: "core-1", Host: "10.1.0.1", Port: 22, User: "ops", Password: "pw",
		Bastion: &ConnectConfig{
			Name: "jump-1", Host: "10.1.0.254", Port: 2222, User: "jump", Password: "jump-pw",
		},
	}); err != nil {
		t.Fatalf("UpdateSavedConnection 返回错误: %v", err)
	}

	reloaded := connectionstore.NewStoreWithPath(filepath.Join(workDir, "sessions.json"))
	if err := reloaded.Load(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	saved := findSavedNode(reloaded.Snapshot(), id)
	if saved == nil || saved.Config == nil || saved.Config.Bastion == nil {
		t.Fatalf("更新后跳板机配置丢失")
	}
	b := saved.Config.Bastion
	if b.Host != "10.1.0.254" || b.Port != 2222 || b.User != "jump" || b.Password != "jump-pw" {
		t.Errorf("跳板机字段未持久化: %+v", b)
	}
}

// TestUpdateSavedConnection_BindingContract 防护 Wails 边界契约（#71 根因的回归闸门）：
// 边界方法的 config 入参必须是 app 侧驼峰 ConnectConfig。
// 若有人把它改回 sshclient.ConnectConfig（= remote.ConnectConfig 别名，root_password
// 下划线 tag），前端 rootPassword 会被 JSON 反序列化静默丢弃，且纯 Go 测试察觉不到
// ——所以这里直接断言方法签名与 JSON tag。
func TestUpdateSavedConnection_BindingContract(t *testing.T) {
	appCfgType := reflect.TypeOf(ConnectConfig{})
	forbidden := []reflect.Type{
		reflect.TypeOf(remote.ConnectConfig{}),
		reflect.TypeOf(sshclient.ConnectConfig{}),
	}

	// 所有会接收前端连接配置的边界方法都要走驼峰 DTO。
	for _, name := range []string{"UpdateSavedConnection", "CreateSavedConnection", "Connect", "ConnectWithID"} {
		method, ok := reflect.TypeOf(&App{}).MethodByName(name)
		if !ok {
			t.Fatalf("App.%s 方法不存在", name)
		}
		found := false
		for i := 1; i < method.Type.NumIn(); i++ {
			in := method.Type.In(i)
			for _, bad := range forbidden {
				if in == bad {
					t.Errorf("App.%s 不得直接接收 remote/sshclient ConnectConfig（下划线 tag）: %s", name, in)
				}
			}
			if in == appCfgType {
				found = true
			}
		}
		if !found {
			t.Errorf("App.%s 的 config 入参必须是 app 侧驼峰 ConnectConfig，实际签名: %s", name, method.Type)
		}
	}

	// 驼峰结构体必须向前端暴露 rootPassword 字段名。
	tag, ok := reflect.TypeOf(ConnectConfig{}).FieldByName("RootPassword")
	if !ok || !strings.Contains(tag.Tag.Get("json"), "rootPassword") {
		t.Errorf("app ConnectConfig.RootPassword 的 json tag 必须是驼峰 rootPassword，实际 %q", tag.Tag.Get("json"))
	}
	// 对照：持久化结构体保持历史下划线格式（sessions.json 向后兼容）。
	persistTag, ok := reflect.TypeOf(remote.ConnectConfig{}).FieldByName("RootPassword")
	if !ok || !strings.Contains(persistTag.Tag.Get("json"), "root_password") {
		t.Errorf("remote ConnectConfig.RootPassword 的 json tag 必须保持下划线 root_password，实际 %q", persistTag.Tag.Get("json"))
	}
}
