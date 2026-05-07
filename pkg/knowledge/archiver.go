package knowledge

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"opscopilot/pkg/recorder"
)

// 预编译正则表达式，避免每次调用都重新编译
var unsafeFileNameChar = regexp.MustCompile(`[^\p{Han}a-zA-Z0-9_\-]`)
var trailingHR = regexp.MustCompile(`(?m)\n---+\s*$`)

// ArchiveInput 归档输入参数
type ArchiveInput struct {
	Session    *recorder.RecordingSession
	Conclusion string // AI 生成的结论（Markdown 格式）
	Service    string // 用户选择的微服务名
	Module     string // 用户选择的模块名
	FilePath   string // 追加到的目标文件（空则新建）
}

// AppendRecord 追加排查记录到目标文件，返回文件路径
func AppendRecord(knowledgeDir string, input *ArchiveInput) (string, error) {
	if input.Service == "" {
		return "", fmt.Errorf("service name is required")
	}
	if input.Module == "" {
		input.Module = "默认模块"
	}

	record := buildArchiveRecord(input)

	var targetPath string
	if input.FilePath != "" {
		// 追加到已有文件 — 先验证路径安全性
		targetPath = filepath.Join(knowledgeDir, input.FilePath)
		if err := validatePathWithinDir(knowledgeDir, targetPath); err != nil {
			return "", fmt.Errorf("invalid file path: %w", err)
		}
		if err := appendToFile(targetPath, record); err != nil {
			return "", fmt.Errorf("append to file: %w", err)
		}
	} else {
		// 创建新文件 — 放入 archive/ 子目录，与已有文档区隔
		fileName := sanitizeFileName(input.Service, input.Module) + ".md"
		targetPath = filepath.Join(knowledgeDir, "archive", fileName)
		// 确保 new/ 目录存在
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return "", fmt.Errorf("create new directory: %w", err)
		}
		content := buildNewFile(input.Service, input.Module, record)
		if err := os.WriteFile(targetPath, []byte(content), 0644); err != nil {
			return "", fmt.Errorf("create file: %w", err)
		}
	}

	// 返回相对路径
	relPath, err := filepath.Rel(knowledgeDir, targetPath)
	if err != nil {
		return "", fmt.Errorf("get relative path: %w", err)
	}
	return filepath.ToSlash(relPath), nil
}

// validatePathWithinDir 验证 targetPath 在 baseDir 内部，防止路径遍历
func validatePathWithinDir(baseDir, targetPath string) error {
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("resolve target path: %w", err)
	}
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return fmt.Errorf("resolve base dir: %w", err)
	}
	if !strings.HasPrefix(absTarget, absBase+string(filepath.Separator)) && absTarget != absBase {
		return fmt.Errorf("path traversal detected")
	}
	return nil
}

// buildArchiveRecord 构建归档记录文本
// 策略：保留 AI 结论的完整原文，只在开头加 `## 场景：` 标题 + bullet points（供 Catalog 索引器识别）
func buildArchiveRecord(input *ArchiveInput) string {
	var sb strings.Builder

	// 提取索引字段（仅用于 bullet points，供 Catalog 索引器识别）
	keywords := extractSectionAny(input.Conclusion, "关键词", "关键字")
	phenomena := extractSectionAny(input.Conclusion, "问题现象", "问题描述", "现象", "故障现象")
	components := extractSectionAny(input.Conclusion, "涉及组件", "相关组件", "组件")

	// 标题：从现象中提取简短描述
	title := sanitizeTitle(phenomena)
	if title == "" && input.Session != nil {
		title = input.Session.Problem
	}
	if title == "" {
		title = "未命名问题"
	}

	dateStr := time.Now().Format("2006-01-02")
	sb.WriteString(fmt.Sprintf("\n\n## 场景：%s - %s 排查记录\n", title, dateStr))

	// 索引用 bullet points（Catalog extractSOPScenarios 需要识别这些字段）
	if phenomena != "" {
		sb.WriteString(fmt.Sprintf("- **现象**: %s\n", singleLine(phenomena)))
	}
	if keywords != "" {
		sb.WriteString(fmt.Sprintf("- **关键词**: %s\n", singleLine(keywords)))
	}
	if components != "" {
		sb.WriteString(fmt.Sprintf("- **涉及组件**: %s\n", singleLine(components)))
	}

	// 完整保留 AI 结论原文（去掉可能的尾部空行）
	sb.WriteString("\n")
	conclusionBody := strings.TrimSpace(input.Conclusion)
	sb.WriteString(conclusionBody)
	sb.WriteString("\n")

	// 归档元信息
	sessionID := ""
	if input.Session != nil {
		sessionID = input.Session.ID
	}
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	sb.WriteString(fmt.Sprintf("\n---\n*归档时间: %s | 会话ID: %s*\n", timestamp, sessionID))

	return sb.String()
}

