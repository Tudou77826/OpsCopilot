package knowledge

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const catalogFileName = ".catalog.json"

// BuildCatalog 构建/增量更新目录
// 遍历 dir 下所有 .md 文件，hash 比对后只重新解析变更文件
func BuildCatalog(dir string) (*Catalog, error) {
	existing := loadExistingCatalog(dir)

	currentFiles, err := walkMarkdownFiles(dir)
	if err != nil {
		return nil, fmt.Errorf("walk markdown files: %w", err)
	}

	for relPath, content := range currentFiles {
		hash := md5Hash(content)

		// 未变更 → 跳过
		if existing.FileHash != nil && existing.FileHash[relPath] == hash {
			continue
		}

		// 变更/新增 → 重新解析
		serviceName, moduleName, entries := parseDocument(relPath, content)
		_ = serviceName // 避免编译警告
		_ = moduleName
		existing.ReplaceEntries(relPath, serviceName, moduleName, entries)

		if existing.FileHash == nil {
			existing.FileHash = make(map[string]string)
		}
		existing.FileHash[relPath] = hash
	}

	// 清理已删除文件的条目
	existingPaths := make(map[string]bool, len(currentFiles))
	for p := range currentFiles {
		existingPaths[p] = true
	}
	existing.RemoveDeletedFiles(existingPaths)

	existing.Version = 1
	existing.BuildAt = time.Now()

	// 持久化
	if err := saveCatalog(dir, existing); err != nil {
		return nil, fmt.Errorf("save catalog: %w", err)
	}

	return existing, nil
}

// loadExistingCatalog 加载已有 catalog.json，失败则返回空 Catalog
func loadExistingCatalog(dir string) *Catalog {
	path := filepath.Join(dir, catalogFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return &Catalog{FileHash: make(map[string]string)}
	}
	var cat Catalog
	if err := json.Unmarshal(data, &cat); err != nil {
		return &Catalog{FileHash: make(map[string]string)}
	}
	if cat.FileHash == nil {
		cat.FileHash = make(map[string]string)
	}
	return &cat
}

// saveCatalog 持久化 catalog 到文件
func saveCatalog(dir string, cat *Catalog) error {
	data, err := json.MarshalIndent(cat, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, catalogFileName)
	return os.WriteFile(path, data, 0644)
}

// walkMarkdownFiles 遍历所有 .md 文件，返回 relPath → content
func walkMarkdownFiles(dir string) (map[string]string, error) {
	files := make(map[string]string)

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// 跳过目录、非 md 文件、以及 .bak 备份文件
		if d.IsDir() || strings.HasSuffix(strings.ToLower(d.Name()), ".bak") ||
			!strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		// 跳过 catalog 文件自身
		if d.Name() == catalogFileName {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relPath)] = string(data)
		return nil
	})

	return files, err
}

// parseDocument 解析单个文档
// 返回 serviceName, moduleName, 以及提取的场景条目列表
func parseDocument(relPath string, content string) (string, string, []ScenarioEntry) {
	// 提取 Front Matter
	fm, body := extractFrontMatter(content)

	serviceName := fm["service"]
	moduleName := fm["module"]
	docType := fm["type"]

	// 如果没有 Front Matter，尝试从服务信息表格提取
	if serviceName == "" {
		serviceName = extractServiceFromTable(body)
	}
	if moduleName == "" {
		moduleName = extractModuleFromTable(body)
	}

	// 没有 service 信息 → 跳过该文件（避免设计文档中的示例场景被索引）
	if serviceName == "" {
		return "", "", nil
	}

	if moduleName == "" {
		moduleName = "默认模块"
	}

	// 判断文档类型
	if docType == "" {
		if strings.Contains(relPath, "troubleshooting/") {
			docType = "archive"
		} else {
			docType = "sop"
		}
	}

	// 根据类型提取场景
	var entries []ScenarioEntry
	if docType == "archive" {
		entries = extractArchiveScenarios(body, relPath)
	} else {
		entries = extractSOPScenarios(body, relPath)
	}

	return serviceName, moduleName, entries
}

// extractFrontMatter 提取 YAML Front Matter（简化版，只做 key: value 解析）
func extractFrontMatter(content string) (map[string]string, string) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return nil, content
	}

	// 找到第二个 ---
	endIdx := strings.Index(content[3:], "---")
	if endIdx < 0 {
		return nil, content
	}

	fmText := strings.TrimSpace(content[3 : endIdx+3])
	body := strings.TrimSpace(content[endIdx+6:])

	fields := make(map[string]string)
	for _, line := range strings.Split(fmText, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		// 去掉引号
		val = strings.Trim(val, "\"'")
		if key != "" {
			fields[key] = val
		}
	}

	return fields, body
}

// extractServiceFromTable 从 ## 服务信息 表格提取 微服务 字段
func extractServiceFromTable(content string) string {
	return extractTableField(content, "微服务")
}

// extractModuleFromTable 从 ## 服务信息 表格提取 模块 字段
func extractModuleFromTable(content string) string {
	return extractTableField(content, "模块")
}

func extractTableField(content string, fieldName string) string {
	// 找到 ## 服务信息 区域
	lines := strings.Split(content, "\n")
	inTable := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## 服务信息") {
			inTable = true
			continue
		}
		if inTable && strings.HasPrefix(trimmed, "## ") {
			break // 下一个 section，退出表格区域
		}
		if inTable && strings.HasPrefix(trimmed, "|") {
			// 解析表格行
			cols := strings.Split(trimmed, "|")
			// cols[0] 是空的（前导 |），实际数据从 cols[1] 开始
			if len(cols) >= 3 {
				key := strings.TrimSpace(cols[1])
				val := strings.TrimSpace(cols[2])
				if key == fieldName {
					return val
				}
			}
		}
	}
	return ""
}

