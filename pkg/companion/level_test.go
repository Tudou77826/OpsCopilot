package companion

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestLevelBoundariesAndProgress(t *testing.T) {
	for _, tc := range []struct{ xp, level, progress, required int }{
		{0, 1, 0, 10}, {9, 1, 9, 10}, {10, 2, 0, 10}, {19, 2, 9, 10},
		{20, 3, 0, 10}, {60, 7, 0, 10}, {980, 99, 0, 10}, {989, 99, 9, 10},
		{990, 100, 0, 0}, {10000, 100, 0, 0}, {int(^uint(0) >> 1), 100, 0, 0},
	} {
		st := &state{Pet: &Pet{ID: "pet", SpeciesID: SpeciesID, Name: "微光", PaletteID: "mint", XP: tc.xp}}
		snap := snapshot(st, time.Now())
		if snap.Level != tc.level || snap.LevelXP != tc.progress || snap.LevelXPRequired != tc.required || snap.Pet.XP != tc.xp {
			t.Fatalf("xp %d: %+v", tc.xp, snap)
		}
		card, err := snap.PublicCard()
		if err != nil || card.Level != tc.level || !card.Valid() {
			t.Fatalf("card xp %d: %+v %v", tc.xp, card, err)
		}
		raw, _ := json.Marshal(card)
		if strings.Contains(string(raw), "stage") || strings.Contains(string(raw), "xp") {
			t.Fatalf("unexpected public fields: %s", raw)
		}
	}
	if LevelOf(-1) != 1 {
		t.Fatal("negative input must not produce a negative level")
	}
}

func TestLevelCardValidationAndLegacyReadCompatibility(t *testing.T) {
	card := Card{SpeciesID: SpeciesID, Name: "微光", PaletteID: "mint"}
	for _, level := range []int{-1, 0, 101} {
		card.Level = level
		if card.Valid() {
			t.Fatalf("accepted level %d", level)
		}
	}
	card.Level = 0
	card.Stage = "child"
	if !card.Valid() {
		t.Fatal("stored legacy card rejected")
	}
	card.Level = 101
	if card.Valid() {
		t.Fatal("legacy stage bypassed invalid numeric level")
	}
}