// buildNewFile 创建新文件内容（带 front matter）
func buildNewFile(service, module, record string) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("service: %q\n", service))
	sb.WriteString(fmt.Sprintf("module: %q\n", module))
	sb.WriteString("type: sop\n")
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("# %s - %s 运维文档\n\n", service, module))
	sb.WriteString("## 服务信息\n\n")
	sb.WriteString(fmt.Sprintf("| 字段 | 值 |\n|------|----|\n| 微服务 | %s |\n| 模块 | %s |\n\n", service, module))
	sb.WriteString(record)
	sb.WriteString("\n")
	return sb.String()
}

// extractSection 提取指定 heading 下的内容到下一个 ## 之前
func extractSection(md, heading string) string {
	return extractSectionAny(md, heading)
}

// extractSectionAny 按多个候选 heading 名提取，返回第一个命中的内容
func extractSectionAny(md string, headings ...string) string {
	for _, h := range headings {
		if result := extractSingleSection(md, h); result != "" {
			return result
		}
	}
	return ""
}

// extractSingleSection 提取单个 heading 下的内容到下一个 ## 之前
func extractSingleSection(md, heading string) string {
	if md == "" {
		return ""
	}

	lines := strings.Split(md, "\n")
	var sb strings.Builder
	collecting := false
	headingPrefix := "## " + heading

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// 检查是否是目标 heading
		if strings.HasPrefix(trimmed, headingPrefix) {
			collecting = true
			continue
		}

		// 如果正在收集，遇到下一个 ## heading 则停止
		if collecting {
			if strings.HasPrefix(trimmed, "## ") {
				break
			}
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(line)
		}
	}

	result := strings.TrimSpace(sb.String())
	// 去掉尾部的 Markdown 分隔线（---）
	result = trailingHR.ReplaceAllString(result, "")
	return strings.TrimSpace(result)
}

// extractKeywords 提取关键词列表
func extractKeywords(conclusion string) []string {
	content := extractSection(conclusion, "关键词")
	return splitCommaList(content)
}

// sanitizeTitle 清理场景标题：取第一行，去除特殊字符
func sanitizeTitle(s string) string {
	if s == "" {
		return ""
	}
	// 取第一行
	lines := strings.Split(s, "\n")
	title := strings.TrimSpace(lines[0])

	// 去掉列表标记
	title = strings.TrimPrefix(title, "- ")
	title = strings.TrimPrefix(title, "* ")
	title = strings.TrimPrefix(title, "- **现象**: ")
	title = strings.TrimPrefix(title, "**现象**: ")

	// 截断到合理长度（按 rune 截断，避免切断多字节字符）
	runes := []rune(title)
	if len(runes) > 80 {
		runes = runes[:80]
		// 在最近的分隔符处截断
		for i := len(runes) - 1; i > 40; i-- {
			if runes[i] == '，' || runes[i] == '。' || runes[i] == ',' || runes[i] == ' ' {
				runes = runes[:i]
				break
			}
		}
		title = string(runes)
	}

	return strings.TrimSpace(title)
}

// sanitizeFileName 生成文件名：service_module 格式
func sanitizeFileName(service, module string) string {
	name := fmt.Sprintf("%s_%s", service, module)
	// 空格转下划线
	name = strings.ReplaceAll(name, " ", "_")
	// 去掉文件名不安全字符（保留字母、数字、下划线、中文、连字符）
	name = unsafeFileNameChar.ReplaceAllString(name, "_")
	// 去掉连续下划线
	for strings.Contains(name, "__") {
		name = strings.ReplaceAll(name, "__", "_")
	}
	name = strings.Trim(name, "_")
	return name
}

// singleLine 将多行文本压缩为单行
func singleLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// appendToFile 追加内容到文件末尾
func appendToFile(path, content string) error {
	// 确保父目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}
