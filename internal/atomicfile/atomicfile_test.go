package atomicfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteCreatesFileWithContent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "sessions.json")

	if err := Write(target, []byte(`{"a":1}`), 0644); err != nil {
		t.Fatalf("Write 失败: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("读取结果失败: %v", err)
	}
	if string(got) != `{"a":1}` {
		t.Fatalf("内容不符: %q", got)
	}
}

func TestWriteOverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "sessions.json")

	if err := os.WriteFile(target, []byte("old-and-longer-content"), 0644); err != nil {
		t.Fatalf("预置文件失败: %v", err)
	}
	if err := Write(target, []byte("new"), 0644); err != nil {
		t.Fatalf("Write 失败: %v", err)
	}

	got, _ := os.ReadFile(target)
	// 长度必须缩短到新内容，确认不是"只覆盖前缀"。
	if string(got) != "new" {
		t.Fatalf("覆写后内容不符: %q", got)
	}
}

func TestWriteLeavesNoTempFilesBehind(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "sessions.json")

	if err := Write(target, []byte("x"), 0644); err != nil {
		t.Fatalf("Write 失败: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读取目录失败: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "sessions.json" {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("期望目录中只有 sessions.json，实际: %v", names)
	}
}

// 写入失败时不得破坏已存在的目标文件，这是原子写的核心价值。
func TestWriteFailurePreservesExistingFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "sessions.json")
	original := `{"keep":"me"}`
	if err := os.WriteFile(target, []byte(original), 0644); err != nil {
		t.Fatalf("预置文件失败: %v", err)
	}

	// 把父路径指向一个普通文件而非目录，CreateTemp 必然失败。
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("not a dir"), 0644); err != nil {
		t.Fatalf("创建阻塞文件失败: %v", err)
	}
	unwritable := filepath.Join(blocker, "sessions.json")

	if err := Write(unwritable, []byte("should-not-land"), 0644); err == nil {
		t.Fatal("期望写入失败，实际成功")
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("原文件丢失: %v", err)
	}
	if string(got) != original {
		t.Fatalf("失败写入改动了原文件: %q", got)
	}

	// 失败路径也不能留下临时文件残渣。
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Fatalf("失败后残留临时文件: %s", e.Name())
		}
	}
}

func TestWriteAppliesPermissions(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "nested.bin")

	if err := Write(target, []byte("data"), 0600); err != nil {
		t.Fatalf("Write 失败: %v", err)
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("Stat 失败: %v", err)
	}
	// Windows 上 chmod 只影响只读位，因此只断言非只读可写。
	if info.Mode().Perm()&0200 == 0 {
		t.Fatalf("期望文件可写，实际权限 %v", info.Mode().Perm())
	}
}
