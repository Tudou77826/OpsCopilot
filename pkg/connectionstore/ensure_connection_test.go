package connectionstore

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"opscopilot/pkg/remote"
)

// 本文件覆盖"连接时自动落库"的 ensure 契约（EnsureConnectionByEndpoint）。
// 回归背景：旧实现（UpsertByEndpoint）在连接重放时按端点整节点重写——
// 名字回退、按旧分组名重建文件夹、把连接拽离新分组，用户报告为
// "名字改完又被还原""改分组名后连接回到旧名字的分组"。

func newEnsureStore(t *testing.T) *Store {
	t.Helper()
	s := NewStoreWithPath(filepath.Join(t.TempDir(), "sessions.json"))
	if err := s.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	return s
}

// 用户报告的完整场景：分组改名 + 连接改名后，批量连接/重连重放旧配置。
// 全部连接发起后（其中一个用新配置、其余重放旧配置），树必须保持用户的整理结果。
func TestEnsureConnectionByEndpoint_FolderRenameSurvivesBatchReplay(t *testing.T) {
	s := newEnsureStore(t)

	// 五台机器首次连接，保存到分组"分组A"。
	hosts := []string{"10.0.0.1", "10.0.0.2", "10.0.0.3", "10.0.0.4", "10.0.0.5"}
	originals := make([]remote.ConnectConfig, 0, len(hosts))
	for _, h := range hosts {
		cfg := remote.ConnectConfig{Name: h, Host: h, Port: 22, User: "ops", Password: "pw", Group: "分组A"}
		originals = append(originals, cfg)
		if _, err := s.EnsureConnectionByEndpoint(cfg, cfg.Group); err != nil {
			t.Fatalf("seed %s: %v", h, err)
		}
	}

	// 用户把分组"分组A"改名"分组B"。
	var folderID string
	for _, n := range s.nodes {
		if n.Type == KindFolder && n.Name == "分组A" {
			folderID = n.ID
		}
	}
	if folderID == "" {
		t.Fatalf("种子分组不存在")
	}
	if err := s.RenameNode(folderID, "分组B"); err != nil {
		t.Fatalf("重命名分组: %v", err)
	}

	// 批量重连：全部用连接当时捕获的旧配置（分组还是"分组A"）。
	for _, cfg := range originals {
		if _, err := s.EnsureConnectionByEndpoint(cfg, cfg.Group); err != nil {
			t.Fatalf("replay %s: %v", cfg.Host, err)
		}
	}

	snap := s.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("根下应只有 1 个节点（分组B），实际 %d 个——旧分组被重放复活", len(snap))
	}
	if snap[0].Name != "分组B" || len(snap[0].Children) != len(hosts) {
		t.Fatalf("分组B 应保留全部 %d 个连接，实际 %s 下 %d 个", len(hosts), snap[0].Name, len(snap[0].Children))
	}
}

