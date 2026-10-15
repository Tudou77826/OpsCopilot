package companionsocial

import (
	"fmt"
	"time"
)

const AchievementRuleVersion = 1
const MaxShowcaseMedals = 3

type MedalKind string

const (
	MedalKnowledgeArchive    MedalKind = "knowledge-archive"
	MedalScriptCreated       MedalKind = "script-created"
	MedalQuickCommandCreated MedalKind = "quick-command-created"
	MedalCaseCompleted       MedalKind = "case-completed"
	MedalKnowledgeRead       MedalKind = "knowledge-read"
	MedalScriptUsed          MedalKind = "script-used"
	MedalQuickCommandUsed    MedalKind = "quick-command-used"
	MedalActiveDay           MedalKind = "active-day"
)

type MedalTier string

const (
	MedalBronze MedalTier = "bronze"
	MedalSilver MedalTier = "silver"
	MedalGold   MedalTier = "gold"
)

// AchievementReceipt is a private, account-bound business confirmation. The
// application service validates eligibility and origin; this is not a public
// request or proof of authentication. ObjectID is the stable counted object,
// never a save/retry ID. For active-day it is the server-approved UTC date.
// Inactive tombstones prevent a delayed old notification restoring a count.
type AchievementReceipt struct {
	Kind        MedalKind `json:"kind"`
	ObjectID    string    `json:"objectId"`
	Contributor UserID    `json:"contributor"`
	Revision    uint64    `json:"revision"`
	Active      bool      `json:"active"`
}

// Medal exposes aggregate facts only, never artifact identifiers,
// contents, terminal output or publication receipts.
type Medal struct {
	Kind  MedalKind `json:"kind"`
	Tier  MedalTier `json:"tier"`
	Count int       `json:"count"`
}

type AchievementProgress struct {
	Kind          MedalKind `json:"kind"`
	Count         int       `json:"count"`
	Medal         *Medal    `json:"medal,omitempty"`
	NextThreshold int       `json:"nextThreshold,omitempty"`
}

type AchievementSummary struct {
	RuleVersion int                   `json:"ruleVersion"`
	Progress    []AchievementProgress `json:"progress"`
}

type MedalDefinition struct {
	Kind       MedalKind `json:"kind"`
	Name       string    `json:"name"`
	Unit       string    `json:"unit"`
	Group      string    `json:"group"`
	Thresholds [3]int    `json:"thresholds"`
}

var medalDefinitions = [...]MedalDefinition{
	{MedalKnowledgeArchive, "知识沉淀者", "篇", "creation", [3]int{1, 10, 50}},
	{MedalScriptCreated, "脚本工匠", "个", "creation", [3]int{1, 10, 30}},
	{MedalQuickCommandCreated, "指令工匠", "条", "creation", [3]int{1, 10, 30}},
	{MedalCaseCompleted, "排查记录者", "次", "practice", [3]int{1, 10, 50}},
	{MedalKnowledgeRead, "知识探索者", "篇", "practice", [3]int{5, 20, 100}},
	{MedalScriptUsed, "自动化实践者", "个", "practice", [3]int{3, 10, 30}},
	{MedalQuickCommandUsed, "快捷指令实践者", "条", "practice", [3]int{3, 10, 30}},
	{MedalActiveDay, "持续实践者", "天", "consistency", [3]int{7, 30, 90}},
}

// MedalCatalog returns a copy, so callers cannot alter rule thresholds.
func MedalCatalog() []MedalDefinition {
	return append([]MedalDefinition(nil), medalDefinitions[:]...)
}

func (k MedalKind) valid() bool {
	for _, definition := range medalDefinitions {
		if definition.Kind == k {
			return true
		}
	}
	return false
}

func (k MedalKind) Valid() bool { return k.valid() }
func ValidID(id string) bool    { return stableID.MatchString(id) }
func (k MedalKind) Creation() bool {
	return k == MedalKnowledgeArchive || k == MedalScriptCreated || k == MedalQuickCommandCreated
}

// ProjectAchievements counts latest active versions of distinct eligible
// objects. The service must authenticate ownership, validate business results
// and read confirmations consistently. Local feature usage is not proof of
// shared publication, remote command success or the user's work quality.
func ProjectAchievements(owner UserID, receipts []AchievementReceipt) (AchievementSummary, error) {
	if !stableID.MatchString(string(owner)) {
		return AchievementSummary{}, fmt.Errorf("成员身份无效")
	}
	type objectKey struct {
		kind MedalKind
		id   string
	}
	type versionKey struct {
		objectKey
		revision uint64
	}
	latest := make(map[objectKey]AchievementReceipt)
	versions := make(map[versionKey]AchievementReceipt)
	for _, receipt := range receipts {
		if !receipt.Kind.valid() || !stableID.MatchString(receipt.ObjectID) || receipt.Revision == 0 || receipt.Contributor != owner {
			return AchievementSummary{}, fmt.Errorf("奖章确认记录无效")
		}
		if receipt.Kind == MedalActiveDay {
			if _, err := time.Parse("2006-01-02", receipt.ObjectID); err != nil {
				return AchievementSummary{}, fmt.Errorf("活跃日期无效: %w", err)
			}
		}
		key := objectKey{receipt.Kind, receipt.ObjectID}
		version := versionKey{key, receipt.Revision}
		if previous, ok := versions[version]; ok && previous != receipt {
			return AchievementSummary{}, fmt.Errorf("同一奖章确认版本存在冲突")
		}
		versions[version] = receipt
		if previous, ok := latest[key]; !ok || previous.Revision < receipt.Revision {
			latest[key] = receipt
		}
	}
	counts := make(map[MedalKind]int)
	for _, receipt := range latest {
		if receipt.Active {
			counts[receipt.Kind]++
		}
	}
	summary := AchievementSummary{RuleVersion: AchievementRuleVersion, Progress: make([]AchievementProgress, 0, len(medalDefinitions))}
	for _, definition := range medalDefinitions {
		kind := definition.Kind
		count := counts[kind]
		progress := AchievementProgress{Kind: kind, Count: count, NextThreshold: definition.Thresholds[0]}
		var tier MedalTier
		switch {
		case count >= definition.Thresholds[2]:
			tier, progress.NextThreshold = MedalGold, 0
		case count >= definition.Thresholds[1]:
			tier, progress.NextThreshold = MedalSilver, definition.Thresholds[2]
		case count >= definition.Thresholds[0]:
			tier, progress.NextThreshold = MedalBronze, definition.Thresholds[1]
		}
		if tier != "" {
			progress.Medal = &Medal{Kind: kind, Tier: tier, Count: count}
		}
		summary.Progress = append(summary.Progress, progress)
	}
	return summary, nil
}

// Showcase projects an explicit ordered selection. No selection publishes no
// medals. Withdrawn/unearned categories disappear while the stored selection
// may remain for a later re-earned medal. Invalid selections fail as a whole.
func (s AchievementSummary) Showcase(selected []MedalKind) ([]Medal, error) {
	if len(selected) > MaxShowcaseMedals {
		return nil, fmt.Errorf("最多展示三枚奖章")
	}
	medals := make([]Medal, 0, len(selected))
	seen := make(map[MedalKind]bool)
	for _, kind := range selected {
		if !kind.valid() || seen[kind] {
			return nil, fmt.Errorf("展示奖章类型无效或重复")
		}
		seen[kind] = true
		for _, progress := range s.Progress {
			if progress.Kind == kind && progress.Medal != nil {
				medals = append(medals, *progress.Medal)
				break
			}
		}
	}
	return medals, nil
}
