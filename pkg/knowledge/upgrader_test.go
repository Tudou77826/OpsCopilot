package knowledge

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"opscopilot/pkg/llm"
)

// mockReorganizer 实现 ContentReorganizer 接口，用于测试
type mockReorganizer struct {
	response  []*ReorganizedDocument
	err       error
	callCount int
}

func (m *mockReorganizer) Reorganize(ctx context.Context, content string) ([]*ReorganizedDocument, error) {
	m.callCount++
	return m.response, m.err
}

const testSOPContent = `---
service: Test Service
module: 测试模块
type: sop
---

# 测试手册

## 场景：测试场景

- **现象**: 测试现象
- **关键词**: test
- **涉及组件**: TestComponent

<!-- META: {"resend_from_line": null} -->`

func TestUpgradeDocuments_BasicFlow(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "upgrade_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建一个不规范文档
	originalContent := "一些松散的运维文档\n没有格式\nAPI会超时"
	if err := os.WriteFile(filepath.Join(tmpDir, "test.md"), []byte(originalContent), 0644); err != nil {
		t.Fatal(err)
	}

	reorganizer := &mockReorganizer{
		response: []*ReorganizedDocument{{
			Content: testSOPContent,
			Service: "Test Service",
			Module:  "测试模块",
			DocType: "sop",
		}},
	}

	results, err := UpgradeDocuments(context.Background(), tmpDir, reorganizer, nil)
	if err != nil {
		t.Fatalf("UpgradeDocuments error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Status != "upgraded" {
		t.Errorf("status = %q, want 'upgraded'", results[0].Status)
	}

	// 验证旧文件改名为 .md.bak
	bakPath := filepath.Join(tmpDir, "test.md.bak")
	if _, err := os.Stat(bakPath); os.IsNotExist(err) {
		t.Error("backup file .md.bak should exist")
	}

	// 验证新文件包含重组内容
	newContent, err := os.ReadFile(filepath.Join(tmpDir, "test.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(newContent), "## 场景：") {
		t.Error("new file should contain standard SOP format")
	}
	if !strings.Contains(string(newContent), "---") {
		t.Error("new file should contain Front Matter")
	}
}

func TestUpgradeDocuments_IncrementalSkip(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "upgrade_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	if err := os.WriteFile(filepath.Join(tmpDir, "test.md"), []byte("原始内容"), 0644); err != nil {
		t.Fatal(err)
	}

	reorganizer := &mockReorganizer{
		response: []*ReorganizedDocument{{
			Content: testSOPContent,
			Service: "Test Service",
			DocType: "sop",
		}},
	}

	// 第一次运行
	results1, err := UpgradeDocuments(context.Background(), tmpDir, reorganizer, nil)
	if err != nil {
		t.Fatalf("first run error: %v", err)
	}
	if len(results1) != 1 || results1[0].Status != "upgraded" {
		t.Fatalf("first run: expected 1 upgraded, got %v", results1)
	}

	// 第二次运行（文件内容未变，hash 匹配）
	reorganizer.callCount = 0
	results2, err := UpgradeDocuments(context.Background(), tmpDir, reorganizer, nil)
	if err != nil {
		t.Fatalf("second run error: %v", err)
	}
	if len(results2) != 1 || results2[0].Status != "skipped" {
		t.Fatalf("second run: expected 1 skipped, got %v", results2)
	}
	if reorganizer.callCount != 0 {
		t.Errorf("reorganizer should not be called on skip, called %d times", reorganizer.callCount)
	}
}

func TestUpgradeDocuments_BackupConflict(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "upgrade_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建文档和已有的 .md.bak 文件
	if err := os.WriteFile(filepath.Join(tmpDir, "test.md"), []byte("新内容v2"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "test.md.bak"), []byte("旧备份"), 0644); err != nil {
		t.Fatal(err)
	}

	reorganizer := &mockReorganizer{
		response: []*ReorganizedDocument{{
			Content: testSOPContent,
			Service: "Test Service",
			DocType: "sop",
		}},
	}

	results, err := UpgradeDocuments(context.Background(), tmpDir, reorganizer, nil)
	if err != nil {
		t.Fatalf("UpgradeDocuments error: %v", err)
	}
	if len(results) != 1 || results[0].Status != "upgraded" {
		t.Fatalf("expected 1 upgraded, got %v", results)
	}

	// 验证 .md.bak.1 被创建
	bak1Path := filepath.Join(tmpDir, "test.md.bak.1")
	if _, err := os.Stat(bak1Path); os.IsNotExist(err) {
		t.Error("backup file .md.bak.1 should exist when .md.bak already exists")
	}

	// 原来的 .md.bak 应该还在
	if _, err := os.Stat(filepath.Join(tmpDir, "test.md.bak")); os.IsNotExist(err) {
		t.Error("original .md.bak should still exist")
	}
}

func TestUpgradeDocuments_MultipleFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "upgrade_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建 3 个文件
	for i := 1; i <= 3; i++ {
		content := fmt.Sprintf("文档内容 %d", i)
		if err := os.WriteFile(filepath.Join(tmpDir, fmt.Sprintf("doc%d.md", i)), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	reorganizer := &mockReorganizer{
		response: []*ReorganizedDocument{{
			Content: testSOPContent,
			Service: "Test Service",
			DocType: "sop",
		}},
	}

	results, err := UpgradeDocuments(context.Background(), tmpDir, reorganizer, nil)
	if err != nil {
		t.Fatalf("UpgradeDocuments error: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}

	upgraded := 0
	for _, r := range results {
		if r.Status == "upgraded" {
			upgraded++
		}
	}
	if upgraded != 3 {
		t.Errorf("upgraded = %d, want 3", upgraded)
	}

	// 验证每个文件都有备份
	for i := 1; i <= 3; i++ {
		bakPath := filepath.Join(tmpDir, fmt.Sprintf("doc%d.md.bak", i))
		if _, err := os.Stat(bakPath); os.IsNotExist(err) {
			t.Errorf("doc%d.md.bak should exist", i)
		}
	}
}

func TestUpgradeDocuments_EmptyDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "upgrade_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	reorganizer := &mockReorganizer{}

	results, err := UpgradeDocuments(context.Background(), tmpDir, reorganizer, nil)
	if err != nil {
		t.Fatalf("UpgradeDocuments error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results for empty dir, got %v", results)
	}
}

func TestUpgradeDocuments_StatePersistence(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "upgrade_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	if err := os.WriteFile(filepath.Join(tmpDir, "test.md"), []byte("内容"), 0644); err != nil {
		t.Fatal(err)
	}

	reorganizer := &mockReorganizer{
		response: []*ReorganizedDocument{{
			Content: testSOPContent,
			Service: "Test Service",
			DocType: "sop",
		}},
	}

	_, err = UpgradeDocuments(context.Background(), tmpDir, reorganizer, nil)
	if err != nil {
		t.Fatal(err)
	}

	// 检查 .upgrade_state.json 存在
	statePath := filepath.Join(tmpDir, ".upgrade_state.json")
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Fatal(".upgrade_state.json should exist")
	}

	// 加载并验证状态
	state := loadUpgradeState(tmpDir)
	if len(state.Files) != 1 {
		t.Fatalf("state.Files len = %d, want 1", len(state.Files))
	}
	fileState, ok := state.Files["test.md"]
	if !ok {
		t.Fatal("state should contain 'test.md'")
	}
	if fileState.Hash == "" {
		t.Error("hash should not be empty")
	}
	if fileState.UpgradedAt.IsZero() {
		t.Error("upgradedAt should not be zero")
	}
	if fileState.BackupFile == "" {
		t.Error("backupFile should not be empty")
	}
}

func TestUpgradeDocuments_ProgressCallback(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "upgrade_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	if err := os.WriteFile(filepath.Join(tmpDir, "doc1.md"), []byte("内容1"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "doc2.md"), []byte("内容2"), 0644); err != nil {
		t.Fatal(err)
	}

	reorganizer := &mockReorganizer{
		response: []*ReorganizedDocument{{
			Content: testSOPContent,
			Service: "Test Service",
			DocType: "sop",
		}},
	}

	var stages []string
	onProgress := func(stage string, current, total int, file, message string) {
		stages = append(stages, stage)
	}

	_, err = UpgradeDocuments(context.Background(), tmpDir, reorganizer, onProgress)
	if err != nil {
		t.Fatal(err)
	}

	// 应该至少有 scanning 和 processing 阶段
	hasScanning := false
	hasProcessing := false
	for _, s := range stages {
		if s == "scanning" {
			hasScanning = true
		}
		if s == "processing" {
			hasProcessing = true
		}
	}
	if !hasScanning {
		t.Error("should have 'scanning' stage")
	}
	if !hasProcessing {
		t.Error("should have 'processing' stage")
	}
}

