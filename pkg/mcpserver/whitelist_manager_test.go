package mcpserver

import (
	"os"
	"testing"
)

func TestWhitelistManager_DefaultConfig(t *testing.T) {
	// 测试默认配置生成
	config := DefaultWhitelistConfig()

	if config.Version != "1.0" {
		t.Errorf("Expected version 1.0, got %s", config.Version)
	}

	if len(config.Policies) == 0 {
		t.Error("Expected at least one policy")
	}

	// 检查默认策略
	defaultPolicy := config.Policies[0]
	if defaultPolicy.ID != "default" {
		t.Errorf("Expected default policy ID 'default', got %s", defaultPolicy.ID)
	}

	if len(defaultPolicy.IPRanges) == 0 || defaultPolicy.IPRanges[0] != "*" {
		t.Error("Expected default policy to have '*' IP range")
	}

	if len(defaultPolicy.Commands) == 0 {
		t.Error("Expected default policy to have commands")
	}
}

func TestWhitelistManager_CheckCommand(t *testing.T) {
	// 创建临时配置文件
	tmpFile := "test_whitelist.json"
	defer os.Remove(tmpFile)

	mgr, err := NewWhitelistManager(tmpFile)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// 测试允许的命令
	tests := []struct {
		command  string
		ip       string
		expected bool
	}{
		{"ls -la", "192.168.1.100", true},
		{"cat /etc/passwd", "10.0.0.1", true},
		{"ps aux", "172.16.0.1", true},
		{"rm -rf /", "192.168.1.100", false}, // 危险命令不允许
		{"systemctl restart nginx", "10.0.0.1", false}, // 写入命令默认不允许
		{"", "192.168.1.100", false}, // 空命令
	}

	for _, tt := range tests {
		result := mgr.Check(tt.command, tt.ip)
		if result.Allowed != tt.expected {
			t.Errorf("Check(%q, %q) = %v, expected %v, reason: %s",
				tt.command, tt.ip, result.Allowed, tt.expected, result.Reason)
		}
	}
}

func TestWhitelistManager_MatchesIPRange(t *testing.T) {
	tests := []struct {
		ip       string
		ranges   []string
		expected bool
	}{
		{"192.168.1.100", []string{"*"}, true},
		{"192.168.1.100", []string{"192.168.1.0/24"}, true},
		{"192.168.2.100", []string{"192.168.1.0/24"}, false},
		{"10.0.0.50", []string{"10.0.0.0/8"}, true},
		{"172.16.5.10", []string{"192.168.0.0/16", "172.16.0.0/12"}, true},
		{"192.168.1.100", []string{"192.168.1.100"}, true}, // 单 IP
		{"192.168.1.101", []string{"192.168.1.100"}, false},
	}

	for _, tt := range tests {
		result := matchesIPRange(tt.ip, tt.ranges)
		if result != tt.expected {
			t.Errorf("matchesIPRange(%q, %v) = %v, expected %v",
				tt.ip, tt.ranges, result, tt.expected)
		}
	}
}

func TestWhitelistManager_AddUpdateDeletePolicy(t *testing.T) {
	tmpFile := "test_whitelist_ops.json"
	defer os.Remove(tmpFile)

	mgr, err := NewWhitelistManager(tmpFile)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// 添加新策略
	newPolicy := Policy{
		ID:          "test-policy",
		Name:        "测试策略",
		Description: "用于测试",
		IPRanges:    []string{"10.0.0.0/8"},
		Commands: []Command{
			{Pattern: "^test-cmd", Category: CategoryReadOnly, Description: "测试命令", Enabled: true},
		},
	}

	if err := mgr.AddPolicy(newPolicy); err != nil {
		t.Fatalf("Failed to add policy: %v", err)
	}

	// 验证添加成功
	config := mgr.GetConfig()
	found := false
	for _, p := range config.Policies {
		if p.ID == "test-policy" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Policy not found after adding")
	}

	// 更新策略
	newPolicy.Name = "更新后的策略"
	if err := mgr.UpdatePolicy(newPolicy); err != nil {
		t.Fatalf("Failed to update policy: %v", err)
	}

	// 验证更新成功
	config = mgr.GetConfig()
	for _, p := range config.Policies {
		if p.ID == "test-policy" && p.Name != "更新后的策略" {
			t.Error("Policy name not updated")
		}
	}

	// 删除策略
	if err := mgr.DeletePolicy("test-policy"); err != nil {
		t.Fatalf("Failed to delete policy: %v", err)
	}

	// 验证删除成功
	config = mgr.GetConfig()
	for _, p := range config.Policies {
		if p.ID == "test-policy" {
			t.Error("Policy still exists after deletion")
		}
	}
}

