package dbcontrol

import (
	"context"
	"testing"

	"opscopilot/pkg/dbworkspace"
)

func TestRevealRetainedDisconnectedSQLNeverExecutesOrReconnects(t *testing.T) {
	s, db, _, w, lease := openTest(t)
	b, err := s.Append(lease, w.ID, "turn", "查询测试", "select 1")
	if err != nil {
		t.Fatal(err)
	}
	s.SetBlock(w.ID, b.ID, "completed", dbworkspace.QueryResult{}, "")
	s.Release(w.ID, "turn")
	if err = s.Close(context.Background(), w.ID, ""); err != nil {
		t.Fatal(err)
	}
	stop := presentAll(s)
	defer stop()
	if err = s.Reveal(context.Background(), w.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Snapshot(w.ID)
	if got.Connected || got.OwnerTurn != "" || got.RevealBlock != b.ID || got.RevealNonce == "" || len(db.queries) != 0 {
		t.Fatal("navigation executed or took a connection", got)
	}
	if s.Reveal(context.Background(), w.ID, "missing") == nil {
		t.Fatal("missing block accepted")
	}
	if err = s.Dismiss(w.ID); err != nil {
		t.Fatal(err)
	}
	if s.Reveal(context.Background(), w.ID, b.ID) == nil {
		t.Fatal("dismissed workspace reopened")
	}
}