func TestUpgradeDocuments_LLMFailure(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "upgrade_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	if err := os.WriteFile(filepath.Join(tmpDir, "fail.md"), []byte("内容"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "ok.md"), []byte("内容"), 0644); err != nil {
		t.Fatal(err)
	}

	reorganizer := &mockReorganizer{
		err: fmt.Errorf("LLM unavailable"),
	}

	results, err := UpgradeDocuments(context.Background(), tmpDir, reorganizer, nil)
	if err != nil {
		// UpgradeDocuments itself shouldn't error, individual file errors are in results
		t.Fatalf("UpgradeDocuments should not return error for individual file failures: %v", err)
	}

	// Both should fail since mock always returns error
	for _, r := range results {
		if r.Status != "error" {
			t.Errorf("expected status 'error', got %q", r.Status)
		}
	}
}

func TestBackupOriginal(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "backup_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	t.Run("file exists", func(t *testing.T) {
		filePath := filepath.Join(tmpDir, "doc.md")
		os.WriteFile(filePath, []byte("original"), 0644)

		backupName, err := backupOriginal(filePath)
		if err != nil {
			t.Fatal(err)
		}
		if backupName != "doc.md.bak" {
			t.Errorf("backupName = %q, want 'doc.md.bak'", backupName)
		}
		// 原文件应该不存在了
		if _, err := os.Stat(filePath); !os.IsNotExist(err) {
			t.Error("original file should have been renamed")
		}
		// 备份文件应该存在
		if _, err := os.Stat(filePath + ".bak"); os.IsNotExist(err) {
			t.Error("backup file should exist")
		}
	})

	t.Run("file does not exist", func(t *testing.T) {
		backupName, err := backupOriginal(filepath.Join(tmpDir, "nonexistent.md"))
		if err != nil {
			t.Fatal(err)
		}
		if backupName != "" {
			t.Errorf("backupName should be empty for nonexistent file, got %q", backupName)
		}
	})

	t.Run("backup already exists", func(t *testing.T) {
		filePath := filepath.Join(tmpDir, "doc2.md")
		os.WriteFile(filePath, []byte("new content"), 0644)
		os.WriteFile(filePath+".bak", []byte("old backup"), 0644)

		backupName, err := backupOriginal(filePath)
		if err != nil {
			t.Fatal(err)
		}
		if backupName != "doc2.md.bak.1" {
			t.Errorf("backupName = %q, want 'doc2.md.bak.1'", backupName)
		}
		// .md.bak.1 应该存在
		if _, err := os.Stat(filePath + ".bak.1"); os.IsNotExist(err) {
			t.Error(".md.bak.1 should exist")
		}
		// 原来的 .md.bak 应该还在
		if _, err := os.Stat(filePath + ".bak"); os.IsNotExist(err) {
			t.Error("original .md.bak should still exist")
		}
	})
}

