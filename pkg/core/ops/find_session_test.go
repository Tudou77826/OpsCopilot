package ops

import (
	"testing"

	"opscopilot/pkg/sessionmanager"
	"opscopilot/pkg/sshclient"
)

// makeSession 构造一个 type=session 的节点，Host=ip，Name=name。
// Name 故意与 IP 不同，用于验证匹配只看 Host 不看 Name。
func makeSession(name, ip string) *sessionmanager.Session {
	return &sessionmanager.Session{
		ID:   "id-" + ip,
		Name: name,
		Type: sessionmanager.TypeSession,
		Config: &sshclient.ConnectConfig{
			Name: name,
			Host: ip,
			Port: 22,
			User: "root",
		},
	}
}

// TestFindSessionConfig_MatchByIP 按 IP 匹配应命中
func TestFindSessionConfig_MatchByIP(t *testing.T) {
	nodes := []*sessionmanager.Session{
		makeSession("web-01", "10.1.1.1"),
		makeSession("db-01", "10.1.1.2"),
	}
	cfg := findSessionConfig(nodes, "10.1.1.1")
	if cfg == nil {
		t.Fatal("按 IP 10.1.1.1 应命中")
	}
	if cfg.Host != "10.1.1.1" {
		t.Errorf("命中 Host = %q, 期望 10.1.1.1", cfg.Host)
	}
}

// TestFindSessionConfig_NameNotMatched 按 Name 不应命中（验证废弃了 Name 匹配）。
// 回归保护：若有人把匹配逻辑改回 node.Name == name，此测试会失败。
func TestFindSessionConfig_NameNotMatched(t *testing.T) {
	nodes := []*sessionmanager.Session{
		makeSession("web-01", "10.1.1.1"),
	}
	// 传 Name（别名）应找不到，因为只按 Host(IP) 匹配
	cfg := findSessionConfig(nodes, "web-01")
	if cfg != nil {
		t.Errorf("按 Name 匹配不应命中，但找到了 Host=%q 的配置", cfg.Host)
	}
}

// TestFindSessionConfig_NestedFolder 嵌套在文件夹下的服务器也应能按 IP 找到
func TestFindSessionConfig_NestedFolder(t *testing.T) {
	nodes := []*sessionmanager.Session{
		{
			ID:   "folder-1",
			Name: "生产环境",
			Type: sessionmanager.TypeFolder,
			Children: []*sessionmanager.Session{
				makeSession("web-01", "10.1.1.1"),
				{
					ID:   "folder-2",
					Name: "数据库",
					Type: sessionmanager.TypeFolder,
					Children: []*sessionmanager.Session{
						makeSession("db-master", "10.2.2.2"),
					},
				},
			},
		},
	}
	cfg := findSessionConfig(nodes, "10.2.2.2")
	if cfg == nil {
		t.Fatal("嵌套文件夹下的 10.2.2.2 应命中")
	}
	if cfg.Host != "10.2.2.2" {
		t.Errorf("命中 Host = %q, 期望 10.2.2.2", cfg.Host)
	}
}

// TestFindSessionConfig_NotFound 未登记的 IP 应返回 nil
func TestFindSessionConfig_NotFound(t *testing.T) {
	nodes := []*sessionmanager.Session{
		makeSession("web-01", "10.1.1.1"),
	}
	cfg := findSessionConfig(nodes, "10.9.9.9")
	if cfg != nil {
		t.Errorf("未登记的 IP 不应命中, 但找到了 Host=%q", cfg.Host)
	}
}

// TestFindSessionConfig_EmptyNodes 空列表返回 nil
func TestFindSessionConfig_EmptyNodes(t *testing.T) {
	cfg := findSessionConfig([]*sessionmanager.Session{}, "10.1.1.1")
	if cfg != nil {
		t.Error("空列表应返回 nil")
	}
}
