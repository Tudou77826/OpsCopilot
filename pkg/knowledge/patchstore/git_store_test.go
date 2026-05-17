package patchstore

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func TestFormatAndParsePatchFile(t *testing.T) {
	store := &GitPatchStore{
		remoteURL:   "https://example.com/test.git",
		localDir:    t.TempDir(),
		branch:      "main",
		authorName:  "testuser",
		authorEmail: "test@example.com",
	}

	patch := Patch{
		ID:        "a1b2c3d4",
		Service:   "Payment Service",
		Module:    "核心支付模块",
		Author:    "zhangsan",
		Timestamp: time.Date(2026, 5, 17, 14, 30, 0, 0, time.Local),
		Content:   "## 场景：支付接口超时 - 2026-05-17 排查记录\n\n现象：504 超时",
	}

	content := store.formatPatchFile(patch)

	// 验证 front matter
	if want := "service: \"Payment Service\"\n"; !contains(content, want) {
		t.Errorf("missing service in front matter, got: %s", content)
	}
	if want := "module: \"核心支付模块\"\n"; !contains(content, want) {
		t.Errorf("missing module in front matter, got: %s", content)
	}
	if want := "patch_id: \"a1b2c3d4\"\n"; !contains(content, want) {
		t.Errorf("missing patch_id, got: %s", content)
	}
	if want := "timestamp: \"2026-05-17T14:30:00"; !contains(content, want) {
		t.Errorf("missing timestamp, got: %s", content)
	}

	// 解析回来
	parsed, err := parsePatchFile([]byte(content))
	if err != nil {
		t.Fatalf("parsePatchFile error: %v", err)
	}

	if parsed.ID != patch.ID {
		t.Errorf("ID: got %q, want %q", parsed.ID, patch.ID)
	}
	if parsed.Service != patch.Service {
		t.Errorf("Service: got %q, want %q", parsed.Service, patch.Service)
	}
	if parsed.Module != patch.Module {
		t.Errorf("Module: got %q, want %q", parsed.Module, patch.Module)
	}
	if parsed.Author != patch.Author {
		t.Errorf("Author: got %q, want %q", parsed.Author, patch.Author)
	}
	if !parsed.Timestamp.Equal(patch.Timestamp) {
		t.Errorf("Timestamp: got %v, want %v", parsed.Timestamp, patch.Timestamp)
	}
}

func TestReadAllPatches(t *testing.T) {
	dir := t.TempDir()
	store := &GitPatchStore{localDir: dir}

	// 手动创建补丁文件
	patchesDir := filepath.Join(dir, "patches", "Payment_Service", "core")
	if err := os.MkdirAll(patchesDir, 0755); err != nil {
		t.Fatal(err)
	}

	patch1 := "---\nservice: \"Payment Service\"\nmodule: \"core\"\ntype: \"archive\"\ndate: \"2026-05-16\"\nauthor: \"alice\"\npatch_id: \"11111111\"\n---\n\n场景1内容"
	patch2 := "---\nservice: \"Payment Service\"\nmodule: \"core\"\ntype: \"archive\"\ndate: \"2026-05-17\"\nauthor: \"bob\"\npatch_id: \"22222222\"\n---\n\n场景2内容"

	if err := os.WriteFile(filepath.Join(patchesDir, "2026-05-16_11111111.md"), []byte(patch1), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(patchesDir, "2026-05-17_22222222.md"), []byte(patch2), 0644); err != nil {
		t.Fatal(err)
	}

	patches, err := store.readAllPatches()
	if err != nil {
		t.Fatalf("readAllPatches error: %v", err)
	}

	if len(patches) != 2 {
		t.Fatalf("expected 2 patches, got %d", len(patches))
	}

	sort.Slice(patches, func(i, j int) bool {
		return patches[i].ID < patches[j].ID
	})

	if patches[0].ID != "11111111" {
		t.Errorf("patch 0 ID: got %q, want %q", patches[0].ID, "11111111")
	}
	if patches[0].Author != "alice" {
		t.Errorf("patch 0 Author: got %q, want %q", patches[0].Author, "alice")
	}
	if patches[1].ID != "22222222" {
		t.Errorf("patch 1 ID: got %q, want %q", patches[1].ID, "22222222")
	}
}

func TestReadAllPatchesEmpty(t *testing.T) {
	dir := t.TempDir()
	store := &GitPatchStore{localDir: dir}

	patches, err := store.readAllPatches()
	if err != nil {
		t.Fatalf("readAllPatches error: %v", err)
	}
	if len(patches) != 0 {
		t.Errorf("expected 0 patches, got %d", len(patches))
	}
}

func TestPatchFilePath(t *testing.T) {
	store := &GitPatchStore{localDir: "/tmp/test"}

	patch := Patch{
		ID:        "abc12345",
		Service:   "Payment Service",
		Module:    "核心支付模块",
		Timestamp: time.Date(2026, 5, 17, 0, 0, 0, 0, time.UTC),
	}

	path := store.patchFilePath(patch)

	// sanitizeDirName 保留中文，所以目录名是"核心支付模块"
	if !contains(path, "2026-05-17_abc12345.md") {
		t.Errorf("path should end with date_id.md, got %q", path)
	}
	if !contains(path, "patches") {
		t.Errorf("path should contain patches dir, got %q", path)
	}
	if !contains(path, "Payment_Service") {
		t.Errorf("path should contain sanitized service dir, got %q", path)
	}
}

func TestParseFrontMatter(t *testing.T) {
	content := "---\nservice: \"Test\"\nmodule: \"Mod\"\n---\n\nBody here"

	fm, body := parseFrontMatter(content)
	if fm["service"] != "Test" {
		t.Errorf("service: got %q, want %q", fm["service"], "Test")
	}
	if fm["module"] != "Mod" {
		t.Errorf("module: got %q, want %q", fm["module"], "Mod")
	}
	if !contains(body, "Body here") {
		t.Errorf("body should contain 'Body here', got: %q", body)
	}
}

func TestParseFrontMatterNone(t *testing.T) {
	content := "Just some content without front matter"
	fm, body := parseFrontMatter(content)
	if fm != nil {
		t.Errorf("expected nil front matter, got: %v", fm)
	}
	if body != content {
		t.Errorf("body should be unchanged")
	}
}

func TestParsePatchFileFallbackToDate(t *testing.T) {
	content := "---\nservice: \"Test\"\nmodule: \"Core\"\ndate: \"2026-05-18\"\nauthor: \"alice\"\npatch_id: \"abc12345\"\n---\n\nBody"

	patch, err := parsePatchFile([]byte(content))
	if err != nil {
		t.Fatalf("parsePatchFile error: %v", err)
	}
	if patch.Timestamp.IsZero() {
		t.Fatal("expected timestamp parsed from date fallback")
	}
	if patch.Timestamp.Format("2006-01-02") != "2026-05-18" {
		t.Fatalf("unexpected date fallback timestamp: %v", patch.Timestamp)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(len(s) > 0 && len(sub) > 0 && findSubstring(s, sub)))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
