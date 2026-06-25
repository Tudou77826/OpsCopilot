package ops

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"opscopilot/pkg/core/security"
)

func TestDownloadChecksPolicyBeforeSFTP(t *testing.T) {
	m := newPolicyOnlyTransferManager(t)

	_, err := m.Download("test-server", "tmp/test.log", DownloadOptions{LocalPath: filepath.Join(t.TempDir(), "test.log")})
	if err == nil {
		t.Fatal("Download() error = nil, want policy error")
	}
	if !strings.Contains(err.Error(), "必须是绝对路径") {
		t.Fatalf("Download() error = %q, want absolute path policy error", err.Error())
	}
}

func TestUploadChecksPolicyBeforeLocalStat(t *testing.T) {
	m := newPolicyOnlyTransferManager(t)

	_, err := m.Upload("test-server", "/etc/test.conf", UploadOptions{LocalPath: filepath.Join(t.TempDir(), "missing.conf")})
	if err == nil {
		t.Fatal("Upload() error = nil, want policy error")
	}
	if !strings.Contains(err.Error(), "不在允许写入的路径中") {
		t.Fatalf("Upload() error = %q, want write policy error", err.Error())
	}
	if strings.Contains(err.Error(), "本地文件不存在") {
		t.Fatalf("Upload() checked local file before policy: %q", err.Error())
	}
}

func newPolicyOnlyTransferManager(t *testing.T) *Manager {
	t.Helper()

	checker, err := security.NewFileAccessChecker(filepath.Join(t.TempDir(), "file_access.json"))
	if err != nil {
		t.Fatalf("NewFileAccessChecker() error = %v", err)
	}

	conn := &Connection{Name: "test-server", Host: "127.0.0.1", ConnectedAt: time.Now()}
	conn.LastActive.Store(time.Now().UnixNano())

	return &Manager{
		connections: map[string]*Connection{
			"test-server": conn,
		},
		fileChecker: checker,
	}
}
