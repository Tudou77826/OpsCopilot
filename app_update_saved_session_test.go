package main

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"opscopilot/pkg/remote"
	"opscopilot/pkg/sessionmanager"
	"opscopilot/pkg/sshclient"
)

// newSavedSessionTestApp 构造仅填充已保存会话所需字段的 App（不经过 NewApp 重初始化），
// 用于覆盖"前端编辑连接 → Wails → Go 持久化"这条用户流程。
func newSavedSessionTestApp(t *testing.T, workDir string) (*App, *sessionmanager.Manager) {
	t.Helper()
	savedMgr := sessionmanager.NewManagerWithPath(filepath.Join(workDir, "sessions.json"))
	if err := savedMgr.Load(); err != nil {
		t.Fatalf("saved sessions load: %v", err)
	}
	app := &App{savedSessionMgr: savedMgr}
	return app, savedMgr
}

// findSavedNode 在会话树中按 ID 递归查找节点。
func findSavedNode(nodes []*sessionmanager.Session, id string) *sessionmanager.Session {
	for _, node := range nodes {
		if node.ID == id {
			return node
		}
		if node.Type == sessionmanager.TypeFolder {
			if found := findSavedNode(node.Children, id); found != nil {
				return found
			}
		}
	}
	return nil
}

// TestUpdateSavedSession_PreservesRootPassword 覆盖用户业务流程：
// "编辑一个已有连接的信息（不改 root 密码以外的字段）→ 保存 → 配置完整落盘"。
//
// 回归背景（Issue #71）：UpdateSavedSession 曾直接用 remote.ConnectConfig
// （root_password 下划线 tag）反序列化前端驼峰 payload，rootPassword 被静默
// 丢弃，整体替换保存时每次都清空已存 root 密码。
func TestUpdateSavedSession_PreservesRootPassword(t *testing.T) {
	workDir := t.TempDir()
	app, savedMgr := newSavedSessionTestApp(t, workDir)

	// 初始状态：用户之前连接时保存了 root 密码（Upsert 模拟首次保存）。
	if err := savedMgr.Upsert(sshclient.ConnectConfig{
		Name:         "db-1",
		Host:         "10.0.0.1",
		Port:         22,
		User:         "ops",
		Password:     "login-pw",
		RootPassword: "root-secret",
	}, ""); err != nil {
		t.Fatalf("seed upsert: %v", err)
	}
	// Upsert 后树里只有这一个会话节点。
	id := savedMgr.GetSessions()[0].ID

	// 用户在编辑弹窗里改了名称、端口、用户名；root 密码原样随表单回传
	// （前端表单预填已存值，提交时以驼峰 rootPassword 字段回传）。
	if errStr := app.UpdateSavedSession(id, ConnectConfig{
		Name:         "db-1-renamed",
		Protocol:     "ssh",
		Host:         "10.0.0.1",
		Port:         2222,
		User:         "ops2",
		Password:     "login-pw",
		RootPassword: "root-secret",
		Group:        "",
	}); errStr != "" {
		t.Fatalf("UpdateSavedSession returned error: %s", errStr)
	}

	// 从磁盘重新加载验证（等价于应用重启后用户再次打开编辑弹窗看到的值）。
	reloaded := sessionmanager.NewManagerWithPath(filepath.Join(workDir, "sessions.json"))
	if err := reloaded.Load(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	saved := findSavedNode(reloaded.GetSessions(), id)
	if saved == nil {
		t.Fatalf("updated session %s not found after reload", id)
	}
	cfg := saved.Config
	if cfg == nil {
		t.Fatalf("session %s has no config", id)
	}
	if cfg.RootPassword != "root-secret" {
		t.Errorf("root password lost on update: got %q, want %q", cfg.RootPassword, "root-secret")
	}
	if cfg.Port != 2222 || cfg.User != "ops2" || cfg.Name != "db-1-renamed" {
		t.Errorf("edited fields not persisted: port=%d user=%q name=%q", cfg.Port, cfg.User, cfg.Name)
	}
	if cfg.Password != "login-pw" {
		t.Errorf("login password lost on update: got %q", cfg.Password)
	}
}

// TestUpdateSavedSession_BastionRoundTrip 覆盖用户业务流程：
// "编辑带跳板机的连接 → 保存 → 跳板机配置完整落盘"。
func TestUpdateSavedSession_BastionRoundTrip(t *testing.T) {
	workDir := t.TempDir()
	app, savedMgr := newSavedSessionTestApp(t, workDir)

	if err := savedMgr.Upsert(sshclient.ConnectConfig{
		Name: "core-1", Host: "10.1.0.1", Port: 22, User: "ops", Password: "pw",
	}, ""); err != nil {
		t.Fatalf("seed upsert: %v", err)
	}
	id := savedMgr.GetSessions()[0].ID

	if errStr := app.UpdateSavedSession(id, ConnectConfig{
		Name: "core-1", Host: "10.1.0.1", Port: 22, User: "ops", Password: "pw",
		Bastion: &ConnectConfig{
			Name: "jump-1", Host: "10.1.0.254", Port: 2222, User: "jump", Password: "jump-pw",
		},
	}); errStr != "" {
		t.Fatalf("UpdateSavedSession returned error: %s", errStr)
	}

	reloaded := sessionmanager.NewManagerWithPath(filepath.Join(workDir, "sessions.json"))
	if err := reloaded.Load(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	saved := findSavedNode(reloaded.GetSessions(), id)
	if saved == nil || saved.Config == nil || saved.Config.Bastion == nil {
		t.Fatalf("bastion config lost on update")
	}
	b := saved.Config.Bastion
	if b.Host != "10.1.0.254" || b.Port != 2222 || b.User != "jump" || b.Password != "jump-pw" {
		t.Errorf("bastion fields not persisted: %+v", b)
	}
}

// TestUpdateSavedSession_BindingContract 防护 Wails 边界契约（#71 根因的回归闸门）：
// UpdateSavedSession 的入参结构体必须是 app 侧驼峰 ConnectConfig。
// 若有人把它改回 sshclient.ConnectConfig（= remote.ConnectConfig 别名，root_password
// 下划线 tag），前端 rootPassword 会被 JSON 反序列化静默丢弃，且下方的纯 Go 测试
// 察觉不到——所以这里直接断言方法签名与 JSON tag。
func TestUpdateSavedSession_BindingContract(t *testing.T) {
	method, ok := reflect.TypeOf(&App{}).MethodByName("UpdateSavedSession")
	if !ok {
		t.Fatalf("App.UpdateSavedSession method not found")
	}
	// 方法类型 In(0) 为接收者 *App，其后依次是 id、config 入参；扫描全部入参，
	// 要求其中出现 app 侧驼峰 ConnectConfig，绝不允许 remote/sshclient 别名混入。
	appCfgType := reflect.TypeOf(ConnectConfig{})
	found := false
	for i := 1; i < method.Type.NumIn(); i++ {
		switch method.Type.In(i) {
		case appCfgType:
			found = true
		case reflect.TypeOf(remote.ConnectConfig{}), reflect.TypeOf(sshclient.ConnectConfig{}):
			t.Errorf("UpdateSavedSession must not take remote/sshclient ConnectConfig (snake_case json tags) directly: %s", method.Type.In(i))
		}
	}
	if !found {
		t.Errorf("UpdateSavedSession config param must be app-side camelCase ConnectConfig, signature: %s", method.Type)
	}

	// 驼峰结构体必须向前端暴露 rootPassword 字段名。
	tag, ok := reflect.TypeOf(ConnectConfig{}).FieldByName("RootPassword")
	if !ok || !strings.Contains(tag.Tag.Get("json"), "rootPassword") {
		t.Errorf("app ConnectConfig.RootPassword json tag must be camelCase rootPassword, got %q", tag.Tag.Get("json"))
	}
	// 对照：持久化结构体保持历史下划线格式（sessions.json 向后兼容）。
	persistTag, ok := reflect.TypeOf(remote.ConnectConfig{}).FieldByName("RootPassword")
	if !ok || !strings.Contains(persistTag.Tag.Get("json"), "root_password") {
		t.Errorf("remote ConnectConfig.RootPassword json tag must stay snake_case root_password, got %q", persistTag.Tag.Get("json"))
	}
}
