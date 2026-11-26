package ai

import (
	"context"
	"opscopilot/pkg/config"
	"opscopilot/pkg/llm"
	"testing"
)

func TestParseConnectIntent(t *testing.T) {
	// Mock response
	expectedJSON := `[
		{
			"host": "192.168.1.10",
			"port": 22,
			"user": "root",
			"password": "123",
            "name": "Database Server",
			"bastion": {
				"host": "10.0.0.1",
				"port": 22,
				"user": "admin",
				"password": "abc",
                "name": "Bastion Host"
			}
		},
		{
			"host": "192.168.1.11",
			"port": 22,
			"user": "root",
			"password": "123",
			"bastion": {
				"host": "10.0.0.1",
				"port": 22,
				"user": "admin",
				"password": "abc"
			}
		}
	]`

	mockProvider := &llm.MockProvider{
		Response: expectedJSON,
	}

	cfgMgr := config.NewManager()
	service := NewAIService(mockProvider, mockProvider, cfgMgr)

	input := "通过跳板机 10.0.0.1 (admin/abc) 连接 192.168.1.10 和 1.11，账号 root 密码 123"
	configs, err := service.ParseConnectIntent(input)
	if err != nil {
		t.Fatalf("ParseConnectIntent failed: %v", err)
	}

	if len(configs) != 2 {
		t.Errorf("Expected 2 configs, got %d", len(configs))
	}

	// Verify details of first config
	c1 := configs[0]
	if c1.Host != "192.168.1.10" {
		t.Errorf("Expected host 192.168.1.10, got %s", c1.Host)
	}
	if c1.Name != "Database Server" {
		t.Errorf("Expected name 'Database Server', got %s", c1.Name)
	}
	if c1.Bastion == nil {
		t.Fatal("Expected bastion config, got nil")
	}
	if c1.Bastion.Host != "10.0.0.1" {
		t.Errorf("Expected bastion host 10.0.0.1, got %s", c1.Bastion.Host)
	}
	if c1.Bastion.Name != "Bastion Host" {
		t.Errorf("Expected bastion name 'Bastion Host', got %s", c1.Bastion.Name)
	}

	// Verify second config (no explicit name in JSON, so Name should be empty string or default depending on parsing)
	// The struct parsing will leave it empty if not in JSON.
	c2 := configs[1]
	if c2.Name != "" {
		t.Errorf("Expected empty name for second config, got %s", c2.Name)
	}
}

func TestParseConnectIntent_ProtocolPassthrough(t *testing.T) {
	// AI 返回的协议可能是大写、带空格，也可能输出未知协议或缺失。
	// 期望：合法值归一化透传（telnet 才能真正落库），其余回退空值（按 SSH 处理）。
	expectedJSON := `[
		{"host": "60.60.33.44", "port": 23, "user": "admin", "protocol": "telnet", "password": "x"},
		{"host": "10.0.0.5", "port": 22, "user": "root", "protocol": "SSH", "password": "y"},
		{"host": "10.0.0.6", "port": 22, "user": "root", "protocol": "sftp", "password": "z"},
		{"host": "10.0.0.7", "port": 22, "user": "root"}
	]`

	mockProvider := &llm.MockProvider{Response: expectedJSON}
	service := NewAIService(mockProvider, mockProvider, config.NewManager())

	configs, err := service.ParseConnectIntent("任意输入，桩模型不看内容")
	if err != nil {
		t.Fatalf("ParseConnectIntent failed: %v", err)
	}
	if len(configs) != 4 {
		t.Fatalf("Expected 4 configs, got %d", len(configs))
	}

	want := []string{"telnet", "ssh", "", ""}
	for i, w := range want {
		if configs[i].Protocol != w {
			t.Errorf("config #%d protocol = %q, want %q", i+1, configs[i].Protocol, w)
		}
	}
}

