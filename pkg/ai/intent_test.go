package ai

import (
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
	service := NewAIService(mockProvider, cfgMgr)

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
	service := NewAIService(mockProvider, cfgMgr)

	contextContent := "The answer to everything is 42."
	question := "What is the answer?"

	resp, err := service.AskWithContext(question, contextContent)
	if err != nil {
		t.Fatalf("AskWithContext failed: %v", err)
	}

	if resp != expectedResponse {
		t.Errorf("Expected response %q, got %q", expectedResponse, resp)
	}
}
