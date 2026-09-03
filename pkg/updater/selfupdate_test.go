package updater

import (
	"context"
	"errors"
	"fmt"
	"opscopilot/pkg/installguard"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSelfUpdateProtectsOtherRuntimeBeforeReplacingFiles(t *testing.T) {
	root := t.TempDir()
	source := t.TempDir()
	fs := newMemFS()
	exe := filepath.Join(root, "not-an-executable.exe")
	fs.writeFile(exe, []byte("old"))
	fs.writeFile(filepath.Join(source, "not-an-executable.exe"), []byte("new"))
	fs.writeFile(filepath.Join(source, installguard.RuntimeFile), []byte("must-not-replace-lock"))
	lease, err := installguard.AcquireRuntime(root)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	m := &Manifest{AppDir: root, ExtractedDir: source, ExePath: exe, Version: "test"}
	if err = SelfUpdate(context.Background(), m, fs, &MockProcessWaiter{}); !errors.Is(err, installguard.ErrBusy) {
		t.Fatal("must reject other instance", err)
	}
	data, _ := fs.getFile(exe)
	if string(data) != "old" {
		t.Fatal("live exe replaced")
	}
	result, err := readResultFile(filepath.Join(root, resultFile), fs)
	if err != nil || result.Success {
		t.Fatal("missing failure result", err)
	}
	lease.Close()
	if err = SelfUpdate(context.Background(), m, fs, &MockProcessWaiter{}); err != nil {
		t.Fatal(err)
	}
	data, _ = fs.getFile(exe)
	if string(data) != "new" {
		t.Fatal("update failed after release")
	}
	if fs.hasFile(filepath.Join(root, installguard.RuntimeFile)) {
		t.Fatal("lock file included in update")
	}
	next, err := installguard.AcquireRuntime(root)
	if err != nil {
		t.Fatal("update leaked lock", err)
	}
	next.Close()
}

// MemFS is an in-memory filesystem for testing.
type MemFS struct {
	mu    sync.Mutex
	files map[string][]byte
	dirs  map[string]bool
	errs  map[string]error // inject errors for specific paths
}

func newMemFS() *MemFS {
	return &MemFS{
		files: make(map[string][]byte),
		dirs:  make(map[string]bool),
		errs:  make(map[string]error),
	}
}

func (m *MemFS) ReadDir(dirname string) ([]os.DirEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.errs[dirname] != nil {
		return nil, m.errs[dirname]
	}
	dirPrefix := filepath.Clean(dirname) + string(os.PathSeparator)
	var entries []os.DirEntry
	for path := range m.files {
		clean := filepath.Clean(path)
		if !strings.HasPrefix(clean, dirPrefix) {
			continue
		}
		rel := strings.TrimPrefix(clean, dirPrefix)
		if strings.Contains(rel, string(os.PathSeparator)) {
			continue
		}
		entries = append(entries, &memDirEntry{name: rel, isDir: false})
	}
	for dir := range m.dirs {
		clean := filepath.Clean(dir)
		if !strings.HasPrefix(clean, dirPrefix) {
			continue
		}
		rel := strings.TrimPrefix(clean, dirPrefix)
		if strings.Contains(rel, string(os.PathSeparator)) {
			continue
		}
		if rel == "" {
			continue
		}
		entries = append(entries, &memDirEntry{name: rel, isDir: true})
	}
	return entries, nil
}

func (m *MemFS) Rename(oldpath, newpath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.errs[oldpath] != nil {
		return m.errs[oldpath]
	}
	data, ok := m.files[filepath.Clean(oldpath)]
	if !ok {
		return fmt.Errorf("file not found: %s", oldpath)
	}
	delete(m.files, filepath.Clean(oldpath))
	m.files[filepath.Clean(newpath)] = data
	return nil
}

func (m *MemFS) CopyFile(src, dst string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.errs[src] != nil {
		return m.errs[src]
	}
	if m.errs[dst] != nil {
		return m.errs[dst]
	}
	data, ok := m.files[filepath.Clean(src)]
	if !ok {
		return fmt.Errorf("file not found: %s", src)
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	m.files[filepath.Clean(dst)] = cp
	return nil
}

func (m *MemFS) Stat(name string) (os.FileInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	clean := filepath.Clean(name)
	if data, ok := m.files[clean]; ok {
		return &memFileInfo{name: filepath.Base(clean), size: int64(len(data))}, nil
	}
	return nil, fmt.Errorf("file not found: %s", name)
}

func (m *MemFS) Remove(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.files, filepath.Clean(name))
	return nil
}

func (m *MemFS) MkdirAll(path string, perm os.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dirs[filepath.Clean(path)] = true
	return nil
}

func (m *MemFS) writeFile(path string, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[filepath.Clean(path)] = data
}

func (m *MemFS) hasFile(path string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.files[filepath.Clean(path)]
	return ok
}

func (m *MemFS) getFile(path string) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.files[filepath.Clean(path)]
	return data, ok
}

