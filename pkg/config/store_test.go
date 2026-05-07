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

	if _, err := os.Stat("quick_commands.json"); os.IsNotExist(err) {
		t.Error("quick_commands.json 文件不存在")
	}

	// 验证 QuickCommands 被正确加载
	if mgr.Config.QuickCommands == nil {
		t.Fatal("QuickCommands 未初始化")
	}

	t.Logf("配置加载成功!")
	t.Logf("- QuickCommands 数量: %d", len(mgr.Config.QuickCommands))
	t.Logf("- LLM FastModel: %s", mgr.Config.LLM.FastModel)
	t.Logf("- LLM ComplexModel: %s", mgr.Config.LLM.ComplexModel)
}
