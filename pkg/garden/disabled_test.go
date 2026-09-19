package garden

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDormantStoreDoesNotTouchData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "garden.json")
	s := NewDormant(path)
	if s.Enabled() {
		t.Fatal("must default off")
	}
	if _, err := s.Record(EventSessionEstablished, "disabled"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadSnapshot(); err == nil {
		t.Fatal("disabled API must fail")
	}
	if _, err := s.Purchase("cmd-mint", 35, 50); err == nil {
		t.Fatal("disabled purchase must fail")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("disabled store touched data: %v", err)
	}
	s.SetEnabled(true)
	if _, err := s.Record(EventSessionEstablished, "enabled"); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s.SetEnabled(false)
	if _, err := s.Record(EventSessionEstablished, "after-disable"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Place("missing", .5, .5, 1, false); err == nil {
		t.Fatal("disabled placement must fail")
	}
	if _, err := s.Stow("missing"); err == nil {
		t.Fatal("disabled stow must fail")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("disabled settlement changed saved data")
	}
	s.SetEnabled(true)
	snapshot, err := s.ReadSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Balance != StartingBalance+8 {
		t.Fatalf("lost data: %+v", snapshot)
	}
}

func TestDormantCorruptionOnlyReportedOnExplicitUse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "garden.json")
	if err := os.WriteFile(path, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	s := NewDormant(path)
	if _, err := s.Record(EventSessionEstablished, "disabled"); err != nil {
		t.Fatal(err)
	}
	s.SetEnabled(true)
	if _, err := s.ReadSnapshot(); err == nil {
		t.Fatal("corruption must be reported")
	}
	if _, err := s.ReadSignal(); err == nil {
		t.Fatal("corruption must be reported")
	}
}
