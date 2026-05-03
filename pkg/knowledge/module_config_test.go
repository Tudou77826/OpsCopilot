package knowledge

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadModuleConfig_FileExists(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "module_config_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	content := `{
  "modules": [
    {"name": "核心支付模块", "description": "支付接口、退款、对账相关服务"},
    {"name": "订单模块", "description": "订单创建、状态流转、超时处理"}
  ]
}`
	if err := os.WriteFile(filepath.Join(tmpDir, moduleListFile), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := LoadModuleConfig(tmpDir)
	if len(cfg.Modules) != 2 {
		t.Fatalf("len(Modules) = %d, want 2", len(cfg.Modules))
	}
	if cfg.Modules[0].Name != "核心支付模块" {
		t.Errorf("Modules[0].Name = %q, want '核心支付模块'", cfg.Modules[0].Name)
	}
	if cfg.Modules[0].Description != "支付接口、退款、对账相关服务" {
		t.Errorf("Modules[0].Description = %q, unexpected", cfg.Modules[0].Description)
	}
	if cfg.Modules[1].Name != "订单模块" {
		t.Errorf("Modules[1].Name = %q, want '订单模块'", cfg.Modules[1].Name)
	}
}

func TestLoadModuleConfig_FileNotExists(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "module_config_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := LoadModuleConfig(tmpDir)
	if cfg == nil {
		t.Fatal("cfg should not be nil")
	}
	if len(cfg.Modules) != 0 {
		t.Errorf("len(Modules) = %d, want 0 for missing file", len(cfg.Modules))
	}
}

func TestSaveModuleConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "module_config_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &ModuleConfig{
		Modules: []ModuleConfigEntry{
			{Name: "Redis模块", Description: "Redis缓存、分布式锁、连接池"},
		},
	}

	if err := SaveModuleConfig(tmpDir, cfg); err != nil {
		t.Fatal(err)
	}

	// 重新加载验证
	loaded := LoadModuleConfig(tmpDir)
	if len(loaded.Modules) != 1 {
		t.Fatalf("len(Modules) = %d, want 1", len(loaded.Modules))
	}
	if loaded.Modules[0].Name != "Redis模块" {
		t.Errorf("Modules[0].Name = %q, want 'Redis模块'", loaded.Modules[0].Name)
	}
}

func TestFormatModuleList(t *testing.T) {
	t.Run("empty modules returns empty string", func(t *testing.T) {
		result := FormatModuleList(nil)
		if result != "" {
			t.Errorf("expected empty string, got %q", result)
		}
	})

	t.Run("modules with descriptions", func(t *testing.T) {
		modules := []ModuleConfigEntry{
			{Name: "核心支付模块", Description: "支付接口、退款"},
			{Name: "订单模块", Description: "订单创建、状态流转"},
		}
		result := FormatModuleList(modules)
		if result == "" {
			t.Error("expected non-empty string")
		}
		if !contains(result, "核心支付模块：支付接口、退款") {
			t.Error("should contain module with description")
		}
		if !contains(result, "订单模块：订单创建、状态流转") {
			t.Error("should contain second module")
		}
		if !contains(result, "如果都不匹配") {
			t.Error("should contain fallback instruction")
		}
	})

	t.Run("modules without descriptions", func(t *testing.T) {
		modules := []ModuleConfigEntry{
			{Name: "测试模块"},
		}
		result := FormatModuleList(modules)
		if !contains(result, "- 测试模块\n") {
			t.Error("should contain module without description colon")
		}
	})
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