func TestBakFilesExcludedFromWalk(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "walk_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建 .md、.md.bak、.md.bak.1 文件
	os.WriteFile(filepath.Join(tmpDir, "doc.md"), []byte("normal"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "doc.md.bak"), []byte("backup"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "doc.md.bak.1"), []byte("backup1"), 0644)

	files, err := walkMarkdownFiles(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(files) != 1 {
		t.Fatalf("len(files) = %d, want 1 (only .md, not .bak)", len(files))
	}
	if _, ok := files["doc.md"]; !ok {
		t.Error("should find doc.md")
	}
}

func TestBakFilesExcludedFromCatalog(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "catalog_bak_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建一个规范文档
	content := `---
service: Test Service
module: Test Module
---

# Test

## 场景：Test Scenario

- **现象**: test phenomenon
- **关键词**: test
- **涉及组件**: TestComponent
`
	os.WriteFile(filepath.Join(tmpDir, "doc.md"), []byte(content), 0644)
	os.WriteFile(filepath.Join(tmpDir, "doc.md.bak"), []byte("backup content"), 0644)

	cat, err := BuildCatalog(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// catalog 不应该包含 .bak 文件的条目
	if len(cat.FileHash) != 1 {
		t.Errorf("FileHash len = %d, want 1", len(cat.FileHash))
	}
	if _, ok := cat.FileHash["doc.md.bak"]; ok {
		t.Error("doc.md.bak should not be in catalog FileHash")
	}
}

func TestUpgradeThenBuildCatalog(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "upgrade_catalog_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建不规范文档
	os.WriteFile(filepath.Join(tmpDir, "messy.md"), []byte("一些散乱的运维笔记"), 0644)

	reorganizedContent := `---
service: MyService
module: MyModule
type: sop
---

# MyService 排查手册

## 场景：接口超时

- **现象**: 请求超时
- **关键词**: timeout, 504
- **涉及组件**: Nginx

**可能原因**:
1. 负载过高

**排查步骤**:
1. 检查日志
`

	reorganizer := &mockReorganizer{
		response: []*ReorganizedDocument{{
			Content: reorganizedContent,
			Service: "MyService",
			Module:  "MyModule",
			DocType: "sop",
		}},
	}

	results, err := UpgradeDocuments(context.Background(), tmpDir, reorganizer, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Status != "upgraded" {
		t.Fatalf("expected 1 upgraded, got %v", results)
	}

	// 重建目录
	cat, err := BuildCatalog(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	total := cat.TotalScenarios()
	if total == 0 {
		t.Error("catalog should have scenarios after upgrade")
	}

	// 验证有对应的 SOP 条目
	entry := cat.FindEntry("messy.md", "接口超时")
	if entry == nil {
		t.Error("catalog should find '接口超时' scenario")
	}
}

func TestUpgradeDocuments_WithLLMMockProvider(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "upgrade_llm_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.WriteFile(filepath.Join(tmpDir, "doc.md"), []byte("散乱文档"), 0644)

	mockProvider := &llm.MockProvider{
		Response: `---
service: MockService
module: MockModule
type: archive
---

# Mock 问题

## 问题现象

mock现象

## 关键词

mock, test

## 根本原因

mock原因

## 解决方案

mock方案

<!-- META: {"resend_from_line": null} -->`,
	}

	reorganizer := NewLLMContentReorganizer(mockProvider, nil)
	results, err := UpgradeDocuments(context.Background(), tmpDir, reorganizer, nil)
	if err != nil {
		t.Fatalf("UpgradeDocuments error: %v", err)
	}

	if len(results) != 1 || results[0].Status != "upgraded" {
		t.Fatalf("expected 1 upgraded, got %v", results)
	}

	// 验证备份
	if _, err := os.Stat(filepath.Join(tmpDir, "doc.md.bak")); os.IsNotExist(err) {
		t.Error("backup should exist")
	}

	// 验证新文件
	content, err := os.ReadFile(filepath.Join(tmpDir, "doc.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "## 问题现象") {
		t.Error("new file should contain archive format")
	}
}

func TestRollbackBackup(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "rollback_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	t.Run("rollback restores original file", func(t *testing.T) {
		original := filepath.Join(tmpDir, "test.md")
		backup := filepath.Join(tmpDir, "test.md.bak")
		originalContent := "原始内容"

		os.WriteFile(original, []byte(originalContent), 0644)
		os.Rename(original, backup)

		// 原文件已不存在
		if _, err := os.Stat(original); !os.IsNotExist(err) {
			t.Fatal("original should not exist after rename")
		}

		// 执行回滚
		rollbackBackup(original, "test.md.bak")

		// 原文件应该恢复
		data, err := os.ReadFile(original)
		if err != nil {
			t.Fatalf("original file should be restored: %v", err)
		}
		if string(data) != originalContent {
			t.Errorf("restored content = %q, want %q", string(data), originalContent)
		}
		// 备份文件应该不存在了
		if _, err := os.Stat(backup); !os.IsNotExist(err) {
			t.Error("backup file should be removed after rollback")
		}
	})

	t.Run("rollback with empty backup name is no-op", func(t *testing.T) {
		original := filepath.Join(tmpDir, "noop.md")
		os.WriteFile(original, []byte("content"), 0644)

		rollbackBackup(original, "")

		data, err := os.ReadFile(original)
		if err != nil || string(data) != "content" {
			t.Error("file should be unchanged with empty backup name")
		}
	})
}

func TestUpgradeDocuments_SplitFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "upgrade_split_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建一个模拟多模块文档
	multiContent := "## 故障记录 [2024-03-15] 支付超时\n支付接口504\n\n## 故障记录 [2024-03-20] 订单超时\n订单创建失败"
	if err := os.WriteFile(filepath.Join(tmpDir, "troubleshooting_history.md"), []byte(multiContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Mock 返回两个拆分的文档
	reorganizer := &mockReorganizer{
		response: []*ReorganizedDocument{
			{
				Content: "---\nservice: Payment Service\nmodule: 核心支付模块\ntype: archive\n---\n\n# 支付超时\n",
				Service: "Payment Service",
				Module:  "核心支付模块",
				DocType: "archive",
				SubPath: "核心支付模块",
			},
			{
				Content: "---\nservice: Order Service\nmodule: 订单模块\ntype: archive\n---\n\n# 订单超时\n",
				Service: "Order Service",
				Module:  "订单模块",
				DocType: "archive",
				SubPath: "订单模块",
			},
		},
	}

	results, err := UpgradeDocuments(context.Background(), tmpDir, reorganizer, nil)
	if err != nil {
		t.Fatalf("UpgradeDocuments error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Status != "upgraded" {
		t.Errorf("status = %q, want 'upgraded'", results[0].Status)
	}
	if len(results[0].Outputs) != 2 {
		t.Errorf("len(Outputs) = %d, want 2", len(results[0].Outputs))
	}

	// 验证备份文件
	if _, err := os.Stat(filepath.Join(tmpDir, "troubleshooting_history.md.bak")); os.IsNotExist(err) {
		t.Error("backup file should exist")
	}

	// 验证拆分输出文件
	paymentPath := filepath.Join(tmpDir, "核心支付模块", "troubleshooting_history.md")
	if _, err := os.Stat(paymentPath); os.IsNotExist(err) {
		t.Error("核心支付模块/troubleshooting_history.md should exist")
	}
	orderPath := filepath.Join(tmpDir, "订单模块", "troubleshooting_history.md")
	if _, err := os.Stat(orderPath); os.IsNotExist(err) {
		t.Error("订单模块/troubleshooting_history.md should exist")
	}
}

func TestUpgradeDocuments_SplitThenSkip(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "upgrade_split_skip_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	content := "原始内容"
	if err := os.WriteFile(filepath.Join(tmpDir, "test.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	reorganizer := &mockReorganizer{
		response: []*ReorganizedDocument{{
			Content: testSOPContent,
			Service: "Test Service",
			Module:  "测试模块",
			DocType: "sop",
		}},
	}

	// 第一次运行
	results1, err := UpgradeDocuments(context.Background(), tmpDir, reorganizer, nil)
	if err != nil {
		t.Fatalf("first run error: %v", err)
	}
	if len(results1) != 1 || results1[0].Status != "upgraded" {
		t.Fatalf("first run: expected 1 upgraded, got %v", results1)
	}

	// 第二次运行（文件内容未变）
	reorganizer.callCount = 0
	results2, err := UpgradeDocuments(context.Background(), tmpDir, reorganizer, nil)
	if err != nil {
		t.Fatalf("second run error: %v", err)
	}
	if len(results2) != 1 || results2[0].Status != "skipped" {
		t.Fatalf("second run: expected 1 skipped, got %v", results2)
	}
}

func TestRollbackSplit(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "rollback_split_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建一些模拟的拆分输出文件
	dir1 := filepath.Join(tmpDir, "模块A")
	os.MkdirAll(dir1, 0755)
	file1 := filepath.Join(dir1, "test.md")
	os.WriteFile(file1, []byte("content A"), 0644)

	dir2 := filepath.Join(tmpDir, "模块B")
	os.MkdirAll(dir2, 0755)
	file2 := filepath.Join(dir2, "test.md")
	os.WriteFile(file2, []byte("content B"), 0644)

	rollbackSplit(tmpDir, []string{file1, file2})

	if _, err := os.Stat(file1); !os.IsNotExist(err) {
		t.Error("file1 should be removed after rollbackSplit")
	}
	if _, err := os.Stat(file2); !os.IsNotExist(err) {
		t.Error("file2 should be removed after rollbackSplit")
	}
}