func (m *MemFS) ReadFile(name string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if data, ok := m.files[filepath.Clean(name)]; ok {
		cp := make([]byte, len(data))
		copy(cp, data)
		return cp, nil
	}
	return nil, fmt.Errorf("file not found: %s", name)
}

func (m *MemFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[filepath.Clean(name)] = data
	return nil
}

type memDirEntry struct {
	name  string
	isDir bool
}

func (e *memDirEntry) Name() string               { return e.name }
func (e *memDirEntry) IsDir() bool                { return e.isDir }
func (e *memDirEntry) Type() os.FileMode          { return 0 }
func (e *memDirEntry) Info() (os.FileInfo, error) { return nil, nil }

type memFileInfo struct {
	name string
	size int64
}

func (i *memFileInfo) Name() string       { return i.name }
func (i *memFileInfo) Size() int64        { return i.size }
func (i *memFileInfo) Mode() os.FileMode  { return 0644 }
func (i *memFileInfo) ModTime() time.Time { return time.Time{} }
func (i *memFileInfo) IsDir() bool        { return false }
func (i *memFileInfo) Sys() interface{}   { return nil }

// MockProcessWaiter returns immediately.
type MockProcessWaiter struct {
	Err error
}

func (m *MockProcessWaiter) Wait(pid int, timeout time.Duration) error {
	return m.Err
}

func TestPlanUpdate(t *testing.T) {
	fs := newMemFS()
	srcDir := "/tmp/extracted"
	dstDir := "/app"

	fs.writeFile(filepath.Join(srcDir, "opscopilot.exe"), []byte("new-exe"))
	fs.writeFile(filepath.Join(srcDir, "prompts.json"), []byte("{}"))
	fs.writeFile(filepath.Join(srcDir, "config.json"), []byte("{}"))
	fs.writeFile(filepath.Join(srcDir, "mcp-server.exe"), []byte("mcp"))

	ops, err := planUpdate(srcDir, dstDir, fs)
	if err != nil {
		t.Fatalf("planUpdate: %v", err)
	}

	names := make(map[string]bool)
	for _, op := range ops {
		names[op.fileName] = true
	}
	if !names["opscopilot.exe"] {
		t.Error("expected opscopilot.exe in plan")
	}
	if !names["prompts.json"] {
		t.Error("expected prompts.json in plan")
	}
	if names["config.json"] {
		t.Error("did NOT expect config.json in plan (protected)")
	}
	if names["mcp-server.exe"] {
		t.Error("did NOT expect mcp-server.exe in plan (protected)")
	}
}

func TestPlanUpdateEmptyDir(t *testing.T) {
	fs := newMemFS()
	srcDir := "/tmp/extracted"

	ops, err := planUpdate(srcDir, "/app", fs)
	if err != nil {
		t.Fatalf("planUpdate empty dir: %v", err)
	}
	if len(ops) != 0 {
		t.Errorf("expected 0 ops for empty dir, got %d", len(ops))
	}
}

func TestBackupFiles(t *testing.T) {
	fs := newMemFS()
	appDir := "/app"

	ops := []fileOp{
		{srcPath: "/tmp/a.txt", dstPath: filepath.Join(appDir, "a.txt"), bakPath: filepath.Join(appDir, "a.txt.bak"), fileName: "a.txt"},
		{srcPath: "/tmp/b.txt", dstPath: filepath.Join(appDir, "b.txt"), bakPath: filepath.Join(appDir, "b.txt.bak"), fileName: "b.txt"},
	}

	fs.writeFile(filepath.Join(appDir, "a.txt"), []byte("old-a"))
	fs.writeFile(filepath.Join(appDir, "b.txt"), []byte("old-b"))

	log := &updateLogger{}
	rollback, err := backupFiles(ops, fs, log)
	if err != nil {
		t.Fatalf("backupFiles: %v", err)
	}
	if rollback == nil {
		t.Error("expected non-nil rollback")
	}

	if !fs.hasFile(filepath.Join(appDir, "a.txt.bak")) {
		t.Error("expected a.txt.bak to exist")
	}
	if !fs.hasFile(filepath.Join(appDir, "b.txt.bak")) {
		t.Error("expected b.txt.bak to exist")
	}

	// Verify backup content
	if data, ok := fs.getFile(filepath.Join(appDir, "a.txt.bak")); !ok || string(data) != "old-a" {
		t.Error("backup content mismatch for a.txt")
	}
}

