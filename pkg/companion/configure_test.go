package companion

import (
	"errors"
	"opscopilot/pkg/filetxn"
	"os"
	"testing"
	"time"
)

func TestRenamePreservesProgressAndRejectsStaleHost(t *testing.T) {
	s := enabled(t)
	date := time.Date(2026, 10, 10, 23, 59, 0, 0, time.UTC)
	s.now = func() time.Time { return date }
	s.Record(EventSessionEstablished, "connection")
	before, _ := s.ReadSnapshot()
	if before.TodayXP != 2 || before.DailyXPMax != 12 {
		t.Fatal(before)
	}
	other := NewDormant(s.path)
	other.SetEnabled(true)
	next, err := other.Rename(" 新名字 ", before.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if next.Pet.XP != 2 || next.Pet.ID != before.Pet.ID || next.Pet.AdoptedAt != before.Pet.AdoptedAt || next.Pet.Name != "新名字" || next.Revision != before.Revision+1 {
		t.Fatal(next)
	}
	if _, err = s.Rename("stale", before.Revision); !errors.Is(err, filetxn.ErrConflict) {
		t.Fatal(err)
	}
	stable, err := s.Rename("新名字", next.Revision)
	if err != nil || stable.Revision != next.Revision {
		t.Fatal(stable, err)
	}
	date = date.Add(2 * time.Minute)
	tomorrow, _ := s.ReadSnapshot()
	if tomorrow.TodayXP != 0 || tomorrow.Pet.XP != 2 {
		t.Fatal(tomorrow)
	}
	s.SetEnabled(false)
	raw, _ := os.ReadFile(s.path)
	if _, err = s.Rename("disabled", next.Revision); !errors.Is(err, ErrDisabled) {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(s.path)
	if string(raw) != string(after) {
		t.Fatal("disabled changed save")
	}
}
