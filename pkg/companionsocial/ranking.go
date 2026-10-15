package companionsocial

import (
	"fmt"
	"sort"
	"strings"
)

// RankingCandidate is supplied by the account service from a consistent
// scope/snapshot, after checking membership and explicit public participation.
// Counts must come from ProjectAchievements, never a client-submitted score.
type RankingCandidate struct {
	Profile       Profile
	Achievements  AchievementSummary
	Participating bool
}

type RankingEntry struct {
	Rank    int     `json:"rank"`
	Count   int     `json:"count"`
	Profile Profile `json:"profile"`
}

type RankingPage struct {
	Kind        MedalKind      `json:"kind"`
	RuleVersion int            `json:"ruleVersion"`
	SnapshotID  string         `json:"snapshotId"`
	Offset      int            `json:"offset"`
	Total       int            `json:"total"`
	Entries     []RankingEntry `json:"entries"`
}

// RankAchievements compares one category only. Equal counts share competition
// ranks (1,2,2,4); stable user IDs break display ties without breaking the rank.
// Zero counts and nonparticipants are excluded. Pagination follows ranking,
// so a page boundary does not reset ranks. Snapshot retention/access checks
// remain application-service responsibilities, not guarantees of this rule.
func RankAchievements(kind MedalKind, snapshotID string, candidates []RankingCandidate, offset, limit int) (RankingPage, error) {
	if !kind.valid() || !stableID.MatchString(snapshotID) || offset < 0 || limit < 1 || limit > 100 {
		return RankingPage{}, fmt.Errorf("排名查询参数无效")
	}
	entries := make([]RankingEntry, 0, len(candidates))
	seen := make(map[UserID]bool)
	for _, candidate := range candidates {
		if !candidate.Participating {
			continue
		}
		id := candidate.Profile.UserID
		if !stableID.MatchString(string(id)) || strings.TrimSpace(candidate.Profile.DisplayName) == "" || seen[id] || candidate.Achievements.RuleVersion != AchievementRuleVersion {
			return RankingPage{}, fmt.Errorf("排名成员记录无效或重复")
		}
		seen[id] = true
		found := false
		count := 0
		for _, progress := range candidate.Achievements.Progress {
			if progress.Kind != kind {
				continue
			}
			if found || progress.Count < 0 {
				return RankingPage{}, fmt.Errorf("排名计数无效")
			}
			found, count = true, progress.Count
		}
		if !found {
			return RankingPage{}, fmt.Errorf("排名类别统计缺失")
		}
		if count > 0 {
			profile := candidate.Profile
			profile.Medals = append([]Medal{}, profile.Medals...)
			if profile.Companion != nil {
				card := *profile.Companion
				profile.Companion = &card
			}
			entries = append(entries, RankingEntry{Count: count, Profile: profile})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Count != entries[j].Count {
			return entries[i].Count > entries[j].Count
		}
		return entries[i].Profile.UserID < entries[j].Profile.UserID
	})
	for i := range entries {
		if i > 0 && entries[i].Count == entries[i-1].Count {
			entries[i].Rank = entries[i-1].Rank
		} else {
			entries[i].Rank = i + 1
		}
	}
	page := RankingPage{Kind: kind, RuleVersion: AchievementRuleVersion, SnapshotID: snapshotID, Offset: offset, Total: len(entries), Entries: []RankingEntry{}}
	if offset >= len(entries) {
		return page, nil
	}
	end := len(entries)
	if limit < len(entries)-offset {
		end = offset + limit
	}
	page.Entries = entries[offset:end]
	return page, nil
}
