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
//   {{OPSCOPILOT_BIN}}     —— 当前 exe 绝对路径（模板中带引号处写 "{{OPSCOPILOT_BIN}}"）
//   {{OPSCOPILOT_VERSION}} —— 当前版本号，渲染进 frontmatter 的 version 字段
//
// 版本信息以 frontmatter 的 version 字段为唯一来源，不再使用独立 .version 文件，
// 避免信息冗余和两处不同步的风险。
//
//go:embed skills/opscopilot-ops/SKILL.md
var skillTemplateFS embed.FS

const (
	skillTemplateName = "skills/opscopilot-ops/SKILL.md"
	skillSubDir       = "opscopilot-ops" // 在用户指定父目录下创建的子目录名
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
//   {{OPSCOPILOT_BIN}}     → exe 绝对路径（不带引号；模板中需带引号处直接写 "{{OPSCOPILOT_BIN}}"）
//   {{OPSCOPILOT_VERSION}} → 当前版本号（main.Version），渲染进 frontmatter 的 version 字段
//
// 不用 fmt.Sprintf("%q", ...) —— 它会把 Windows 路径的反斜杠过度转义为 \\。
func renderSkillTemplate(tpl string, exePath string) string {
	out := strings.ReplaceAll(tpl, "{{OPSCOPILOT_VERSION}}", Version)
	return strings.ReplaceAll(out, "{{OPSCOPILOT_BIN}}", exePath)
}

// skillDir 保存 skill 的目标目录：<parentDir>/opscopilot-ops
func skillDir(parentDir string) string {
	return filepath.Join(parentDir, skillSubDir)
}

// parseSkillVersion 从已安装的 SKILL.md 内容中解析 frontmatter 的 version 字段。
// frontmatter 形如 `version: '1.8.0'` 或 `version: 1.8.0`，用简单的行扫描即可，
// 避免引入 YAML 依赖。找不到时返回空串。
func parseSkillVersion(content string) string {
	lines := strings.Split(content, "\n")
	inFrontmatter := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			}
			break // frontmatter 结束
		}
		if !inFrontmatter {
			continue
		}
		if strings.HasPrefix(trimmed, "version:") {
			v := strings.TrimSpace(strings.TrimPrefix(trimmed, "version:"))
			// 去掉可选的引号
			v = strings.Trim(v, "\"'")
			return v
		}
	}
	return ""
}

// CheckSkillStatus 检测指定父目录下是否已安装 OpsCopilot skill，以及版本是否最新。
// parentDir 为用户指定的 skill 父目录（如 ~/.claude/skills）。
// 版本信息从已安装 SKILL.md 的 frontmatter 解析，不再依赖独立 .version 文件。
// 返回 JSON 字符串，供前端解析展示状态。
//
// 状态机：
//   not_installed  —— SKILL.md 不存在
//   up_to_date     —— 已安装且 version 与当前 exe 一致
//   outdated       —— 已安装但 version 与当前 exe 不一致（可更新）
func (a *App) CheckSkillStatus(parentDir string) string {
	dir := strings.TrimSpace(parentDir)
	if dir == "" {
		return toJSONError("请输入 skill 安装目录")
	}

	skillPath := filepath.Join(skillDir(dir), "SKILL.md")
	data, err := os.ReadFile(skillPath)
	if err != nil {
		// 文件不存在视为未安装
		return skillStatusJSON("", Version, "not_installed")
	}

	installed := parseSkillVersion(string(data))
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
//   <parentDir>/opscopilot-ops/SKILL.md   —— 已渲染（含 exe 绝对路径 + 版本号）的 skill
//
// 版本信息内嵌于 SKILL.md 的 frontmatter（version 字段），不另写 .version 文件。
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

	content := renderSkillTemplate(tpl, exePath)

	targetDir := skillDir(dir)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return toJSONError(fmt.Sprintf("创建目录失败: %v", err))
	}

	skillPath := filepath.Join(targetDir, "SKILL.md")
	if err := writeAtomic(skillPath, []byte(content), 0644); err != nil {
		return toJSONError(fmt.Sprintf("写入 SKILL.md 失败: %v", err))
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
