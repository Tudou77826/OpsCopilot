package knowledge

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"opscopilot/pkg/knowledge/patchstore"
)

func TestRebuildFromPatches(t *testing.T) {
	dir := t.TempDir()

	patches := []patchstore.Patch{
		{
			ID:        "aaa",
			Service:   "Payment",
			Module:    "Core",
			Author:    "alice",
			Timestamp: time.Date(2026, 5, 16, 10, 0, 0, 0, time.UTC),
			Content:   "## 场景：支付超时\n\n504 超时排查",
		},
		{
			ID:        "bbb",
			Service:   "Payment",
			Module:    "Core",
			Author:    "bob",
			Timestamp: time.Date(2026, 5, 17, 14, 0, 0, 0, time.UTC),
			Content:   "## 场景：支付回调失败\n\n回调 500 错误",
		},
	}

	count, err := RebuildFromPatches(dir, patches)
	if err != nil {
		t.Fatalf("RebuildFromPatches error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 file, got %d", count)
	}

	fileName := sanitizeFileName("Payment", "Core") + ".md"
	data, err := os.ReadFile(filepath.Join(dir, "archive", fileName))
	if err != nil {
		t.Fatalf("read file error: %v", err)
	}

	content := string(data)

	// 验证 front matter
	if !containsStr(content, "service: \"Payment\"") {
		t.Error("missing service in front matter")
	}
	if !containsStr(content, "module: \"Core\"") {
		t.Error("missing module in front matter")
	}

	// 验证两个场景都存在，且按时间排序
	idx1 := indexOf(content, "支付超时")
	idx2 := indexOf(content, "支付回调失败")
	if idx1 < 0 || idx2 < 0 {
		t.Error("missing scenario content")
	}
	if idx1 >= idx2 {
		t.Error("scenarios not in chronological order")
	}
}

func TestRebuildFromPatchesEmpty(t *testing.T) {
	dir := t.TempDir()
	count, err := RebuildFromPatches(dir, nil)
	if err != nil {
		t.Fatalf("RebuildFromPatches error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 files, got %d", count)
	}
}

func TestRebuildFromPatchesMultipleServices(t *testing.T) {
	dir := t.TempDir()

	patches := []patchstore.Patch{
		{ID: "a", Service: "Payment", Module: "Core", Timestamp: time.Now(), Content: "场景1"},
		{ID: "b", Service: "Order", Module: "Process", Timestamp: time.Now(), Content: "场景2"},
	}

	count, err := RebuildFromPatches(dir, patches)
	if err != nil {
		t.Fatalf("RebuildFromPatches error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 files, got %d", count)
	}
}

func TestBuildServiceFile(t *testing.T) {
	patches := []patchstore.Patch{
		{
			ID:        "a",
			Service:   "Test",
			Module:    "Mod",
			Timestamp: time.Date(2026, 5, 17, 0, 0, 0, 0, time.UTC),
			Content:   "## 场景：测试\n\n内容",
		},
	}

	content := buildServiceFile("Test", "Mod", patches)

	if !containsStr(content, "# Test - Mod 运维文档") {
		t.Error("missing title")
	}
	if !containsStr(content, "| 微服务 | Test |") {
		t.Error("missing service info table")
	}
	if !containsStr(content, "## 场景：测试") {
		t.Error("missing scenario content")
	}
}

func TestGroupPatchesByServiceModule(t *testing.T) {
	patches := []patchstore.Patch{
		{ID: "a", Service: "S1", Module: "M1"},
		{ID: "b", Service: "S1", Module: "M1"},
		{ID: "c", Service: "S2", Module: "M1"},
	}

	groups := groupPatchesByServiceModule(patches)

	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if len(groups["S1|M1"]) != 2 {
		t.Errorf("S1|M1: expected 2, got %d", len(groups["S1|M1"]))
	}
	if len(groups["S2|M1"]) != 1 {
		t.Errorf("S2|M1: expected 1, got %d", len(groups["S2|M1"]))
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
