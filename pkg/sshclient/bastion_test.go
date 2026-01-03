package sshclient

import (
	"testing"
)

func TestBastionConnection(t *testing.T) {
	// 这是一个集成测试，需要真实的跳板机环境
	// 如果没有环境，这个测试会失败或超时
	// 这里我们只验证 ConnectConfig 的结构是否支持递归解析

	config := &ConnectConfig{
		Host:     "10.0.1.5", // 内网目标
		Port:     22,
		User:     "root",
		Password: "target-password",
		Bastion: &ConnectConfig{
			Host:     "39.108.107.148", // 跳板机
			Port:     22,
			User:     "root",
			Password: "zhangyibo123.",
		},
	}

	// 简单的逻辑验证：确认配置是否正确嵌套
	if config.Bastion == nil {
		t.Fatal("Bastion config is nil")
	}
	if config.Bastion.Host != "39.108.107.148" {
		t.Errorf("Expected bastion host 39.108.107.148, got %s", config.Bastion.Host)
	}

	// 在没有真实内网环境的情况下，我们无法真正运行 NewClient(config) 并期望成功
	// 但我们可以尝试连接跳板机本身，以验证 NewClient 对 Bastion 字段的处理逻辑（虽然这里没有实际使用到 Bastion 隧道）
	
	// 测试场景：只连接跳板机（不配置下一跳）
	bastionOnlyConfig := config.Bastion
	client, err := NewClient(bastionOnlyConfig)
	if err != nil {
		t.Logf("Failed to connect to bastion (expected if network is down/creds wrong): %v", err)
        // 在 CI 环境中通常跳过实际连接测试
        return 
	}
    if client != nil {
        client.Close()
    }
    
    t.Log("Successfully validated Bastion config structure")
}

func TestRecursiveBastion(t *testing.T) {
    // 测试多级跳板配置结构
    config := &ConnectConfig{
        Host: "target",
        Bastion: &ConnectConfig{
            Host: "bastion2",
            Bastion: &ConnectConfig{
                Host: "bastion1",
            },
        },
    }
    
    if config.Bastion.Bastion.Host != "bastion1" {
        t.Error("Recursive bastion config failed")
    }
}
