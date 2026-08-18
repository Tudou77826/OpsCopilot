package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"opscopilot/pkg/config"
	"opscopilot/pkg/sessionmanager"
)

// initBareRepoForShare 创建带 main 分支的裸仓库（共享仓库模拟）。
func initBareRepoForShare(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	tmpDir := t.TempDir()
	bare := filepath.Join(tmpDir, "shared-bare.git")

	runGitCmd(t, tmpDir, "init", "--bare", bare)
	clone := filepath.Join(tmpDir, "init-clone")
	runGitCmd(t, tmpDir, "clone", bare, clone)
	runGitCmd(t, clone, "config", "user.name", "Init")
	runGitCmd(t, clone, "config", "user.email", "init@test.com")
	os.WriteFile(filepath.Join(clone, ".gitkeep"), []byte(""), 0644)
	runGitCmd(t, clone, "add", ".gitkeep")
	runGitCmd(t, clone, "commit", "-m", "init")
	runGitCmd(t, clone, "branch", "-M", "main")
	runGitCmd(t, clone, "push", "-u", "origin", "main")
	os.RemoveAll(clone)
	return bare
}

func runGitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %s: %v", args, out, err)
	}
}

// newShareTestApp 构造仅填充会话共享所需字段的 App（不经过 NewApp 的重初始化），
// 覆盖编排层真实代码路径：init → 记录登录 → 列表 → 解密保存 → 删除。
func newShareTestApp(t *testing.T, bareRepo, workDir, secretKey string) *App {
	t.Helper()
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatalf("mkdir workdir: %v", err)
	}
	cfgMgr := config.NewManagerWithDir(workDir)
	if err := cfgMgr.Load(); err != nil {
		t.Fatalf("config load: %v", err)
	}
	cfgMgr.Config.Log.Dir = filepath.Join(workDir, "logs")
	cfgMgr.Config.SessionShare = config.SessionShareConfig{
		Enabled:   true,
		RemoteURL: bareRepo,
		Branch:    "main",
		SecretKey: secretKey,
	}

	savedMgr := sessionmanager.NewManagerWithPath(filepath.Join(workDir, "sessions.json"))
	if err := savedMgr.Load(); err != nil {
		t.Fatalf("saved sessions load: %v", err)
	}

	app := &App{
		configMgr:       cfgMgr,
		savedSessionMgr: savedMgr,
		sessionStates:   make(map[string]*SessionState),
		activeConfigs:   make(map[string]ConnectConfig),
	}
	return app
}

// waitUntil 轮询断言，超时失败。
func waitUntil(t *testing.T, timeout time.Duration, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for: %s", what)
}

