package ai

import (
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
			"bastion": {
				"host": "10.0.0.1",
				"port": 22,
				"user": "admin",
				"password": "abc"
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

	service := NewAIService(mockProvider)

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
	if c1.Bastion == nil {
		t.Fatal("Expected bastion config, got nil")
	}
	if c1.Bastion.Host != "10.0.0.1" {
		t.Errorf("Expected bastion host 10.0.0.1, got %s", c1.Bastion.Host)
	}
}
