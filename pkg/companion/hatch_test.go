package companion

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"opscopilot/pkg/filetxn"
)

func funded(t *testing.T, xp int) *Store {
	t.Helper()
	s := enabled(t)
	st, err := s.load()
	if err != nil {
		t.Fatal(err)
	}
	st.Pet.XP = xp
	st.Pet.Name, st.Pet.PaletteID = "原来的名字", "lavender"
	if err := s.write(st); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestFirstHatchDrawsFromEntireCatalogAndPersists(t *testing.T) {
	for index, species := range speciesCatalog {
		s := NewDormant(filepath.Join(t.TempDir(), "companion.json"))
		s.SetEnabled(true)
		s.draw = func(n int) (int, error) {
			if n != len(speciesCatalog) {
				t.Fatal("first draw excluded candidates", n)
			}
			return index, nil
		}
		empty, _ := s.ReadSnapshot()
		if empty.HatchCost != 0 || !empty.CanHatch {
			t.Fatal(empty)
		}
		first, err := s.Hatch(empty.Revision)
		if err != nil {
			t.Fatal(err)
		}
		if first.Pet.SpeciesID != species.ID || first.Pet.Name != species.Name || first.Pet.XP != 0 || first.Level != 1 || first.HatchCost != 50 || first.CanHatch || !firstMustCard(first, t).Valid() {
			t.Fatal(first)
		}
		reopened := NewDormant(s.path)
		reopened.SetEnabled(true)
		saved, err := reopened.ReadSnapshot()
		if err != nil || !reflect.DeepEqual(first, saved) {
			t.Fatal("hatch not persisted", err)
		}
	}
}

func TestPaidHatchExcludesCurrentAndPreservesHistory(t *testing.T) {
	for index := 0; index < len(speciesCatalog)-1; index++ {
		s := funded(t, 73)
		s.now = func() time.Time { return time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC) }
		if _, err := s.Record(EventSessionEstablished, "once"); err != nil {
			t.Fatal(err)
		}
		before, _ := s.ReadSnapshot()
		history, _ := s.load()
		s.draw = func(n int) (int, error) {
			if n != len(speciesCatalog)-1 {
				t.Fatal("paid draw includes current species", n)
			}
			return index, nil
		}
		next, err := s.Hatch(before.Revision)
		if err != nil {
			t.Fatal(err)
		}
		expected := speciesCatalog[index+1].ID
		if next.Pet.SpeciesID != expected || next.Pet.SpeciesID == before.Pet.SpeciesID || next.Pet.PaletteID != "mint" || next.Pet.Name != before.Pet.Name || next.Pet.ID != before.Pet.ID || next.Pet.AdoptedAt != before.Pet.AdoptedAt {
			t.Fatal(next)
		}
		if next.Pet.XP != 25 || next.Level != 3 || next.LevelXP != 5 || next.Revision != before.Revision+1 || next.TodayXP != before.TodayXP {
			t.Fatal("incorrect settlement", next)
		}
		after, _ := s.load()
		if !reflect.DeepEqual(history.Seen, after.Seen) || !reflect.DeepEqual(history.Daily, after.Daily) {
			t.Fatal("hatch reset reward limits")
		}
		out, err := s.Record(EventSessionEstablished, "once")
		if err != nil || !out.Duplicate {
			t.Fatal("replayed reward after spending", err)
		}
	}
}

func TestHatchAtExactCostAndConcurrentRetryChargesOnlyOnce(t *testing.T) {
	s := funded(t, 50)
	before, _ := s.ReadSnapshot()
	other := NewDormant(s.path)
	other.SetEnabled(true)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, store := range []*Store{s, other} {
		wg.Add(1)
		go func(store *Store) { defer wg.Done(); _, err := store.Hatch(before.Revision); results <- err }(store)
	}
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, filetxn.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	after, _ := s.ReadSnapshot()
	if successes != 1 || conflicts != 1 || after.Pet.XP != 0 || after.Level != 1 || after.Revision != before.Revision+1 {
		t.Fatal(successes, conflicts, after)
	}
}

func TestHatchFailureNeverChargesOrReplacesPartner(t *testing.T) {
	for _, kind := range []string{"balance", "stale", "random", "write", "disabled"} {
		t.Run(kind, func(t *testing.T) {
			xp := 65
			if kind == "balance" {
				xp = 49
			}
			s := funded(t, xp)
			before, _ := s.ReadSnapshot()
			raw, _ := os.ReadFile(s.path)
			revision := before.Revision
			if kind == "stale" {
				revision--
			}
			if kind == "disabled" {
				s.SetEnabled(false)
			}
			var blocker *os.File
			s.draw = func(int) (int, error) {
				if kind == "random" {
					return 0, errors.New("entropy unavailable")
				}
				if kind == "write" {
					// Prevent atomic replacement on Windows; on Unix replace the
					// target with a directory after the locked read instead.
					blocker, _ = os.OpenFile(s.path, os.O_RDONLY, 0)
					if os.PathSeparator != '\\' {
						blocker.Close()
						blocker = nil
						if err := os.Rename(s.path, s.path+".original"); err != nil {
							t.Fatal(err)
						}
						if err := os.Mkdir(s.path, 0700); err != nil {
							t.Fatal(err)
						}
					}
				}
				return 1, nil
			}
			_, err := s.Hatch(revision)
			if blocker != nil {
				blocker.Close()
			}
			if kind == "write" && os.PathSeparator != '\\' {
				os.Remove(s.path)
				os.Rename(s.path+".original", s.path)
			}
			if err == nil {
				t.Fatal("failure accepted")
			}
			if kind == "balance" && !errors.Is(err, ErrInsufficientXP) {
				t.Fatal(err)
			}
			after, _ := os.ReadFile(s.path)
			if !bytes.Equal(raw, after) {
				t.Fatal("failed hatch changed save")
			}
		})
	}
}
