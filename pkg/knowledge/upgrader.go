package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const upgradeStateFile = ".upgrade_state.json"

// UpgradeState 记录已整理文件的状态
type UpgradeState struct {
	Files map[string]FileUpgradeState `json:"files"`
}

// FileUpgradeState 单个文件的整理状态
type FileUpgradeState struct {
	Hash       string    `json:"hash"`
	UpgradedAt time.Time `json:"upgradedAt"`
	BackupFile string    `json:"backupFile,omitempty"` // "old_doc.md.bak"
}

// UpgradeResult 单个文件的整理结果
type UpgradeResult struct {
	File   string `json:"file"`
	Status string `json:"status"` // "upgraded" | "skipped" | "error"
	Error  string `json:"error,omitempty"`

	backupFile string // 内部使用：备份文件名
}

// ProgressFunc 进度回调函数
// stage: "scanning" | "processing" | "catalog" | "done"
// current: 当前已处理的文件数
// total: 总文件数
// file: 当前处理的文件名
// message: 人类可读的进度描述
type ProgressFunc func(stage string, current, total int, file, message string)

// UpgradeDocuments 扫描知识库目录，对新增或修改过的 .md 文件进行整理
// 使用 hash 增量跟踪，已整理且内容未变的文件自动跳过
func UpgradeDocuments(ctx context.Context, knowledgeDir string, reorganizer ContentReorganizer, onProgress ProgressFunc) ([]UpgradeResult, error) {
	// 扫描所有 .md 文件
	if onProgress != nil {
		onProgress("scanning", 0, 0, "", "正在扫描知识库文件...")
	}
	currentFiles, err := walkMarkdownFiles(knowledgeDir)
	if err != nil {
		return nil, fmt.Errorf("scan markdown files: %w", err)
	}

	totalFiles := len(currentFiles)
	if totalFiles == 0 {
		if onProgress != nil {
			onProgress("done", 0, 0, "", "知识库目录为空，没有需要整理的文件")
		}
		return nil, nil
	}

	// 加载已有状态
	state := loadUpgradeState(knowledgeDir)
	if state.Files == nil {
		state.Files = make(map[string]FileUpgradeState)
	}

	// 确定文件处理顺序（按路径排序，便于跟踪进度）
	type fileEntry struct {
		relPath string
		content string
	}
	var orderedFiles []fileEntry
	for relPath, content := range currentFiles {
		orderedFiles = append(orderedFiles, fileEntry{relPath: relPath, content: content})
	}

	var results []UpgradeResult

	for i, entry := range orderedFiles {
		relPath := entry.relPath
		content := entry.content
		hash := md5Hash(content)

		// 已整理且 hash 匹配 → 跳过（不报进度，减少噪音）
		if existing, ok := state.Files[relPath]; ok && existing.Hash == hash {
			results = append(results, UpgradeResult{
				File:   relPath,
				Status: "skipped",
			})
			continue
		}

		// 需要整理 — 报告进度
		if onProgress != nil {
			onProgress("processing", i+1, totalFiles, relPath,
				fmt.Sprintf("正在处理 (%d/%d): %s", i+1, totalFiles, relPath))
		}

		result := upgradeFile(ctx, knowledgeDir, relPath, content, reorganizer)
		results = append(results, result)

		if result.Status == "upgraded" {
			// 重新读取文件内容以获取更新后的 hash
			fullPath := filepath.Join(knowledgeDir, filepath.FromSlash(relPath))
			if newContent, err := os.ReadFile(fullPath); err == nil {
				state.Files[relPath] = FileUpgradeState{
					Hash:       md5Hash(string(newContent)),
					UpgradedAt: time.Now(),
					BackupFile: result.backupFile,
				}
			}
		}
		// 失败的文件不更新状态，下次重试
	}

	// 清理已删除文件的状态
	existingPaths := make(map[string]bool, len(currentFiles))
	for p := range currentFiles {
		existingPaths[p] = true
	}
	for p := range state.Files {
		if !existingPaths[p] {
			delete(state.Files, p)
		}
	}

	// 持久化状态
	if err := saveUpgradeState(knowledgeDir, state); err != nil {
		return results, fmt.Errorf("save upgrade state: %w", err)
	}

	return results, nil
}