// TestSessionShareEndToEnd 编排层端到端：初始化 → 连接成功记录 → 面板列表 →
// 错密钥拒绝 → 解密保存到本地 → 删除共享，并断言仓库/本地簿记不含明文密码。
func TestSessionShareEndToEnd(t *testing.T) {
	bareRepo := initBareRepoForShare(t)
	tmpRoot := filepath.Dir(bareRepo)
	const plainPassword = "super-secret-pw-123"

	app := newShareTestApp(t, bareRepo, filepath.Join(tmpRoot, "userA"), "team-key-2026")

	// 1. 初始化（后台 clone + 首次同步）
	app.initSessionShareStore()
	rt := app.getSessionShare()
	if rt == nil {
		t.Fatal("sessionShare runtime should be initialized")
	}
	waitUntil(t, 30*time.Second, func() bool {
		return !rt.statusSnapshot().Running
	}, "initial sync to finish")

	// 2. 模拟连接成功（recordSharedLogin 是 ConnectWithID 中的钩子）
	app.recordSharedLogin(ConnectConfig{
		Name:     "web-01",
		Host:     "10.0.0.1",
		Port:     22,
		User:     "root",
		Password: plainPassword,
	})

	// 3. 面板列表出现该条目（异步推送 + 就地视图更新）
	var entry SharedSessionView
	waitUntil(t, 30*time.Second, func() bool {
		var parsed struct {
			Enabled  bool                `json:"enabled"`
			Sessions []SharedSessionView `json:"sessions"`
		}
		if err := json.Unmarshal([]byte(app.GetSharedSessions()), &parsed); err != nil {
			return false
		}
		if len(parsed.Sessions) != 1 {
			return false
		}
		entry = parsed.Sessions[0]
		return true
	}, "shared session to appear in panel list")

	if entry.Name != "web-01" || entry.Host != "10.0.0.1" || entry.User != "root" {
		t.Errorf("entry fields mismatch: %+v", entry)
	}
	if !entry.Own {
		t.Errorf("entry should be own (owner=%s)", entry.Owner)
	}
	if !entry.HasSecrets || !entry.Decryptable {
		t.Errorf("entry should carry decryptable secrets: %+v", entry)
	}
	if entry.LastLoginAt == "" {
		t.Error("entry should carry lastLoginAt as endorsement")
	}

	// 4. 仓库文件与本地簿记均不含明文密码
	assertNoPlaintextSecret(t, filepath.Join(tmpRoot, "userA", "sessionstore"), plainPassword)
	assertNoPlaintextSecret(t, filepath.Join(tmpRoot, "userA", "sessionstore-local.json"), plainPassword)

	// 5. 错密钥 → 拒绝并给出明确提示
	app.configMgr.Config.SessionShare.SecretKey = "wrong-key"
	bad := app.ConnectSharedSession(entry.EntryKey)
	if bad.Success || bad.Config != nil {
		t.Fatal("connect with wrong key must fail without config")
	}
	if !strings.Contains(bad.Message, "共享密钥可能不正确") {
		t.Errorf("wrong-key message mismatch: %q", bad.Message)
	}

	// 6. 正确密钥 → 返回解密后的连接配置（由前端走统一连接流程）
	app.configMgr.Config.SessionShare.SecretKey = "team-key-2026"
	res := app.ConnectSharedSession(entry.EntryKey)
	if !res.Success || res.Config == nil {
		t.Fatalf("ConnectSharedSession should return decrypted config: %+v", res)
	}
	if res.Config.Host != "10.0.0.1" || res.Config.User != "root" || res.Config.Password != plainPassword {
		t.Errorf("decrypted config mismatch: host=%s user=%s password-set=%v",
			res.Config.Host, res.Config.User, res.Config.Password != "")
	}

	// 7. 保存到我的会话（解密后 Upsert sessions.json）
	if err := app.SaveSharedSessionToLocal(entry.EntryKey); err != "" {
		t.Fatalf("SaveSharedSessionToLocal: %s", err)
	}
	saved := app.savedSessionMgr.GetSessions()
	if len(saved) != 1 || saved[0].Config == nil {
		t.Fatalf("saved session tree should contain 1 node, got %+v", saved)
	}
	if saved[0].Config.Host != "10.0.0.1" || saved[0].Config.Password != plainPassword {
		t.Errorf("decrypted config mismatch: host=%s password-set=%v",
			saved[0].Config.Host, saved[0].Config.Password != "")
	}

	// 8. 删除共享 → 列表清空、仓库文件条目移除
	if err := app.RemoveSharedSession(entry.EntryKey); err != "" {
		t.Fatalf("RemoveSharedSession: %s", err)
	}
	var after struct {
		Sessions []SharedSessionView `json:"sessions"`
	}
	if err := json.Unmarshal([]byte(app.GetSharedSessions()), &after); err != nil {
		t.Fatal(err)
	}
	if len(after.Sessions) != 0 {
		t.Errorf("entry should disappear after removal, got %d", len(after.Sessions))
	}

	// 9. 状态接口可用
	status := app.GetSessionShareStatus()
	if !strings.Contains(status, `"enabled":true`) {
		t.Errorf("status should report enabled, got: %s", status)
	}
}

// TestSessionShareDisabled 覆盖 nil 守卫：未启用时钩子/接口全部安全短路。
func TestSessionShareDisabled(t *testing.T) {
	app := newShareTestApp(t, "unused-remote", t.TempDir(), "")
	app.configMgr.Config.SessionShare.Enabled = false
	app.initSessionShareStore()

	if app.getSessionShare() != nil {
		t.Fatal("runtime should be nil when disabled")
	}
	// 钩子不 panic、不记录
	app.recordSharedLogin(ConnectConfig{Host: "h", Port: 22, User: "u", Password: "p"})

	if got := app.GetSharedSessions(); got != `{"enabled":false,"sessions":[]}` {
		t.Errorf("GetSharedSessions disabled shape mismatch: %s", got)
	}
	if res := app.ConnectSharedSession("x|ssh|h|22|u"); res.Success {
		t.Error("connect should fail when disabled")
	}
}

// assertNoPlaintextSecret 断言 path（文件或目录树）中不含明文密码，但应含密文痕迹。
func assertNoPlaintextSecret(t *testing.T, path, plaintext string) {
	t.Helper()
	foundCipher := false
	var walk func(p string) error
	walk = func(p string) error {
		st, err := os.Stat(p)
		if err != nil {
			return err
		}
		if st.IsDir() {
			entries, err := os.ReadDir(p)
			if err != nil {
				return err
			}
			for _, e := range entries {
				if e.Name() == ".git" {
					continue
				}
				if err := walk(filepath.Join(p, e.Name())); err != nil {
					return err
				}
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(p), ".json") {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), plaintext) {
			t.Errorf("plaintext password leaked in %s", p)
		}
		if strings.Contains(string(data), "v1:") {
			foundCipher = true
		}
		return nil
	}
	if err := walk(path); err != nil {
		t.Fatalf("walk %s: %v", path, err)
	}
	if !foundCipher {
		t.Errorf("expected ciphertext marker in %s", path)
	}
}