func TestParseConnectIntent_SanitizesInvisibleCharsAndDefaults(t *testing.T) {
	expectedJSON := `[
		{
			"name": " \u200b订单节点\u00a0",
			"host": "\ufeff 10.0.1.8 \r\n",
			"user": " root\u200b ",
			"password": " \u200bp@ss word#1\u00a0\t",
			"root_password": "\u200broot pass\r\n",
			"bastion": {
				"host": " 192.168.1.1\u00a0",
				"user": "\ufeffsopuser ",
				"password": " jump@123\u200b "
			}
		},
		{
			"host": "10.0.1.9",
			"protocol": "telnet",
			"user": "admin",
			"password": "abc"
		}
	]`

	mockProvider := &llm.MockProvider{Response: expectedJSON}
	service := NewAIService(mockProvider, mockProvider, config.NewManager())

	configs, err := service.ParseConnectIntent("任意输入")
	if err != nil {
		t.Fatalf("ParseConnectIntent failed: %v", err)
	}
	if len(configs) != 2 {
		t.Fatalf("Expected 2 configs, got %d", len(configs))
	}

	c := configs[0]
	if c.Name != "订单节点" || c.Host != "10.0.1.8" || c.User != "root" {
		t.Errorf("unexpected cleaned identifiers: name=%q host=%q user=%q", c.Name, c.Host, c.User)
	}
	if c.Password != "p@ss word#1" {
		t.Errorf("password = %q, want %q", c.Password, "p@ss word#1")
	}
	if c.RootPassword != "root pass" {
		t.Errorf("rootPassword = %q, want %q", c.RootPassword, "root pass")
	}
	if c.Port != 22 {
		t.Errorf("port = %d, want 22", c.Port)
	}
	if c.Bastion == nil {
		t.Fatal("expected bastion")
	}
	if c.Bastion.Host != "192.168.1.1" || c.Bastion.User != "sopuser" || c.Bastion.Password != "jump@123" || c.Bastion.Port != 22 {
		t.Errorf("unexpected cleaned bastion: %+v", c.Bastion)
	}
	if configs[1].Port != 23 {
		t.Errorf("telnet default port = %d, want 23", configs[1].Port)
	}
}

func TestAskWithContext(t *testing.T) {
	// Mock response that follows the new format
	expectedResponse := `## 排查思路
Based on the context, the answer is 42.

## 建议命令
echo 42`
	mockProvider := &llm.MockProvider{
		Response: expectedResponse,
	}

	cfgMgr := config.NewManager()
	service := NewAIService(mockProvider, mockProvider, cfgMgr)

	// Since AskWithContext now expects a directory path for Agent mode,
	// and we are using MockProvider which doesn't actually read files but returns static response,
	// we can pass a dummy directory.
	// However, if Agent mode is triggered, it will try to call ListFiles tool.
	// Our MockProvider (from llm package) doesn't implement ChatWithTools logic (it just returns Content).
	// So AskWithContext will receive the static content immediately as "Answer".
	
	// Wait, AskWithContext now calls RunAgent. RunAgent calls ChatWithTools.
	// llm.MockProvider implementation of ChatWithTools returns Content + ToolCalls.
	// If ToolCalls is empty, Agent loop finishes and returns Content.
	// So if we set Response in MockProvider, it will return that content and 0 ToolCalls.
	// This simulates a scenario where Agent decides to answer directly without looking at files.
	
	dummyDir := "/tmp/knowledge"
	question := "What is the answer?"

	// We need to pass context.Background() as first arg
	resp, err := service.AskWithContext(context.Background(), question, dummyDir)
	if err != nil {
		t.Fatalf("AskWithContext failed: %v", err)
	}

	if resp != expectedResponse {
		t.Errorf("Expected response %q, got %q", expectedResponse, resp)
	}
}

func TestGenerateLinuxCommand(t *testing.T) {
	fastProvider := &llm.MockProvider{
		Response: `{"command":"ls -la","explanation":"列出包含隐藏文件的详细信息"}`,
	}
	complexProvider := &llm.MockProvider{
		Response: `{"command":"echo should-not-use","explanation":"x"}`,
	}

	cfgMgr := config.NewManager()
	service := NewAIService(fastProvider, complexProvider, cfgMgr)

	result, err := service.GenerateLinuxCommand("列出当前目录所有文件")
	if err != nil {
		t.Fatalf("GenerateLinuxCommand failed: %v", err)
	}
	if result.Command != "ls -la" {
		t.Fatalf("Command = %q, want %q", result.Command, "ls -la")
	}
}
