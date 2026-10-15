package companionsocial

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func receipt(kind MedalKind, id string, revision uint64, active bool) AchievementReceipt {
	return AchievementReceipt{Kind: kind, ObjectID: id, Contributor: "alice", Revision: revision, Active: active}
}

func TestAchievementProjectionDeduplicatesRevisionsAndWithdrawals(t *testing.T) {
	input := []AchievementReceipt{
		receipt(MedalKnowledgeArchive, "private-document", 2, false),
		receipt(MedalKnowledgeArchive, "private-document", 1, true),
		receipt(MedalKnowledgeArchive, "other-document", 1, true),
		receipt(MedalKnowledgeArchive, "other-document", 1, true),
		receipt(MedalKnowledgeArchive, "other-document", 2, true),
		receipt(MedalScriptCreated, "other-document", 1, true),
	}
	first, err := ProjectAchievements("alice", input)
	if err != nil {
		t.Fatal(err)
	}
	for i, j := 0, len(input)-1; i < j; i, j = i+1, j-1 {
		input[i], input[j] = input[j], input[i]
	}
	second, err := ProjectAchievements("alice", input)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(first)
	b, _ := json.Marshal(second)
	if string(a) != string(b) || first.Progress[0].Count != 1 || first.Progress[1].Count != 1 {
		t.Fatalf("arrival order or retries changed counts: %s / %s", a, b)
	}
}

func TestAchievementMedalBoundaries(t *testing.T) {
	for _, tc := range []struct {
		count int
		tier  MedalTier
		next  int
	}{{0, "", 1}, {1, MedalBronze, 10}, {9, MedalBronze, 10}, {10, MedalSilver, 50}, {49, MedalSilver, 50}, {50, MedalGold, 0}, {51, MedalGold, 0}} {
		t.Run(fmt.Sprint(tc.count), func(t *testing.T) {
			input := make([]AchievementReceipt, tc.count)
			for i := range input {
				input[i] = receipt(MedalKnowledgeArchive, fmt.Sprintf("archive-%d", i), 1, true)
			}
			summary, err := ProjectAchievements("alice", input)
			if err != nil {
				t.Fatal(err)
			}
			progress := summary.Progress[0]
			if progress.Count != tc.count || progress.NextThreshold != tc.next || summary.RuleVersion != 1 {
				t.Fatalf("wrong progress: %+v", progress)
			}
			if tc.tier == "" {
				if progress.Medal != nil {
					t.Fatal("zero contributions earned a medal")
				}
			} else if progress.Medal == nil || progress.Medal.Tier != tc.tier || progress.Medal.Count != tc.count {
				t.Fatalf("wrong medal: %+v", progress.Medal)
			}
		})
	}
}

func TestAchievementProjectionRejectsInvalidEvidenceAndConflicts(t *testing.T) {
	base := receipt(MedalKnowledgeArchive, "archive-1", 1, true)
	for _, tc := range []struct {
		name string
		edit func(*AchievementReceipt)
	}{
		{"unknown feature", func(r *AchievementReceipt) { r.Kind = "script-replay" }},
		{"wrong owner", func(r *AchievementReceipt) { r.Contributor = "bob" }},
		{"missing object", func(r *AchievementReceipt) { r.ObjectID = "" }},
		{"zero revision", func(r *AchievementReceipt) { r.Revision = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invalid := base
			tc.edit(&invalid)
			if _, err := ProjectAchievements("alice", []AchievementReceipt{invalid}); err == nil {
				t.Fatal("invalid evidence accepted")
			}
		})
	}
	if _, err := ProjectAchievements("", nil); err == nil {
		t.Fatal("missing owner accepted")
	}
	conflict := base
	conflict.Active = false
	// Conflicting old revisions must fail even if a newer revision arrived first.
	for _, input := range [][]AchievementReceipt{{base, conflict}, {receipt(MedalKnowledgeArchive, "archive-1", 2, true), base, conflict}} {
		if _, err := ProjectAchievements("alice", input); err == nil {
			t.Fatal("conflicting receipt accepted")
		}
	}
}

