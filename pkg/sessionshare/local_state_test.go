package sessionshare

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestState(t *testing.T) *LocalState {
	t.Helper()
	state := NewLocalState(filepath.Join(t.TempDir(), "sessionstore-local.json"))
	if err := state.Load(); err != nil {
		t.Fatalf("Load error: %v", err)
	}
	return state
}

func TestLocalStateRecordAndPending(t *testing.T) {
	state := newTestState(t)
	now := time.Now()

	state.RecordLogin(LocalEntry{
		Host: "10.0.0.1", Port: 22, User: "root",
		SecretsEnc: "v1:abc", LastLoginAt: now,
	})

	pending := state.PendingEntries()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(pending))
	}
	if !pending[0].LastLoginAt.Equal(now) {
		t.Errorf("pending LastLoginAt mismatch")
	}

	// 推送后不再待推
	state.MarkPushed("", "10.0.0.1", 22, "root", now)
	if pending := state.PendingEntries(); len(pending) != 0 {
		t.Errorf("expected no pending after MarkPushed, got %d", len(pending))
	}

	// 新登录再次进入待推
	later := now.Add(time.Hour)
	state.RecordLogin(LocalEntry{
		Host: "10.0.0.1", Port: 22, User: "root",
		SecretsEnc: "v1:def", LastLoginAt: later,
	})
	if pending := state.PendingEntries(); len(pending) != 1 {
		t.Fatalf("expected 1 pending after new login, got %d", len(pending))
	}
}

func TestLocalStateLoginMonotonic(t *testing.T) {
	state := newTestState(t)
	later := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	earlier := later.Add(-time.Hour)

	state.RecordLogin(LocalEntry{Host: "h", Port: 22, User: "u", LastLoginAt: later})
	state.RecordLogin(LocalEntry{Host: "h", Port: 22, User: "u", LastLoginAt: earlier})

	pending := state.PendingEntries()
	if len(pending) != 1 || !pending[0].LastLoginAt.Equal(later) {
		t.Errorf("out-of-order login must not rewind LastLoginAt: %+v", pending)
	}
}

func TestLocalStatePreservesSecretsOnEmptyNewLogin(t *testing.T) {
	state := newTestState(t)
	first := time.Now()

	// 第一次登录带密文；第二次（密钥认证，无密码）不带
	state.RecordLogin(LocalEntry{Host: "h", Port: 22, User: "u", SecretsEnc: "v1:old", LastLoginAt: first})
	state.RecordLogin(LocalEntry{Host: "h", Port: 22, User: "u", LastLoginAt: first.Add(time.Minute)})

	pending := state.PendingEntries()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(pending))
	}
	if pending[0].SecretsEnc != "v1:old" {
		t.Errorf("empty-secrets login must preserve previous ciphertext, got %q", pending[0].SecretsEnc)
	}
}

func TestLocalStateRemove(t *testing.T) {
	state := newTestState(t)
	state.RecordLogin(LocalEntry{Host: "h", Port: 22, User: "u", LastLoginAt: time.Now()})
	state.Remove("", "h", 22, "u")
	if pending := state.PendingEntries(); len(pending) != 0 {
		t.Errorf("expected entry removed, got %d pending", len(pending))
	}
}

func TestLocalStatePersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sessionstore-local.json")

	state := NewLocalState(path)
	if err := state.Load(); err != nil {
		t.Fatal(err)
	}
	login := time.Date(2026, 8, 18, 10, 0, 0, 0, time.UTC)
	state.RecordLogin(LocalEntry{Host: "h", Port: 2222, User: "ops", SecretsEnc: "v1:x", LastLoginAt: login})

	// 重新加载（模拟应用重启）
	reloaded := NewLocalState(path)
	if err := reloaded.Load(); err != nil {
		t.Fatal(err)
	}
	pending := reloaded.PendingEntries()
	if len(pending) != 1 || pending[0].Port != 2222 || !pending[0].LastLoginAt.Equal(login) {
		t.Errorf("state must survive reload, got %+v", pending)
	}
}
