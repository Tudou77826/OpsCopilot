package sessionshare

import (
	"path/filepath"
	"testing"
	"time"
)

func TestEncryptDecryptDatabaseSecrets(t *testing.T) {
	payload := DatabaseSecretsPayload{
		Password: "db-secret-password",
		Gateway: &GatewaySecrets{
			Name:     "bastion-ssh",
			Host:     "39.108.107.148",
			Port:     22,
			User:     "root",
			Password: "ssh-password",
			Bastion: &GatewaySecrets{
				Host:     "10.0.0.1",
				Port:     22,
				User:     "jump",
				Password: "jump-password",
			},
		},
	}

	enc, err := EncryptDatabaseSecrets(payload, "team-key-123")
	if err != nil {
		t.Fatalf("EncryptDatabaseSecrets failed: %v", err)
	}

	got, err := DecryptDatabaseSecrets(enc, "team-key-123")
	if err != nil {
		t.Fatalf("DecryptDatabaseSecrets failed: %v", err)
	}
	if got.Password != payload.Password {
		t.Fatalf("password mismatch: got %q want %q", got.Password, payload.Password)
	}
	if got.Gateway == nil || got.Gateway.Host != "39.108.107.148" || got.Gateway.Password != "ssh-password" {
		t.Fatalf("gateway mismatch: %+v", got.Gateway)
	}
	if got.Gateway.Bastion == nil || got.Gateway.Bastion.Host != "10.0.0.1" || got.Gateway.Bastion.Password != "jump-password" {
		t.Fatalf("bastion mismatch: %+v", got.Gateway.Bastion)
	}

	if _, err := DecryptDatabaseSecrets(enc, "wrong-key"); err == nil {
		t.Fatalf("expected error with wrong key")
	}
}

func TestMergeSharedDatabasesDistinguishesGateway(t *testing.T) {
	t1 := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)

	all := []SharedDatabase{
		{Owner: "alice", Name: "DB-A", Engine: "jdbc", DriverClassName: "com.mysql.cj.jdbc.Driver", Host: "127.0.0.1", Port: 3306, User: "root", GatewayHost: "10.0.0.1", LastLoginAt: t1},
		{Owner: "bob", Name: "DB-A-newer", Engine: "jdbc", DriverClassName: "com.mysql.cj.jdbc.Driver", Host: "127.0.0.1", Port: 3306, User: "root", GatewayHost: "10.0.0.1", LastLoginAt: t2},
		{Owner: "carol", Name: "DB-B", Engine: "jdbc", DriverClassName: "com.mysql.cj.jdbc.Driver", Host: "127.0.0.1", Port: 3306, User: "root", GatewayHost: "10.0.0.2", LastLoginAt: t1},
	}

	merged := MergeSharedDatabases(all)
	if len(merged) != 2 {
		t.Fatalf("expected 2 merged databases, got %d", len(merged))
	}
	if merged[0].Owner != "bob" || merged[0].Name != "DB-A-newer" {
		t.Fatalf("expected bob's newer entry first, got %+v", merged[0])
	}
	if merged[1].GatewayHost != "10.0.0.2" {
		t.Fatalf("expected distinct gateway host entry second, got %+v", merged[1])
	}
}

func TestDatabaseLocalStateLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessionstore-databases-local.json")
	st := NewDatabaseLocalState(path)
	if err := st.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	now := time.Now().Truncate(time.Second)
	entry := LocalDatabaseEntry{
		SharedDatabase: SharedDatabase{
			Name:        "MySQL Prod",
			Engine:      "mysql",
			Host:        "10.0.1.5",
			Port:        3306,
			User:        "readonly",
			LastLoginAt: now,
		},
	}
	st.RecordLogin(entry)

	pending := st.PendingEntries()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending entry, got %d", len(pending))
	}

	st.MarkPushed(entry.EndpointKey(), now)
	if len(st.PendingEntries()) != 0 {
		t.Fatalf("expected 0 pending entries after MarkPushed")
	}

	// Reload from disk and verify persistence
	st2 := NewDatabaseLocalState(path)
	if err := st2.Load(); err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	if len(st2.PendingEntries()) != 0 {
		t.Fatalf("expected 0 pending after reload")
	}

	st2.Remove(entry.EndpointKey())
}
