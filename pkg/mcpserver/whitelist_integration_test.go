package mcpserver

import (
	"encoding/json"
	"os"
	"testing"
)

// TestIntegration_CommandWhitelistFlow 测试完整的命令白名单流程
func TestIntegration_CommandWhitelistFlow(t *testing.T) {
	// 1. 创建临时配置文件
	tmpFile := "test_integration_whitelist.json"
	defer os.Remove(tmpFile)

	// 2. 创建管理器
	mgr, err := NewWhitelistManager(tmpFile)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// 3. 验证默认配置
	config := mgr.GetConfig()
	if config.Version != "1.0" {
		t.Errorf("Expected version 1.0, got %s", config.Version)
	}

	// 4. 测试命令检查
	testCases := []struct {
		name     string
		command  string
		ip       string
		expected bool
	}{
		// 只读命令应该允许
		{"ls command", "ls -la", "192.168.1.100", true},
		{"cat command", "cat /var/log/syslog", "10.0.0.1", true},
		{"ps command", "ps aux", "172.16.0.1", true},
		{"grep command", "grep error /var/log/app.log", "192.168.1.50", true},
		{"docker ps", "docker ps", "10.0.1.100", true},
		{"kubectl get", "kubectl get pods", "10.0.1.100", true},

		// 危险命令应该拒绝
		{"rm command", "rm -rf /data", "192.168.1.100", false},
		{"shutdown", "shutdown -h now", "10.0.0.1", false},
		{"iptables", "iptables -F", "172.16.0.1", false},

		// 边界情况
		{"empty command", "", "192.168.1.100", false},
		{"whitespace only", "   ", "192.168.1.100", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := mgr.Check(tc.command, tc.ip)
			if result.Allowed != tc.expected {
				t.Errorf("Check(%q, %q) = %v, expected %v, reason: %s",
					tc.command, tc.ip, result.Allowed, tc.expected, result.Reason)
			}
		})
	}

	// 5. 测试 IP 段匹配
	ipRangeTests := []struct {
		ip       string
		ranges   []string
		expected bool
	}{
		{"192.168.1.100", []string{"*"}, true},
		{"192.168.1.100", []string{"192.168.1.0/24"}, true},
		{"192.168.2.100", []string{"192.168.1.0/24"}, false},
		{"10.0.0.50", []string{"10.0.0.0/8"}, true},
		{"172.16.5.10", []string{"172.16.0.0/12"}, true},
		{"8.8.8.8", []string{"192.168.0.0/16", "10.0.0.0/8"}, false},
	}

	for _, tc := range ipRangeTests {
		result := matchesIPRange(tc.ip, tc.ranges)
		if result != tc.expected {
			t.Errorf("matchesIPRange(%q, %v) = %v, expected %v",
				tc.ip, tc.ranges, result, tc.expected)
		}
	}

	// 6. 测试配置持久化
	newConfig := &WhitelistConfig{
		Version:         "1.0",
		LLMCheckEnabled: true,
		Policies: []Policy{
			{
				ID:          "test-policy",
				Name:        "测试策略",
				Description: "用于测试的策略",
				IPRanges:    []string{"10.0.0.0/8"},
				Commands: []Command{
					{
						Pattern:     "^test-cmd",
						Category:    CategoryReadOnly,
						Description: "测试命令",
						Enabled:     true,
					},
				},
			},
		},
	}

	if err := mgr.UpdateConfig(newConfig); err != nil {
		t.Fatalf("Failed to update config: %v", err)
	}

	// 7. 验证持久化 - 创建新管理器读取配置
	mgr2, err := NewWhitelistManager(tmpFile)
	if err != nil {
		t.Fatalf("Failed to create second manager: %v", err)
	}

	loadedConfig := mgr2.GetConfig()
	if loadedConfig.LLMCheckEnabled != true {
		t.Error("LLMCheckEnabled not persisted")
	}

	if len(loadedConfig.Policies) != 1 || loadedConfig.Policies[0].ID != "test-policy" {
		t.Error("Policy not persisted correctly")
	}

	// 8. 打印最终配置
	configJSON, _ := json.MarshalIndent(loadedConfig, "", "  ")
	t.Logf("Final config:\n%s", string(configJSON))
}
