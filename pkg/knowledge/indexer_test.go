package knowledge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractFrontMatter(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantFields map[string]string
		wantBody   string
	}{
		{
			name: "标准 Front Matter",
			input: `---
service: Payment Service
module: 核心支付模块
---

# 正文内容`,
			wantFields: map[string]string{
				"service": "Payment Service",
				"module":  "核心支付模块",
			},
			wantBody: "# 正文内容",
		},
		{
			name: "无 Front Matter",
			input: `# 标题

正文内容`,
			wantFields: nil,
			wantBody:   "# 标题\n\n正文内容",
		},
		{
			name: "Front Matter 带引号",
			input: `---
service: "My Service"
---

正文`,
			wantFields: map[string]string{
				"service": "My Service",
			},
			wantBody: "正文",
		},
		{
			name: "只有开头没有结尾的 Front Matter",
			input: `---
service: Test

没有闭合的正文`,
			wantFields: nil,
			wantBody: "---\nservice: Test\n\n没有闭合的正文",
		},
		{
			name: "代码块中的 --- 不应被误识别为 Front Matter 结束",
			input: `---
service: Test
type: sop
---

# 正文

代码示例：

` + "```yaml" + `
---
apiVersion: v1
kind: ConfigMap
---
` + "```" + `

更多内容`,
			wantFields: map[string]string{
				"service": "Test",
				"type":    "sop",
			},
			wantBody: "# 正文\n\n代码示例：\n\n```yaml\n---\napiVersion: v1\nkind: ConfigMap\n---\n```\n\n更多内容",
		},
		{
			name: "Front Matter 中含 --- 行的代码块",
			input: `---
service: Test
---

正文包含代码

` + "```" + `
一些内容
---
不应该截断
` + "```" + `

尾部内容`,
			wantFields: map[string]string{
				"service": "Test",
			},
			wantBody: "正文包含代码\n\n```\n一些内容\n---\n不应该截断\n```\n\n尾部内容",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields, body := extractFrontMatter(tt.input)

			if tt.wantFields == nil {
				if fields != nil {
					t.Errorf("expected nil fields, got %v", fields)
				}
			} else {
				if fields == nil {
					t.Errorf("expected fields %v, got nil", tt.wantFields)
				} else {
					for k, v := range tt.wantFields {
						if fields[k] != v {
							t.Errorf("fields[%q] = %q, want %q", k, fields[k], v)
						}
					}
				}
			}

			if strings.TrimSpace(body) != strings.TrimSpace(tt.wantBody) {
				t.Errorf("body = %q, want %q", strings.TrimSpace(body), strings.TrimSpace(tt.wantBody))
			}
		})
	}
}

func TestExtractSOPScenarios(t *testing.T) {
	content := `# 支付系统排查手册

## 场景：API 接口超时 (504 Gateway Timeout)

- **所属模块**: 核心支付模块
- **现象**: 前端提示请求超时，Nginx 日志出现大量 504
- **关键词**: 504, timeout, 超时, 网关超时, 接口超时, nginx
- **涉及组件**: Nginx, Core Service, MySQL, Redis

**可能原因**:
1. Core Service 负载过高

## 场景：订单状态未流转

- **现象**: 用户支付成功，但订单状态仍为 "PENDING"
- **关键词**: PENDING, 状态未更新, 回调失败, 订单, 未流转
- **涉及组件**: callback-worker, Redis, Core Service

**排查步骤**: ...`

	entries := extractSOPScenarios(content, "payment.md")

	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}

	// 第一个场景
	e1 := entries[0]
	if e1.Title != "API 接口超时 (504 Gateway Timeout)" {
		t.Errorf("entry[0].Title = %q", e1.Title)
	}
	if e1.Phenomena != "前端提示请求超时，Nginx 日志出现大量 504" {
		t.Errorf("entry[0].Phenomena = %q", e1.Phenomena)
	}
	if len(e1.Keywords) != 6 {
		t.Errorf("entry[0].Keywords len = %d, want 6", len(e1.Keywords))
	}
	if e1.LineStart < 1 {
		t.Errorf("entry[0].LineStart = %d, want >= 1", e1.LineStart)
	}
	if e1.Type != "sop" {
		t.Errorf("entry[0].Type = %q, want 'sop'", e1.Type)
	}
	if e1.File != "payment.md" {
		t.Errorf("entry[0].File = %q, want 'payment.md'", e1.File)
	}

	// 第二个场景
	e2 := entries[1]
	if e2.Title != "订单状态未流转" {
		t.Errorf("entry[1].Title = %q", e2.Title)
	}
	if e2.Phenomena != "用户支付成功，但订单状态仍为 \"PENDING\"" {
		t.Errorf("entry[1].Phenomena = %q", e2.Phenomena)
	}
	if len(e2.Keywords) != 5 {
		t.Errorf("entry[1].Keywords = %v, want len 5", e2.Keywords)
	}
	// LineEnd 应该大于 LineStart
	if e2.LineEnd <= e2.LineStart {
		t.Errorf("entry[1].LineEnd(%d) <= LineStart(%d)", e2.LineEnd, e2.LineStart)
	}
}