func TestMedalShowcaseIsExplicitOrderedAndAggregateOnly(t *testing.T) {
	summary, err := ProjectAchievements("alice", []AchievementReceipt{
		receipt(MedalKnowledgeArchive, "private-archive-id", 1, true),
		receipt(MedalScriptCreated, "private-script-id", 1, true),
	})
	if err != nil {
		t.Fatal(err)
	}
	empty, err := summary.Showcase(nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("medals published without selection: %v / %v", empty, err)
	}
	medals, err := summary.Showcase([]MedalKind{MedalScriptCreated, MedalKnowledgeArchive})
	if err != nil || len(medals) != 2 || medals[0].Kind != MedalScriptCreated {
		t.Fatalf("selection order lost: %v / %v", medals, err)
	}
	payload, _ := json.Marshal(Profile{UserID: "alice", DisplayName: "Alice", Medals: medals})
	for _, private := range []string{"private-archive-id", "private-script-id", "Contributor", "Revision", "Active", "receipts"} {
		if strings.Contains(string(payload), private) {
			t.Fatalf("private evidence exposed: %s", payload)
		}
	}
	for _, selection := range [][]MedalKind{{"unknown"}, {MedalScriptCreated, MedalScriptCreated}, {MedalScriptCreated, MedalKnowledgeArchive, MedalScriptCreated, MedalKnowledgeArchive}} {
		if _, err := summary.Showcase(selection); err == nil {
			t.Fatal("invalid showcase selection accepted")
		}
	}
	medals[0].Count = 999
	if summary.Progress[1].Medal.Count != 1 {
		t.Fatal("public value mutated private summary")
	}
}

func TestWithdrawalRemovesPublicMedalAndRepublicationCountsOnce(t *testing.T) {
	input := []AchievementReceipt{receipt(MedalKnowledgeArchive, "archive-1", 1, true), receipt(MedalKnowledgeArchive, "archive-1", 2, false)}
	summary, err := ProjectAchievements("alice", input)
	if err != nil {
		t.Fatal(err)
	}
	medals, err := summary.Showcase([]MedalKind{MedalKnowledgeArchive})
	if err != nil || len(medals) != 0 {
		t.Fatalf("withdrawn contribution visible: %v / %v", medals, err)
	}
	input = append(input, receipt(MedalKnowledgeArchive, "archive-1", 3, true))
	summary, err = ProjectAchievements("alice", input)
	if err != nil || summary.Progress[0].Count != 1 {
		t.Fatalf("republication overcounted: %+v / %v", summary, err)
	}
}

func TestAllMedalThresholdsAndCatalogIsolation(t *testing.T) {
	catalog := MedalCatalog()
	if len(catalog) != 8 {
		t.Fatalf("unexpected categories: %d", len(catalog))
	}
	for _, definition := range catalog {
		for level, threshold := range definition.Thresholds {
			input := make([]AchievementReceipt, threshold)
			for i := range input {
				id := fmt.Sprintf("object-%d", i)
				if definition.Kind == MedalActiveDay {
					id = fmt.Sprintf("2026-%02d-%02d", i/28+1, i%28+1)
				}
				input[i] = receipt(definition.Kind, id, 1, true)
			}
			summary, err := ProjectAchievements("alice", input)
			if err != nil {
				t.Fatal(err)
			}
			for _, progress := range summary.Progress {
				if progress.Kind == definition.Kind && (progress.Medal == nil || progress.Medal.Tier != []MedalTier{MedalBronze, MedalSilver, MedalGold}[level]) {
					t.Fatalf("wrong threshold for %s: %+v", definition.Kind, progress)
				}
			}
		}
	}
	catalog[0].Thresholds[0] = 999
	if MedalCatalog()[0].Thresholds[0] != 1 {
		t.Fatal("catalog mutated global rules")
	}
	if _, err := ProjectAchievements("alice", []AchievementReceipt{receipt(MedalActiveDay, "2026-02-30", 1, true)}); err == nil {
		t.Fatal("invalid day accepted")
	}
}
