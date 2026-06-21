package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// skillTemplate 嵌入 skills/opscopilot-ops/SKILL.md 作为安装模板。
// 模板内含两个占位符，安装时由 renderSkillTemplate 替换：
//   {{OPSCOPILOT_BIN}}     —— 当前 exe 绝对路径（含引号，防路径空格）
//   {{OPSCOPILOT_VERSION}} —— 当前版本号（main.Version）
//
//go:embed skills/opscopilot-ops/SKILL.md
var skillTemplateFS embed.FS

const (
	skillTemplateName  = "skills/opscopilot-ops/SKILL.md"
	skillSubDir        = "opscopilot-ops" // 在用户指定父目录下创建的子目录名
	skillVersionFile   = ".version"        // 版本标记文件，内容为安装时的 main.Version
)

// readSkillTemplate 读取嵌入的 SKILL.md 模板原文。
func readSkillTemplate() (string, error) {
	data, err := skillTemplateFS.ReadFile(skillTemplateName)
	if err != nil {
		return "", fmt.Errorf("读取 skill 模板失败: %w", err)
	}
	return string(data), nil
}

// renderSkillTemplate 将模板占位符替换为实际值。
//   {{OPSCOPILOT_BIN}} → exe 绝对路径（不带引号；模板中需带引号处直接写 "{{OPSCOPILOT_BIN}}"）
//
// 不用 fmt.Sprintf("%q", ...) —— 它会把 Windows 路径的反斜杠过度转义为 \\。
// 版本号不进模板正文，仅写入独立的 .version 文件，避免 skill 正文随版本变动。
func renderSkillTemplate(tpl string, exePath string) string {
	return strings.ReplaceAll(tpl, "{{OPSCOPILOT_BIN}}", exePath)
}

// skillDir 保存 skill 的目标目录：<parentDir>/opscopilot-ops
func skillDir(parentDir string) string {
	return filepath.Join(parentDir, skillSubDir)
}

// CheckSkillStatus 检测指定父目录下是否已安装 OpsCopilot skill，以及版本是否最新。
// parentDir 为用户指定的 skill 父目录（如 ~/.claude/skills）。
// 返回 JSON 字符串，供前端解析展示状态。
//
// 状态机：
//   not_installed  —— 目录不存在或 .version 缺失
//   up_to_date     —— 已安装且版本与当前 exe 一致
//   outdated       —— 已安装但版本与当前 exe 不一致（可更新）
func (a *App) CheckSkillStatus(parentDir string) string {
	dir := strings.TrimSpace(parentDir)
	if dir == "" {
		return toJSONError("请输入 skill 安装目录")
	}

	versionFile := filepath.Join(skillDir(dir), skillVersionFile)
	data, err := os.ReadFile(versionFile)
	if err != nil {
		// 文件不存在视为未安装（不区分"目录不存在"与"目录存在但无 .version"，对用户意义相同）
		return skillStatusJSON("", Version, "not_installed")
	}

	installed := strings.TrimSpace(string(data))
	state := "outdated"
	if installed == Version {
		state = "up_to_date"
	}
	return skillStatusJSON(installed, Version, state)
}

// skillStatusJSON 构造 CheckSkillStatus 的成功返回。
func skillStatusJSON(installed, builtin, state string) string {
	result, _ := json.Marshal(map[string]interface{}{
		"success":   true,
		"installed": installed,
		"builtin":   builtin,
		"state":     state,
	})
	return string(result)
}

// InstallSkill 将内置 SKILL.md 安装（或更新）到指定父目录下的 opscopilot-ops/ 子目录。
// parentDir 为用户指定的 skill 父目录。
// 写入方式为原子写（临时文件 + rename），避免半截文件。
// 安装产物：
//   <parentDir>/opscopilot-ops/SKILL.md   —— 已渲染（含 exe 绝对路径）的 skill
//   <parentDir>/opscopilot-ops/.version    —— 安装时的版本号
func (a *App) InstallSkill(parentDir string) string {
	dir := strings.TrimSpace(parentDir)
	if dir == "" {
		return toJSONError("请输入 skill 安装目录")
	}

	tpl, err := readSkillTemplate()
	if err != nil {
		return toJSONError(err.Error())
	}

	exePath, err := os.Executable()
	if err != nil {
		return toJSONError(fmt.Sprintf("获取可执行文件路径失败: %v", err))
	}
	// Windows 下 os.Executable 返回的路径已是正斜杠或反斜杠混合，%q 会原样保留。

	content := renderSkillTemplate(tpl, exePath)

	targetDir := skillDir(dir)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return toJSONError(fmt.Sprintf("创建目录失败: %v", err))
	}

	skillPath := filepath.Join(targetDir, "SKILL.md")
	if err := writeAtomic(skillPath, []byte(content), 0644); err != nil {
		return toJSONError(fmt.Sprintf("写入 SKILL.md 失败: %v", err))
	}

	// 版本标记文件
	versionPath := filepath.Join(targetDir, skillVersionFile)
	if err := writeAtomic(versionPath, []byte(Version), 0644); err != nil {
		return toJSONError(fmt.Sprintf("写入版本标记失败: %v", err))
	}

	result, _ := json.Marshal(map[string]interface{}{
		"success": true,
		"path":    skillPath,
		"version": Version,
	})
	return string(result)
}

// writeAtomic 原子写入文件：先写 .tmp 再 rename，避免写入中途崩溃产生半截文件。
// 与 pkg/core/security/whitelist_manager.go 中的原子写实现一致。
func writeAtomic(path string, data []byte, perm os.FileMode) error {
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, perm); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}
