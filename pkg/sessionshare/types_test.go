package sessionshare

import (
	"testing"
	"time"
)

func TestSameEndpoint(t *testing.T) {
	base := SharedSession{Protocol: "ssh", Host: "10.0.0.1", Port: 22, User: "root"}

	cases := []struct {
		name  string
		other SharedSession
		want  bool
	}{
		{"identical", SharedSession{Protocol: "ssh", Host: "10.0.0.1", Port: 22, User: "root"}, true},
		{"empty-protocol-normalized-to-ssh", SharedSession{Host: "10.0.0.1", Port: 22, User: "root"}, true},
		{"different-host", SharedSession{Protocol: "ssh", Host: "10.0.0.2", Port: 22, User: "root"}, false},
		{"different-port", SharedSession{Protocol: "ssh", Host: "10.0.0.1", Port: 23, User: "root"}, false},
		{"different-user", SharedSession{Protocol: "ssh", Host: "10.0.0.1", Port: 22, User: "admin"}, false},
		{"different-protocol", SharedSession{Protocol: "telnet", Host: "10.0.0.1", Port: 22, User: "root"}, false},
		// Owner/Name/时间不影响端点判定
		{"owner-irrelevant", SharedSession{Protocol: "ssh", Host: "10.0.0.1", Port: 22, User: "root", Owner: "other", Name: "x"}, true},
	}
	for _, c := range cases {
		if got := SameEndpoint(base, c.other); got != c.want {
			t.Errorf("%s: SameEndpoint = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestEntryKey(t *testing.T) {
	s := SharedSession{Owner: "张三", Protocol: "", Host: "h1", Port: 22, User: "root"}
	if got, want := s.EntryKey(), "张三|ssh|h1|22|root"; got != want {
		t.Errorf("EntryKey = %q, want %q", got, want)
	}
}

func TestMergeSharedSessionsNewestLoginWins(t *testing.T) {
	older := SharedSession{
		Owner: "alice", Host: "10.0.0.1", Port: 22, User: "root",
		LastLoginAt: time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC),
	}
	newer := SharedSession{
		Owner: "bob", Host: "10.0.0.1", Port: 22, User: "root",
		LastLoginAt: time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC),
	}
	distinct := SharedSession{
		Owner: "alice", Host: "10.0.0.2", Port: 22, User: "root",
		LastLoginAt: time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC),
	}

	merged := MergeSharedSessions([]SharedSession{older, distinct, newer})

	if len(merged) != 2 {
		t.Fatalf("expected 2 merged entries, got %d", len(merged))
	}
	// 排序按 LastLoginAt 降序：bob(08-15) 在前，alice(08-10) 在后
	if merged[0].Owner != "bob" {
		t.Errorf("newest entry should be bob's, got %s", merged[0].Owner)
	}
	if merged[1].Host != "10.0.0.2" {
		t.Errorf("second entry should be distinct host, got %s", merged[1].Host)
	}
}

func TestMergeSharedSessionsOrderIndependent(t *testing.T) {
	a := SharedSession{Owner: "a", Host: "h", Port: 22, User: "u",
		LastLoginAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)}
	b := SharedSession{Owner: "b", Host: "h", Port: 22, User: "u",
		LastLoginAt: time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)}

	m1 := MergeSharedSessions([]SharedSession{a, b})
	m2 := MergeSharedSessions([]SharedSession{b, a})
	if len(m1) != 1 || len(m2) != 1 {
		t.Fatalf("expected dedup to 1 entry, got %d/%d", len(m1), len(m2))
	}
	if m1[0].Owner != m2[0].Owner || m1[0].Owner != "b" {
		t.Errorf("merge result must be order-independent and pick newest, got %s/%s",
			m1[0].Owner, m2[0].Owner)
	}
}

func TestMergeSharedSessionsEmpty(t *testing.T) {
	if merged := MergeSharedSessions(nil); len(merged) != 0 {
		t.Errorf("nil input should yield empty, got %d", len(merged))
	}
}
