package companion

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func enabled(t *testing.T) *Store {
	t.Helper()
	s := NewDormant(filepath.Join(t.TempDir(), "companion.json"))
	s.draw = func(n int) (int, error) { return 0, nil }
	s.SetEnabled(true)
	if _, err := s.Hatch(0); err != nil {
		t.Fatal(err)
	}
	return s
}
func TestDormantAndLegacyDataIsolation(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "garden.json")
	if err := os.WriteFile(legacy, []byte("legacy bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	s := NewDormant(filepath.Join(dir, "companion.json"))
	if _, err := s.Record(EventSessionEstablished, "ignored"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadSnapshot(); !errors.Is(err, ErrDisabled) {
		t.Fatal(err)
	}
	if _, err := s.Hatch(0); !errors.Is(err, ErrDisabled) {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.path); !os.IsNotExist(err) {
		t.Fatal("dormant store touched data")
	}
	s.SetEnabled(true)
	if _, err := s.Hatch(0); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(s.path)
	s.SetEnabled(false)
	s.Record(EventSessionEstablished, "disabled")
	after, _ := os.ReadFile(s.path)
	if string(before) != string(after) {
		t.Fatal("disabled changed data")
	}
	old, _ := os.ReadFile(legacy)
	if string(old) != "legacy bytes" {
		t.Fatal("legacy overwritten")
	}
}
func TestRenameValidationAndInsufficientExperience(t *testing.T) {
	s := enabled(t)
	snap, _ := s.ReadSnapshot()
	for _, name := range []string{"", "line\nbreak", strings.Repeat("字", 25)} {
		if _, err := s.Rename(name, snap.Revision); err == nil {
			t.Fatal("invalid name accepted")
		}
	}
	if _, err := s.Hatch(snap.Revision); !errors.Is(err, ErrInsufficientXP) {
		t.Fatal(err)
	}
}
func TestSharedSettlementCapsAndCrossDayReplay(t *testing.T) {
	s := enabled(t)
	date := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return date }
	other := NewDormant(s.path)
	other.SetEnabled(true)
	other.now = s.now
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			store := s
			if i%2 == 0 {
				store = other
			}
			if _, err := store.Record(EventSessionEstablished, "same-key"); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	snap, _ := s.ReadSnapshot()
	if snap.Pet.XP != 2 {
		t.Fatalf("duplicate XP: %d", snap.Pet.XP)
	}
	for _, event := range []struct {
		kind EventKind
		key  string
	}{{EventSessionEstablished, "second"}, {EventSessionEstablished, "capped"}, {EventTransferCompleted, "t1"}, {EventTransferCompleted, "t2"}, {EventScriptReplayDone, "r1"}} {
		if _, err := s.Record(event.kind, event.key); err != nil {
			t.Fatal(err)
		}
	}
	snap, _ = s.ReadSnapshot()
	if snap.Pet.XP != DailyXPMax {
		t.Fatal(snap)
	}
	date = date.Add(24 * time.Hour)
	out, err := other.Record(EventSessionEstablished, "capped")
	if err != nil || !out.Duplicate || out.XPGained != 0 {
		t.Fatalf("capped replay: %+v %v", out, err)
	}
	raw, _ := os.ReadFile(s.path)
	if strings.Contains(string(raw), "same-key") || strings.Contains(string(raw), "capped") {
		t.Fatal("raw key leaked")
	}
}
func TestGrowthLevelsAndPublicCard(t *testing.T) {
	s := enabled(t)
	for d := 1; d <= 6; d++ {
		date := time.Date(2026, 10, d, 12, 0, 0, 0, time.UTC)
		s.now = func() time.Time { return date }
		for i, kind := range []EventKind{EventSessionEstablished, EventTransferCompleted, EventScriptReplayDone, EventScriptReplayDone} {
			if _, err := s.Record(kind, date.String()+string(rune('a'+i))); err != nil {
				t.Fatal(err)
			}
		}
		snap, _ := s.ReadSnapshot()
		if snap.Level != LevelOf(d*DailyXPMax) || snap.LevelXP != d*DailyXPMax%XPPerLevel || snap.LevelXPRequired != XPPerLevel {
			t.Fatal(snap)
		}
	}
	snap, _ := s.ReadSnapshot()
	if snap.Level != 8 {
		t.Fatal(snap)
	}
	card, err := snap.PublicCard()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(card)
	for _, private := range []string{"xp", "adoptedAt", "revision", "seen", "daily", snap.Pet.ID} {
		if strings.Contains(string(raw), private) {
			t.Fatalf("private field %s", private)
		}
	}
	snap.Pet.Name = "tampered"
	next, _ := s.ReadSnapshot()
	if next.Pet.Name != speciesCatalog[0].Name {
		t.Fatal("snapshot mutated storage")
	}
}
func TestCorruptionPreservedAndNoRewardBeforeAdoption(t *testing.T) {
	s := NewDormant(filepath.Join(t.TempDir(), "companion.json"))
	s.SetEnabled(true)
	if out, err := s.Record(EventSessionEstablished, "early"); err != nil || out.XPGained != 0 {
		t.Fatal(out, err)
	}
	if _, err := os.Stat(s.path); !os.IsNotExist(err) {
		t.Fatal("unadopted event created save")
	}
	raw := []byte(`{"schemaVersion":99}`)
	os.WriteFile(s.path, raw, 0600)
	if _, err := s.ReadSnapshot(); err == nil {
		t.Fatal("bad version ignored")
	}
	if _, err := s.Hatch(0); err == nil {
		t.Fatal("bad version overwritten")
	}
	after, _ := os.ReadFile(s.path)
	if string(after) != string(raw) {
		t.Fatal("corruption overwritten")
	}
}
