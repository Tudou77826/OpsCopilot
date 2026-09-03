package sessionmanager

import (
	"opscopilot/pkg/remote"
	"path/filepath"
	"testing"
)

func TestSharedSessionEditsPreserveOtherRecordsAndDoNotResurrectDeletedIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	a, b := NewManagerWithPath(path), NewManagerWithPath(path)
	for _, host := range []string{"a.invalid", "b.invalid"} {
		if err := a.Upsert(remote.ConnectConfig{Host: host, Port: 22, User: "test"}, ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.Load(); err != nil {
		t.Fatal(err)
	}
	nodes := b.GetSessions()
	first, second := nodes[0].ID, nodes[1].ID
	if err := a.RenameSession(first, "edited-a"); err != nil {
		t.Fatal(err)
	}
	if err := b.RenameSession(second, "edited-b"); err != nil {
		t.Fatal(err)
	}
	if err := a.Load(); err != nil {
		t.Fatal(err)
	}
	nodes = a.GetSessions()
	if nodes[0].Name != "edited-a" || nodes[1].Name != "edited-b" {
		t.Fatal("stale manager overwrote another record")
	}
	if err := a.DeleteSession(first); err != nil {
		t.Fatal(err)
	}
	if err := b.UpdateSession(first, *nodes[0].Config, ""); err == nil {
		t.Fatal("deleted ID was resurrected by stale editor")
	}
	if err := b.RenameSession(first, "resurrect"); err == nil {
		t.Fatal("deleted ID was renamed")
	}
	if err := a.Load(); err != nil {
		t.Fatal(err)
	}
	nodes = a.GetSessions()
	if len(nodes) != 1 || nodes[0].ID != second || nodes[0].Name != "edited-b" {
		t.Fatal("failed edit changed persisted records")
	}
}
