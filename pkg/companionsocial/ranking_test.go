package companionsocial

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func candidate(id UserID, count int, participating bool) RankingCandidate {
	return RankingCandidate{Profile: Profile{UserID: id, DisplayName: string(id), Medals: []Medal{}}, Participating: participating,
		Achievements: AchievementSummary{RuleVersion: 1, Progress: []AchievementProgress{{Kind: MedalCaseCompleted, Count: count}}}}
}

func TestRankingTiesPagingAndParticipation(t *testing.T) {
	input := []RankingCandidate{candidate("d", 2, true), candidate("c", 5, true), candidate("a", 10, true), candidate("b", 5, true), candidate("zero", 0, true), candidate("private", 99, false)}
	page, err := RankAchievements(MedalCaseCompleted, "snapshot-1", input, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 4 || page.SnapshotID != "snapshot-1" {
		t.Fatalf("bad snapshot: %+v", page)
	}
	for i, want := range []int{1, 2, 2, 4} {
		if page.Entries[i].Rank != want || page.Entries[i].Profile.UserID != []UserID{"a", "b", "c", "d"}[i] {
			t.Fatalf("unstable ties: %+v", page.Entries)
		}
	}
	partial, err := RankAchievements(MedalCaseCompleted, "snapshot-1", input, 2, 1)
	if err != nil || len(partial.Entries) != 1 || partial.Entries[0].Rank != 2 || partial.Total != 4 {
		t.Fatalf("pagination reset rank: %+v / %v", partial, err)
	}
	empty, err := RankAchievements(MedalCaseCompleted, "snapshot-1", input, math.MaxInt, 100)
	if err != nil || len(empty.Entries) != 0 {
		t.Fatalf("out of range pagination failed: %+v / %v", empty, err)
	}
	payload, _ := json.Marshal(page)
	if strings.Contains(string(payload), "private") || strings.Contains(string(payload), "achievements") {
		t.Fatalf("private data leaked: %s", payload)
	}
}

func TestRankingRejectsInvalidSnapshotsAndMembers(t *testing.T) {
	base := candidate("a", 1, true)
	for _, input := range [][]RankingCandidate{
		{base, base}, {candidate("", 1, true)}, {candidate("a", -1, true)},
		{{Profile: base.Profile, Participating: true, Achievements: AchievementSummary{RuleVersion: 2, Progress: base.Achievements.Progress}}},
		{{Profile: base.Profile, Participating: true, Achievements: AchievementSummary{RuleVersion: 1}}},
		{{Profile: base.Profile, Participating: true, Achievements: AchievementSummary{RuleVersion: 1, Progress: append(base.Achievements.Progress, base.Achievements.Progress...)}}},
	} {
		if _, err := RankAchievements(MedalCaseCompleted, "snapshot", input, 0, 10); err == nil {
			t.Fatal("invalid ranking input accepted")
		}
	}
	for _, args := range []struct {
		kind          MedalKind
		snapshot      string
		offset, limit int
	}{
		{"unknown", "snapshot", 0, 10}, {MedalCaseCompleted, "", 0, 10}, {MedalCaseCompleted, "snapshot", -1, 10}, {MedalCaseCompleted, "snapshot", 0, 0}, {MedalCaseCompleted, "snapshot", 0, 101},
	} {
		if _, err := RankAchievements(args.kind, args.snapshot, nil, args.offset, args.limit); err == nil {
			t.Fatal("invalid query accepted")
		}
	}
}
