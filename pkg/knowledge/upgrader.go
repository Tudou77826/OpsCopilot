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
	Outputs    []string  `json:"outputs,omitempty"`    // 拆分输出文件路径
}

// UpgradeResult 单个文件的整理结果
type UpgradeResult struct {
	File    string   `json:"file"`
	Status  string   `json:"status"` // "upgraded" | "skipped" | "error"
	Error   string   `json:"error,omitempty"`
	Outputs []string `json:"outputs,omitempty"`

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
					Outputs:    result.Outputs,
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
	for p, fileState := range state.Files {
		if !existingPaths[p] {
			// 清理拆分输出的磁盘文件
			for _, output := range fileState.Outputs {
				outputPath := filepath.Join(knowledgeDir, filepath.FromSlash(output))
				os.Remove(outputPath)
			}
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
// 支持 1:N 拆分输出
func upgradeFile(ctx context.Context, knowledgeDir, relPath, content string, reorganizer ContentReorganizer) UpgradeResult {
	// 使用 LLM 重组文档（可能返回多个文档）
	docs, err := reorganizer.Reorganize(ctx, content)
	if err != nil {
		return UpgradeResult{
			File:   relPath,
			Status: "error",
			Error:  fmt.Sprintf("reorganize failed: %v", err),
		}
	}

	if len(docs) == 0 {
		return UpgradeResult{
			File:   relPath,
			Status: "error",
			Error:  "reorganize produced no documents",
		}
	}

	// 检查所有文档内容非空
	for i, doc := range docs {
		if doc.Content == "" {
			return UpgradeResult{
				File:   relPath,
				Status: "error",
				Error:  fmt.Sprintf("reorganize produced empty content for document %d", i),
			}
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

	// 记录已写入的路径，用于回滚
	var writtenPaths []string

	// 写入所有重组后的文档
	for _, doc := range docs {
		var targetPath string
		if doc.SubPath != "" {
			// 拆分模式：写入 knowledgeDir/SubPath
			// 消毒 SubPath，防止 LLM 生成的路径包含 ../ 导致目录遍历
			cleanSub := filepath.Clean(filepath.FromSlash(doc.SubPath))
			if strings.Contains(cleanSub, "..") {
				rollbackBackup(fullPath, backupFile)
				rollbackSplit(knowledgeDir, writtenPaths)
				return UpgradeResult{
					File:   relPath,
					Status: "error",
					Error:  fmt.Sprintf("invalid SubPath %q: path traversal detected", doc.SubPath),
				}
			}
			targetPath = filepath.Join(knowledgeDir, cleanSub, filepath.Base(relPath))
		} else {
			// 1:1 模式：写入原路径
			targetPath = fullPath
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			rollbackBackup(fullPath, backupFile)
			rollbackSplit(knowledgeDir, writtenPaths)
			return UpgradeResult{
				File:   relPath,
				Status: "error",
				Error:  fmt.Sprintf("create directory: %v", err),
			}
		}
		if err := os.WriteFile(targetPath, []byte(doc.Content), 0644); err != nil {
			rollbackBackup(fullPath, backupFile)
			rollbackSplit(knowledgeDir, writtenPaths)
			return UpgradeResult{
				File:   relPath,
				Status: "error",
				Error:  fmt.Sprintf("write file: %v", err),
			}
		}
		writtenPaths = append(writtenPaths, targetPath)
	}

	// 计算 outputs 路径（相对路径）
	var outputs []string
	for _, doc := range docs {
		if doc.SubPath != "" {
			outputs = append(outputs, filepath.ToSlash(filepath.Join(doc.SubPath, filepath.Base(relPath))))
		}
	}

	return UpgradeResult{
		File:       relPath,
		Status:     "upgraded",
		Outputs:    outputs,
		backupFile: backupFile,
	}
}

// rollbackBackup 将备份文件恢复为原文件
func rollbackBackup(originalPath, backupFileName string) {
	if backupFileName == "" {
		return
	}
	backupPath := filepath.Join(filepath.Dir(originalPath), backupFileName)
	if _, err := os.Stat(backupPath); err == nil {
		os.Rename(backupPath, originalPath)
	}
}

// rollbackSplit 清理拆分写入的文件
func rollbackSplit(knowledgeDir string, writtenPaths []string) {
	for _, p := range writtenPaths {
		os.Remove(p)
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