// upgradeFile 整理单个文件：LLM 重组 → 备份原文件 → 写入新文件
func upgradeFile(ctx context.Context, knowledgeDir, relPath, content string, reorganizer ContentReorganizer) UpgradeResult {
	// 使用 LLM 重组文档
	doc, err := reorganizer.Reorganize(ctx, content)
	if err != nil {
		return UpgradeResult{
			File:   relPath,
			Status: "error",
			Error:  fmt.Sprintf("reorganize failed: %v", err),
		}
	}

	if doc.Content == "" {
		return UpgradeResult{
			File:   relPath,
			Status: "error",
			Error:  "reorganize produced empty content",
		}
	}

	fullPath := filepath.Join(knowledgeDir, filepath.FromSlash(relPath))

	// 备份原文件
	backupFile, err := backupOriginal(fullPath)
	if err != nil {
		return UpgradeResult{
			File:   relPath,
			Status: "error",
			Error:  fmt.Sprintf("backup failed: %v", err),
		}
	}

	// 写入重组后的内容
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return UpgradeResult{
			File:   relPath,
			Status: "error",
			Error:  fmt.Sprintf("create directory: %v", err),
		}
	}
	if err := os.WriteFile(fullPath, []byte(doc.Content), 0644); err != nil {
		return UpgradeResult{
			File:   relPath,
			Status: "error",
			Error:  fmt.Sprintf("write file: %v", err),
		}
	}

	return UpgradeResult{
		File:   relPath,
		Status: "upgraded",
		backupFile: backupFile,
	}
}

// backupOriginal 将原文件重命名为 .md.bak
// 如果 .md.bak 已存在，追加数字后缀 .md.bak.1、.md.bak.2 等
func backupOriginal(filePath string) (string, error) {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// 文件不存在，无需备份
		return "", nil
	}

	backupPath := filePath + ".bak"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		// .md.bak 不存在，直接重命名
		if err := os.Rename(filePath, backupPath); err != nil {
			return "", fmt.Errorf("rename to .bak: %w", err)
		}
		return filepath.Base(backupPath), nil
	}

	// .md.bak 已存在，找下一个可用的数字后缀
	for i := 1; i <= 100; i++ {
		numberedBackup := fmt.Sprintf("%s.bak.%d", filePath, i)
		if _, err := os.Stat(numberedBackup); os.IsNotExist(err) {
			if err := os.Rename(filePath, numberedBackup); err != nil {
				return "", fmt.Errorf("rename to .bak.%d: %w", i, err)
			}
			return filepath.Base(numberedBackup), nil
		}
	}

	return "", fmt.Errorf("too many backup files for %s", filePath)
}

// prependFrontMatter 为文档添加 Front Matter
// 如果已有 Front Matter 但缺少字段，则补齐；否则新增
func prependFrontMatter(content string, meta *ExtractedMetadata) string {
	fm, body := extractFrontMatter(content)

	// 确保 body 部分不以 Front Matter 标记开头
	body = strings.TrimPrefix(body, "---")
	body = strings.TrimLeft(body, "\n\r")

	// 构建新的 Front Matter
	if fm == nil {
		fm = make(map[string]string)
	}
	if fm["service"] == "" {
		fm["service"] = meta.Service
	}
	if fm["module"] == "" {
		if meta.Module != "" {
			fm["module"] = meta.Module
		} else {
			fm["module"] = "默认模块"
		}
	}
	if fm["type"] == "" {
		// 尝试推断类型
		if strings.Contains(body, "## 问题现象") || strings.Contains(body, "## 根本原因") {
			fm["type"] = "archive"
		} else {
			fm["type"] = "sop"
		}
	}

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("service: %s\n", fm["service"]))
	sb.WriteString(fmt.Sprintf("module: %s\n", fm["module"]))
	sb.WriteString(fmt.Sprintf("type: %s\n", fm["type"]))
	sb.WriteString("---\n\n")

	// 如果原文档没有关键词段，且 LLM 提取到了，则补充
	if !strings.Contains(body, "## 关键词") && len(meta.Keywords) > 0 {
		// 在第一个 ## 之前插入关键词段
		idx := strings.Index(body, "## ")
		if idx > 0 {
			sb.WriteString(body[:idx])
			sb.WriteString(fmt.Sprintf("## 关键词\n\n%s\n\n", strings.Join(meta.Keywords, ", ")))
			sb.WriteString(body[idx:])
		} else {
			sb.WriteString(body)
			sb.WriteString(fmt.Sprintf("\n## 关键词\n\n%s\n", strings.Join(meta.Keywords, ", ")))
		}
	} else {
		sb.WriteString(body)
	}

	return sb.String()
}

// loadUpgradeState 加载已整理状态
func loadUpgradeState(dir string) *UpgradeState {
	path := filepath.Join(dir, upgradeStateFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return &UpgradeState{}
	}
	var state UpgradeState
	if err := json.Unmarshal(data, &state); err != nil {
		return &UpgradeState{}
	}
	return &state
}

// saveUpgradeState 持久化整理状态
func saveUpgradeState(dir string, state *UpgradeState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, upgradeStateFile)
	return os.WriteFile(path, data, 0644)
}