func TestExtractSOPScenariosNoMatch(t *testing.T) {
	content := `# 普通文档

这里没有场景标题。

只是一些普通内容。`

	entries := extractSOPScenarios(content, "normal.md")
	if entries != nil {
		t.Errorf("expected nil for non-scenario document, got %d entries", len(entries))
	}
}

func TestExtractArchiveScenarios(t *testing.T) {
	content := `---
service: auto
module: auto
type: archive
---

# 支付接口超时

## 概述

一些概述信息

## 问题现象

前端调用支付接口返回504超时，Nginx日志显示upstream timeout

## 关键词

504, timeout, 支付超时, nginx

## 涉及组件

Nginx, Core Service, MySQL

## 根本原因

MySQL慢查询导致支付服务线程池耗尽

## 解决方案

优化慢查询SQL

---

*会话ID: abc123*`

	entries := extractArchiveScenarios(content, "troubleshooting/test.md")

	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}

	e := entries[0]
	if e.Title != "支付接口超时" {
		t.Errorf("Title = %q, want '支付接口超时'", e.Title)
	}
	if !strings.Contains(e.Phenomena, "504") {
		t.Errorf("Phenomena = %q, should contain '504'", e.Phenomena)
	}
	if len(e.Keywords) != 4 {
		t.Errorf("Keywords = %v, want len 4", e.Keywords)
	}
	if e.Type != "archive" {
		t.Errorf("Type = %q, want 'archive'", e.Type)
	}
}

func TestExtractArchiveScenariosNoPhenomena(t *testing.T) {
	content := `# 无现象的归档

## 根本原因

不知道

## 解决方案

重启试试`

	entries := extractArchiveScenarios(content, "test.md")
	if entries != nil {
		t.Errorf("expected nil for archive without phenomena, got %d entries", len(entries))
	}
}

func TestParseDocumentWithFrontMatter(t *testing.T) {
	content := `---
service: Payment Service
module: 核心支付模块
---

# 支付系统排查手册

## 场景：API 接口超时

- **现象**: 超时了
- **关键词**: timeout, 超时
- **涉及组件**: Nginx`

	svc, mod, entries := parseDocument("payment.md", content)

	if svc != "Payment Service" {
		t.Errorf("service = %q, want 'Payment Service'", svc)
	}
	if mod != "核心支付模块" {
		t.Errorf("module = %q, want '核心支付模块'", mod)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	if entries[0].Title != "API 接口超时" {
		t.Errorf("entry.Title = %q", entries[0].Title)
	}
}

func TestParseDocumentWithoutFrontMatter(t *testing.T) {
	content := `# 网络排查手册

## 场景：无法连接公网

- **现象**: ping 8.8.8.8 超时
- **关键词**: ping, 公网`

	svc, _, entries := parseDocument("network_troubleshooting.md", content)

	// 没有 front matter 且没有 service 信息 → 应该跳过
	if svc != "" {
		t.Errorf("service should be empty for docs without front matter, got %q", svc)
	}
	if len(entries) != 0 {
		t.Errorf("entries should be empty for docs without front matter, got %d", len(entries))
	}
}

func TestMd5Hash(t *testing.T) {
	h1 := md5Hash("hello")
	h2 := md5Hash("hello")
	h3 := md5Hash("world")

	if h1 != h2 {
		t.Error("same content should produce same hash")
	}
	if h1 == h3 {
		t.Error("different content should produce different hash")
	}
}

func TestSplitCommaList(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"504, timeout, 超时", 3},
		{"", 0},
		{"single", 1},
		{"a, b, c, d, e", 5},
	}

	for _, tt := range tests {
		result := splitCommaList(tt.input)
		if len(result) != tt.want {
			t.Errorf("splitCommaList(%q) = %d items, want %d", tt.input, len(result), tt.want)
		}
	}
}