func TestWhitelistManager_SaveAndLoad(t *testing.T) {
	tmpFile := "test_whitelist_persist.json"
	defer os.Remove(tmpFile)

	// 创建管理器并修改配置
	mgr1, err := NewWhitelistManager(tmpFile)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// 修改配置
	config := mgr1.GetConfig()
	newPolicy := Policy{
		ID:          "persist-test",
		Name:        "持久化测试",
		Description: "测试配置持久化",
		IPRanges:    []string{"192.168.100.0/24"},
		Commands: []Command{
			{Pattern: "^persist", Category: CategoryReadOnly, Description: "持久化命令", Enabled: true},
		},
	}
	config.Policies = append(config.Policies, newPolicy)

	if err := mgr1.UpdateConfig(config); err != nil {
		t.Fatalf("Failed to update config: %v", err)
	}

	// 创建新的管理器来验证持久化
	mgr2, err := NewWhitelistManager(tmpFile)
	if err != nil {
		t.Fatalf("Failed to create second manager: %v", err)
	}

	loadedConfig := mgr2.GetConfig()

	// 验证策略
	found := false
	for _, p := range loadedConfig.Policies {
		if p.ID == "persist-test" {
			found = true
			if p.Name != "持久化测试" {
				t.Error("Policy name not persisted correctly")
			}
			break
		}
	}
	if !found {
		t.Error("New policy not persisted")
	}
}

func TestWhitelistManager_Reload(t *testing.T) {
	tmpFile := "test_whitelist_reload.json"
	defer os.Remove(tmpFile)

	// 创建第一个管理器
	mgr1, err := NewWhitelistManager(tmpFile)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// 初始检查：reload-cmd 不应被允许
	result := mgr1.Check("reload-cmd test", "192.168.1.100")
	if result.Allowed {
		t.Error("reload-cmd should not be allowed initially")
	}

	// 创建第二个管理器来修改配置（模拟 UI 或其他进程修改）
	mgr2, err := NewWhitelistManager(tmpFile)
	if err != nil {
		t.Fatalf("Failed to create second manager: %v", err)
	}

	// 添加新命令到配置
	config := mgr2.GetConfig()
	for i := range config.Policies {
		if config.Policies[i].ID == "default" {
			config.Policies[i].Commands = append(config.Policies[i].Commands, Command{
				Pattern:     "^reload-cmd",
				Category:    CategoryReadOnly,
				Description: "Reload test command",
				Enabled:     true,
			})
		}
	}
	if err := mgr2.UpdateConfig(config); err != nil {
		t.Fatalf("Failed to update config: %v", err)
	}

	// mgr1 仍然不知道新命令（因为还在使用旧的内存配置）
	result = mgr1.Check("reload-cmd test", "192.168.1.100")
	if result.Allowed {
		t.Error("mgr1 should still not allow reload-cmd before reload")
	}

	// 调用 Reload 方法
	if err := mgr1.Reload(); err != nil {
		t.Fatalf("Failed to reload: %v", err)
	}

	// 现在 mgr1 应该能看到新命令
	result = mgr1.Check("reload-cmd test", "192.168.1.100")
	if !result.Allowed {
		t.Error("mgr1 should allow reload-cmd after reload")
	}
}