func TestBackupFilesNewFileOK(t *testing.T) {
	fs := newMemFS()
	appDir := "/app"

	ops := []fileOp{
		{srcPath: "/tmp/new.txt", dstPath: filepath.Join(appDir, "new.txt"), bakPath: filepath.Join(appDir, "new.txt.bak"), fileName: "new.txt"},
	}

	log := &updateLogger{}
	rollback, err := backupFiles(ops, fs, log)
	if err != nil {
		t.Fatalf("backupFiles for new file: %v", err)
	}

	// No backup created for non-existent file
	if fs.hasFile(filepath.Join(appDir, "new.txt.bak")) {
		t.Error("did NOT expect backup for non-existent file")
	}
	_ = rollback
}

func TestCopyFiles(t *testing.T) {
	fs := newMemFS()
	appDir := "/app"

	fs.writeFile("/tmp/a.txt", []byte("new-a"))
	fs.writeFile(filepath.Join(appDir, "a.txt"), []byte("old-a"))

	ops := []fileOp{
		{srcPath: "/tmp/a.txt", dstPath: filepath.Join(appDir, "a.txt"), bakPath: filepath.Join(appDir, "a.txt.bak"), fileName: "a.txt"},
	}

	log := &updateLogger{}
	rollback := func() {}

	if err := copyFiles(ops, fs, log, rollback); err != nil {
		t.Fatalf("copyFiles: %v", err)
	}

	data, ok := fs.getFile(filepath.Join(appDir, "a.txt"))
	if !ok {
		t.Fatal("a.txt should exist")
	}
	if string(data) != "new-a" {
		t.Errorf("a.txt = %q, want %q", string(data), "new-a")
	}
}

func TestCopyFilesFailureRollback(t *testing.T) {
	fs := newMemFS()
	appDir := "/app"

	fs.writeFile("/tmp/a.txt", []byte("new-a"))
	fs.writeFile("/tmp/b.txt", []byte("new-b"))
	fs.writeFile(filepath.Join(appDir, "a.txt"), []byte("old-a"))
	fs.writeFile(filepath.Join(appDir, "b.txt"), []byte("old-b"))

	ops := []fileOp{
		{srcPath: "/tmp/a.txt", dstPath: filepath.Join(appDir, "a.txt"), bakPath: filepath.Join(appDir, "a.txt.bak"), fileName: "a.txt"},
		{srcPath: "/tmp/b.txt", dstPath: filepath.Join(appDir, "b.txt"), bakPath: filepath.Join(appDir, "b.txt.bak"), fileName: "b.txt"},
	}

	// Inject error on second copy
	fs.errs[filepath.Join(appDir, "b.txt")] = fmt.Errorf("copy denied")

	log := &updateLogger{}
	rollbackCalled := false
	rollback := func() { rollbackCalled = true }

	err := copyFiles(ops, fs, log, rollback)
	if err == nil {
		t.Fatal("expected error for copy failure")
	}
	if !rollbackCalled {
		t.Error("expected rollback to be called")
	}
}

func TestCleanupAfterUpdate(t *testing.T) {
	fs := newMemFS()
	appDir := "/app"

	fs.writeFile(filepath.Join(appDir, "opscopilot.exe"), []byte("exe"))
	fs.writeFile(filepath.Join(appDir, "opscopilot.exe.old"), []byte("old-exe"))
	fs.writeFile(filepath.Join(appDir, "prompts.json.bak"), []byte("bak"))
	fs.writeFile(filepath.Join(appDir, resultFile), []byte(`{"success":true,"version":"v1.5.4","timestamp":"2024-01-01T00:00:00Z"}`))

	result, err := CleanupAfterUpdate(appDir, fs)
	if err != nil {
		t.Fatalf("CleanupAfterUpdate: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
	if !result.Success {
		t.Error("expected Success=true")
	}
	if result.Version != "v1.5.4" {
		t.Errorf("Version = %q, want v1.5.4", result.Version)
	}
	if fs.hasFile(filepath.Join(appDir, "opscopilot.exe.old")) {
		t.Error("expected .old file to be cleaned up")
	}
	if fs.hasFile(filepath.Join(appDir, "prompts.json.bak")) {
		t.Error("expected .bak file to be cleaned up")
	}
	if fs.hasFile(filepath.Join(appDir, resultFile)) {
		t.Error("expected result file to be cleaned up")
	}
	if !fs.hasFile(filepath.Join(appDir, "opscopilot.exe")) {
		t.Error("expected exe to remain")
	}
}

func TestCleanupAfterUpdateNoResult(t *testing.T) {
	fs := newMemFS()
	appDir := "/app"

	fs.writeFile(filepath.Join(appDir, "opscopilot.exe"), []byte("exe"))

	result, err := CleanupAfterUpdate(appDir, fs)
	if err != nil {
		t.Fatalf("CleanupAfterUpdate: %v", err)
	}
	if result != nil {
		t.Error("expected nil result when no result file")
	}
}
