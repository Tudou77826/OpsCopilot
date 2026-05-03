package knowledge

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const moduleListFile = "module_list.json"

// ModuleConfigEntry 模块配置条目
type ModuleConfigEntry struct {
	Name        string `json:"name"`        // 模块名，如 "核心支付模块"
	Description string `json:"description"` // 一句话简介，如 "支付接口、退款、对账相关服务"
}

// ModuleConfig 模块配置
type ModuleConfig struct {
	Modules []ModuleConfigEntry `json:"modules"`
}

// LoadModuleConfig 加载模块配置
func LoadModuleConfig(knowledgeDir string) *ModuleConfig {
	path := filepath.Join(knowledgeDir, moduleListFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return DefaultModuleConfig()
	}
	var cfg ModuleConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultModuleConfig()
	}
	return &cfg
}

// SaveModuleConfig 保存模块配置
func SaveModuleConfig(knowledgeDir string, cfg *ModuleConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(knowledgeDir, moduleListFile)
	return os.WriteFile(path, data, 0644)
}

// DefaultModuleConfig 返回空模块列表
func DefaultModuleConfig() *ModuleConfig {
	return &ModuleConfig{Modules: nil}
}

// FormatModuleList 格式化模块列表为 LLM prompt 片段
// 返回空字符串表示无约束（LLM 自行推断）
func FormatModuleList(modules []ModuleConfigEntry) string {
	if len(modules) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("可选模块列表（请从中选择最匹配的模块）：\n")
	for _, m := range modules {
		if m.Description != "" {
			fmt.Fprintf(&sb, "- %s：%s\n", m.Name, m.Description)
		} else {
			fmt.Fprintf(&sb, "- %s\n", m.Name)
		}
	}
	sb.WriteString("如果都不匹配，使用内容中出现的实际模块名。")
	return sb.String()
}
