package dbworkspace

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestChangedApprovedProfileRevisionFailsBeforeOpeningConnection(t *testing.T) {
	s := newNativeTestService(filepath.Join(t.TempDir(), "database.json"), &memorySecrets{values: map[string]string{}})
	p, err := s.Save(ProfileInput{Profile: Profile{Name: "test", Engine: "mysql", Host: "127.0.0.1", Port: 1, User: "test", TLS: "off"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ConnectRevision(context.Background(), p.ID, p.Revision+1); err == nil || !strings.Contains(err.Error(), "配置已改变") {
		t.Fatal("changed configuration reached connector", err)
	}
}

func TestDatabaseOwnershipIsCheckedAtAllHumanEntryPoints(t *testing.T) {
	s := New("", nil)
	s.sessions["session"] = &session{info: SessionInfo{ID: "session", ProfileID: "profile"}, gate: make(chan struct{}, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	release, err := s.Own(ctx, "session", "lease")
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range []func() error{
		func() error { _, _, err := s.acquire("session"); return err },
		func() error { return s.Disconnect("session") },
		func() error { _, err := s.Save(ProfileInput{Profile: Profile{ID: "profile"}}); return err },
		func() error { return s.Delete("profile", 1) },
	} {
		if !errors.Is(operation(), ErrOwned) {
			t.Fatal("human path bypassed ownership")
		}
	}
	_, done, err := s.acquireContext(WithOwnership(ctx, "lease"), "session")
	if err != nil {
		t.Fatal(err)
	}
	done()
	if _, _, err := s.acquireContext(WithOwnership(ctx, "old-lease"), "session"); !errors.Is(err, ErrOwned) {
		t.Fatal("stale owner admitted", err)
	}
	release()
	_, done, err = s.acquire("session")
	if err != nil {
		t.Fatal("not handed back", err)
	}
	done()
}
func TestTakeCannotOverlapAnAlreadyRunningHumanQuery(t *testing.T) {
	s := New("", nil)
	s.sessions["session"] = &session{gate: make(chan struct{}, 1)}
	_, done, err := s.acquire("session")
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	if _, err := s.Own(context.Background(), "session", "lease"); err == nil {
		t.Fatal("ownership raced past the physical-session gate")
	}
}

func TestStoppedAndReleasedCapabilitiesCannotCloseHumanSession(t *testing.T) {
	for _, stopped := range []bool{true, false} {
		s := New("", nil)
		v := &session{gate: make(chan struct{}, 1)}
		// No real connection is needed to detect an unauthorized removal.
		v.once.Do(func() {})
		s.sessions["session"] = v
		ctx, cancel := context.WithCancel(context.Background())
		release, err := s.Own(ctx, "session", "lease")
		if err != nil {
			t.Fatal(err)
		}
		if stopped {
			cancel()
		} else {
			release()
		}
		if err := s.DisconnectContext(WithOwnership(ctx, "lease"), "session"); err == nil {
			t.Fatal("obsolete AI close removed the handed-back connection")
		}
		if _, err := s.Session("session"); err != nil {
			t.Fatal("human connection lost", err)
		}
		release()
		cancel()
	}
}
func TestAssistantSQLSeparatesBoundariesFromPermission(t *testing.T) {
	for _, test := range []struct {
		sql     string
		read    bool
		invalid bool
	}{
		{"SELECT id, name FROM orders LIMIT 20", true, false}, {"SELECT COUNT(*) FROM orders;", true, false},
		{"-- 核对节点数量\nSELECT COUNT(*) FROM orders;", true, false},
		{"/*! SELECT 1 */ SELECT 1", false, true},
		{"SELECT 1; DELETE FROM orders", false, true}, {"UPDATE orders SET name='x'", false, false},
		{"SELECT load_file('/etc/passwd')", false, false}, {"SELECT password FROM users", false, false},
		{"WITH changed AS (DELETE FROM orders RETURNING *) SELECT * FROM changed", false, false},
		{"SELECT * FROM orders FOR UPDATE", false, false}, {"SELECT app.count(1)", false, false},
	} {
		read, _, err := AssistantSQL(test.sql, "mysql")
		if read != test.read || (err != nil) != test.invalid {
			t.Fatalf("%q: read=%v err=%v", test.sql, read, err)
		}
	}
}
