package sshclient

import (
	"strings"
	"testing"
)

func TestSSHConnection(t *testing.T) {
	// 真实环境集成测试
	// 注意：在CI/CD环境中通常会Mock或跳过此类测试
	config := &ConnectConfig{
		Host:     "39.108.107.148",
		Port:     22,
		User:     "root",
		Password: "zhangyibo123.",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// 验证基础命令执行
	output, err := client.Run("whoami")
	if err != nil {
		t.Fatalf("Failed to run command: %v", err)
	}

	trimmedOutput := strings.TrimSpace(output)
	if trimmedOutput != "root" {
		t.Errorf("Expected 'root', got '%s'", trimmedOutput)
	}
	
	t.Logf("Successfully connected and executed command. Output: %s", trimmedOutput)
}