// inferServiceFromPath 从文件路径推断服务名
func inferServiceFromPath(relPath string) string {
	name := filepath.Base(relPath)
	name = strings.TrimSuffix(name, filepath.Ext(name))
	// 去掉常见后缀
	name = strings.TrimSuffix(name, "_sop")
	name = strings.TrimSuffix(name, "_troubleshooting")
	name = strings.TrimSuffix(name, "_maintenance")
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")
	if name == "" {
		return "Unknown"
	}
	// 首字母大写
	return strings.Title(name)
}

// extractSOPScenarios 从 SOP 文档提取场景
// 识别 `## 场景：` 或 `### 场景：` 或 `### 场景X：` 等格式的标题
func extractSOPScenarios(content string, relPath string) []ScenarioEntry {
	lines := strings.Split(content, "\n")

	// 正则匹配场景标题
	scenarioRe := regexp.MustCompile(`^#{1,4}\s+场景[一二三四五六七八九十\d]*[：:]\s*(.+)`)

	type scenarioRange struct {
		titleLine int
		title     string
	}

	var ranges []scenarioRange
	for i, line := range lines {
		m := scenarioRe.FindStringSubmatch(strings.TrimSpace(line))
		if m != nil {
			ranges = append(ranges, scenarioRange{
				titleLine: i,
				title:     strings.TrimSpace(m[1]),
			})
		}
	}

	if len(ranges) == 0 {
		return nil
	}

	var entries []ScenarioEntry
	for idx, r := range ranges {
		startLine := r.titleLine + 1 // 0-based, 场景标题的下一行

		// 确定结束行
		var endLine int
		if idx+1 < len(ranges) {
			endLine = ranges[idx+1].titleLine
		} else {
			endLine = len(lines)
		}

		// 提取该段落的内容
		sectionLines := lines[startLine:endLine]
		phenomena := extractFieldFromSection(sectionLines, "现象")
		keywordsStr := extractFieldFromSection(sectionLines, "关键词")
		componentsStr := extractFieldFromSection(sectionLines, "涉及组件")

		keywords := splitCommaList(keywordsStr)
		components := splitCommaList(componentsStr)

		entry := ScenarioEntry{
			Title:      r.title,
			File:       relPath,
			LineStart:  r.titleLine + 1, // 1-based
			LineEnd:    endLine,          // 1-based, exclusive
			Phenomena:  phenomena,
			Keywords:   keywords,
			Components: components,
			Type:       "sop",
		}
		entries = append(entries, entry)
	}

	return entries
}

// extractArchiveScenarios 从归档文件提取场景
func extractArchiveScenarios(content string, relPath string) []ScenarioEntry {
	lines := strings.Split(content, "\n")

	phenomena := ""
	keywords := []string{}
	components := []string{}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## 问题现象") {
			// 读取后续非空非标题行作为现象描述
			phenomena = collectSectionContent(lines, i+1)
		}
		if strings.HasPrefix(trimmed, "## 关键词") {
			content := collectSectionContent(lines, i+1)
			keywords = splitCommaList(content)
		}
		if strings.HasPrefix(trimmed, "## 涉及组件") {
			content := collectSectionContent(lines, i+1)
			components = splitCommaList(content)
		}
	}

	if phenomena == "" {
		// 没有现象描述，跳过此归档
		return nil
	}

	// 归档文件整体作为一个场景
	return []ScenarioEntry{
		{
			Title:      extractArchiveTitle(content),
			File:       relPath,
			LineStart:  1,
			LineEnd:    len(lines) + 1,
			Phenomena:  phenomena,
			Keywords:   keywords,
			Components: components,
			Type:       "archive",
		},
	}
}

// extractArchiveTitle 从归档文件提取标题
func extractArchiveTitle(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") && !strings.HasPrefix(trimmed, "## ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		}
	}
	return "归档记录"
}

// collectSectionContent 收集一个章节的内容（到下一个同级或更高级标题为止）
func collectSectionContent(lines []string, startIdx int) string {
	var sb strings.Builder
	for i := startIdx; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		// 遇到同级或更高级标题停止
		if strings.HasPrefix(trimmed, "## ") {
			break
		}
		// 跳过列表标记
		trimmed = strings.TrimPrefix(trimmed, "- ")
		trimmed = strings.TrimPrefix(trimmed, "* ")
		trimmed = strings.TrimSpace(trimmed)
		if trimmed == "" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString(trimmed)
	}
	return sb.String()
}

// extractFieldFromSection 从段落行列表中提取 `- **字段名**: 值` 格式的字段
func extractFieldFromSection(lines []string, fieldName string) string {
	pattern := fmt.Sprintf("- **%s**:", fieldName)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, pattern) {
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, pattern))
			return val
		}
		// 也尝试 `**字段名**:` 格式（不带前导 - ）
		altPattern := fmt.Sprintf("**%s**:", fieldName)
		if strings.HasPrefix(trimmed, altPattern) {
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, altPattern))
			return val
		}
	}
	return ""
}

// splitCommaList 将逗号分隔的字符串拆分为字符串切片
func splitCommaList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// md5Hash 计算内容的 MD5 哈希
func md5Hash(content string) string {
	h := md5.Sum([]byte(content))
	return fmt.Sprintf("%x", h)
}