func TestBuildCatalogFromTestdata(t *testing.T) {
	testdataDir := filepath.Join("testdata")

	cat, err := BuildCatalog(testdataDir)
	if err != nil {
		t.Fatalf("BuildCatalog error: %v", err)
	}

	if cat.Version != 1 {
		t.Errorf("Version = %d, want 1", cat.Version)
	}
	if cat.BuildAt.IsZero() {
		t.Error("BuildAt should not be zero")
	}

	total := cat.TotalScenarios()
	if total == 0 {
		t.Error("TotalScenarios should be > 0 for testdata")
	}

	// 验证 catalog.json 被创建
	catPath := filepath.Join(testdataDir, ".catalog.json")
	if _, err := os.Stat(catPath); os.IsNotExist(err) {
		t.Error(".catalog.json should have been created")
	}

	// 验证 Payment Service 存在
	foundPayment := false
	for _, svc := range cat.Services {
		if svc.Name == "Payment Service" {
			foundPayment = true
			break
		}
	}
	if !foundPayment {
		t.Errorf("expected 'Payment Service' in catalog, got services: %v", serviceNames(cat))
	}

	// 验证 Network Service 存在
	foundNetwork := false
	for _, svc := range cat.Services {
		if svc.Name == "Network Service" {
			foundNetwork = true
			break
		}
	}
	if !foundNetwork {
		t.Errorf("expected 'Network Service' in catalog, got services: %v", serviceNames(cat))
	}

	// 验证归档文件也被索引
	totalScenarios := cat.TotalScenarios()
	if totalScenarios < 3 {
		t.Errorf("expected at least 3 scenarios (2 from SOP + 1 from archive), got %d", totalScenarios)
	}

	// 验证 FindEntry 可以工作
	entry := cat.FindEntry("payment_sop.md", "API 接口超时")
	if entry == nil {
		t.Error("FindEntry should find 'API 接口超时' in payment_sop.md")
	}

	// 验证 RenderForLLM 输出
	rendered := cat.RenderForLLM()
	if rendered == "" {
		t.Error("RenderForLLM should not be empty")
	}
	if !strings.Contains(rendered, "Payment Service") {
		t.Error("RenderForLLM should contain 'Payment Service'")
	}

	// 清理
	os.Remove(catPath)
}

func TestBuildCatalogIncremental(t *testing.T) {
	testdataDir := filepath.Join("testdata")

	// 第一次构建
	cat1, err := BuildCatalog(testdataDir)
	if err != nil {
		t.Fatalf("first BuildCatalog error: %v", err)
	}

	// 第二次构建（文件未变更）
	cat2, err := BuildCatalog(testdataDir)
	if err != nil {
		t.Fatalf("second BuildCatalog error: %v", err)
	}

	// 场景数量应该一致
	if cat1.TotalScenarios() != cat2.TotalScenarios() {
		t.Errorf("incremental build changed scenario count: %d → %d",
			cat1.TotalScenarios(), cat2.TotalScenarios())
	}

	// FileHash 应该一致
	if len(cat1.FileHash) != len(cat2.FileHash) {
		t.Errorf("FileHash count changed: %d → %d", len(cat1.FileHash), len(cat2.FileHash))
	}

	// 清理
	os.Remove(filepath.Join(testdataDir, ".catalog.json"))
}

func TestBuildCatalogEmpty(t *testing.T) {
	// 创建临时空目录
	tmpDir, err := os.MkdirTemp("", "catalog_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cat, err := BuildCatalog(tmpDir)
	if err != nil {
		t.Fatalf("BuildCatalog on empty dir error: %v", err)
	}

	if cat.TotalScenarios() != 0 {
		t.Errorf("empty dir TotalScenarios = %d, want 0", cat.TotalScenarios())
	}
}

func TestBuildCatalogNonexistent(t *testing.T) {
	_, err := BuildCatalog("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

func TestInferServiceFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"payment_system_sop.md", "Payment System"},
		{"network_troubleshooting.md", "Network"},
		{"database_maintenance.md", "Database"},
		{"my_app.md", "My App"},
	}

	for _, tt := range tests {
		got := inferServiceFromPath(tt.path)
		if got != tt.want {
			t.Errorf("inferServiceFromPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// helper functions

func serviceNames(cat *Catalog) []string {
	names := make([]string, len(cat.Services))
	for i, s := range cat.Services {
		names[i] = s.Name
	}
	return names
}
