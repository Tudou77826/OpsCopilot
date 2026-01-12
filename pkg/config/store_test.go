package config

import (
	"os"
	"testing"
)

func TestConfigLoad(t *testing.T) {
	// 创建配置管理器
	mgr := NewManager()

	// 加载配置
	if err := mgr.Load(); err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}

	// 验证配置文件存在
	if _, err := os.Stat("config.json"); os.IsNotExist(err) {
		t.Error("config.json 文件不存在")
	}

	if _, err := os.Stat("prompts.json"); os.IsNotExist(err) {
		t.Error("prompts.json 文件不存在")
	}

	if _, err := os.Stat("quick_commands.json"); os.IsNotExist(err) {
		t.Error("quick_commands.json 文件不存在")
	}

	// 验证 Prompts 被正确加载
	if mgr.Config.Prompts == nil {
		t.Fatal("Prompts 未初始化")
	}

	expectedPrompts := []string{
		"smart_connect",
		"qa_prompt",
		"conclusion_prompt",
		"polish_prompt",
		"troubleshoot_prompt",
	}

	for _, key := range expectedPrompts {
		if _, ok := mgr.Config.Prompts[key]; !ok {
			t.Errorf("缺少 prompt: %s", key)
		}
	}

	// 验证 QuickCommands 被正确加载
	if mgr.Config.QuickCommands == nil {
		t.Fatal("QuickCommands 未初始化")
	}

	t.Logf("配置加载成功!")
	t.Logf("- Prompts 数量: %d", len(mgr.Config.Prompts))
	t.Logf("- QuickCommands 数量: %d", len(mgr.Config.QuickCommands))
	t.Logf("- LLM Model: %s", mgr.Config.LLM.Model)
}