// 密码轮转：同账号连接携带新密码（拨号成功），树里存的是旧密码——必须学进新密码。
func TestEnsureConnectionByEndpoint_LearnsRotatedSecrets(t *testing.T) {
	s := newEnsureStore(t)

	seed := remote.ConnectConfig{Host: "10.0.0.1", Port: 22, User: "ops", Password: "old", RootPassword: "old-root"}
	if _, err := s.EnsureConnectionByEndpoint(seed, "生产"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	fresh := remote.ConnectConfig{Host: "10.0.0.1", Port: 22, User: "ops", Password: "new", RootPassword: "new-root", HostKey: "ssh-ed25519 AAA"}
	if _, err := s.EnsureConnectionByEndpoint(fresh, ""); err != nil {
		t.Fatalf("fresh: %v", err)
	}

	cfg := s.Snapshot()[0].Children[0].Config
	if cfg.Password != "new" || cfg.RootPassword != "new-root" || cfg.HostKey != "ssh-ed25519 AAA" {
		t.Errorf("新凭据未合入: password=%q root=%q hostKey=%q", cfg.Password, cfg.RootPassword, cfg.HostKey)
	}
	// 身份字段不受影响。
	if cfg.User != "ops" || s.Snapshot()[0].Children[0].Name != "10.0.0.1" {
		t.Errorf("身份字段被改动: %+v", cfg)
	}
}

// 空凭据不回写：重放配置可能不带密码（如插件界面），不得把已存密码清掉。
func TestEnsureConnectionByEndpoint_EmptySecretsKeepStored(t *testing.T) {
	s := newEnsureStore(t)

	seed := remote.ConnectConfig{Host: "10.0.0.1", Port: 22, User: "ops", Password: "stored-pw"}
	if _, err := s.EnsureConnectionByEndpoint(seed, ""); err != nil {
		t.Fatalf("seed: %v", err)
	}

	noSecrets := remote.ConnectConfig{Host: "10.0.0.1", Port: 22, User: "ops"}
	if _, err := s.EnsureConnectionByEndpoint(noSecrets, ""); err != nil {
		t.Fatalf("replay: %v", err)
	}
	if got := s.Snapshot()[0].Config.Password; got != "stored-pw" {
		t.Errorf("已存密码被清空: %q", got)
	}
}

// 同端点不同账号：密码归属无法判断，宁可不合入也不错写到别人账号上。
func TestEnsureConnectionByEndpoint_DifferentUserSkipsSecretMerge(t *testing.T) {
	s := newEnsureStore(t)

	seed := remote.ConnectConfig{Host: "10.0.0.1", Port: 22, User: "ops", Password: "ops-pw"}
	if _, err := s.EnsureConnectionByEndpoint(seed, ""); err != nil {
		t.Fatalf("seed: %v", err)
	}

	other := remote.ConnectConfig{Host: "10.0.0.1", Port: 22, User: "root", Password: "root-pw"}
	if _, err := s.EnsureConnectionByEndpoint(other, ""); err != nil {
		t.Fatalf("other: %v", err)
	}
	cfg := s.Snapshot()[0].Config
	if cfg.User != "ops" || cfg.Password != "ops-pw" {
		t.Errorf("不同账号的凭据被错写: user=%q password=%q", cfg.User, cfg.Password)
	}
}

// 跳板机：重放不带跳板机时保留已存；带新跳板机时整体采纳，空密码按账号回填。
func TestEnsureConnectionByEndpoint_BastionMergeRules(t *testing.T) {
	s := newEnsureStore(t)

	withBastion := remote.ConnectConfig{
		Host: "10.0.0.1", Port: 22, User: "ops", Password: "pw",
		Bastion: &remote.ConnectConfig{Host: "10.0.0.254", Port: 22, User: "jump", Password: "jp"},
	}
	if _, err := s.EnsureConnectionByEndpoint(withBastion, ""); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// 不带跳板机的重放：保留已存跳板机。
	noBastion := remote.ConnectConfig{Host: "10.0.0.1", Port: 22, User: "ops"}
	if _, err := s.EnsureConnectionByEndpoint(noBastion, ""); err != nil {
		t.Fatalf("replay: %v", err)
	}
	cfg := s.Snapshot()[0].Config
	if cfg.Bastion == nil || cfg.Bastion.Host != "10.0.0.254" || cfg.Bastion.Password != "jp" {
		t.Fatalf("已存跳板机丢失: %+v", cfg.Bastion)
	}

	// 带新跳板机：同端点同账号、密码为空（插件界面不回传密码的场景）——按已存回填。
	sameEndpointBastion := remote.ConnectConfig{
		Host: "10.0.0.1", Port: 22, User: "ops",
		Bastion: &remote.ConnectConfig{Host: "10.0.0.254", Port: 22, User: "jump", HostKey: "ssh-ed25519 BBB"},
	}
	if _, err := s.EnsureConnectionByEndpoint(sameEndpointBastion, ""); err != nil {
		t.Fatalf("bastion update: %v", err)
	}
	b := s.Snapshot()[0].Config.Bastion
	if b.HostKey != "ssh-ed25519 BBB" {
		t.Errorf("新跳板机密钥未采纳: %+v", b)
	}
	if b.Password != "jp" {
		t.Errorf("同端点跳板机的空密码未按已存回填: %q", b.Password)
	}

	// 换了端点的跳板机且不带密码：不得把旧端点的密码串到新端点上。
	otherEndpointBastion := remote.ConnectConfig{
		Host: "10.0.0.1", Port: 22, User: "ops",
		Bastion: &remote.ConnectConfig{Host: "10.0.0.253", Port: 2222, User: "jump"},
	}
	if _, err := s.EnsureConnectionByEndpoint(otherEndpointBastion, ""); err != nil {
		t.Fatalf("bastion replace: %v", err)
	}
	b = s.Snapshot()[0].Config.Bastion
	if b.Host != "10.0.0.253" || b.Port != 2222 {
		t.Errorf("新跳板机未采纳: %+v", b)
	}
	if b.Password != "" {
		t.Errorf("跨端点回填了旧跳板机密码（凭据串用）: %q", b.Password)
	}
}

// "全部连接"会并行发起多个 Connect，自动落库钩子随之并发执行：
// Store 内部以互斥锁串行化，并发下树必须保持一致（不丢连接、不重复建分组）。
func TestEnsureConnectionByEndpoint_ConcurrentConnectAll(t *testing.T) {
	s := newEnsureStore(t)

	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cfg := remote.ConnectConfig{
				Name: fmt.Sprintf("web-%d", i), Host: fmt.Sprintf("10.0.0.%d", i),
				Port: 22, User: "ops", Group: "分组A",
			}
			if _, err := s.EnsureConnectionByEndpoint(cfg, cfg.Group); err != nil {
				t.Errorf("concurrent ensure: %v", err)
			}
		}(i)
	}
	wg.Wait()

	snap := s.Snapshot()
	if len(snap) != 1 || snap[0].Name != "分组A" || len(snap[0].Children) != 12 {
		t.Fatalf("并发后树不一致: 根=%d 分组=%+v", len(snap), snap)
	}
}
