// Package companion owns one local companion and bounded growth from verified
// work results. It has no rendering, account, friendship or network dependency.
package companion

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const SchemaVersion = 1
const RuleVersion = 1
const DailyXPMax = 12
const DailyKindMax = 2
const SpeciesID = "lumen"
const MaxLevel = 100
const XPPerLevel = 10
const RehatchCost = 50

var ErrDisabled = errors.New("伙伴养成功能未启用")
var ErrInsufficientXP = errors.New("重新开蛋需要 50 点经验")

// LevelOf derives display progression from unspent growth XP, without a second stored counter.
func LevelOf(xp int) int {
	if xp < 0 {
		return 1
	}
	if xp >= (MaxLevel-1)*XPPerLevel {
		return MaxLevel
	}
	return xp/XPPerLevel + 1
}

type EventKind string

const (
	EventSessionEstablished EventKind = "session-established"
	EventTransferCompleted  EventKind = "transfer-completed"
	// Replay is a dispatch result, not a claim that remote execution succeeded.
	EventScriptReplayDone EventKind = "script-replay-dispatched"
)

func reward(kind EventKind) int {
	switch kind {
	case EventSessionEstablished:
		return 2
	case EventTransferCompleted:
		return 3
	case EventScriptReplayDone:
		return 4
	}
	return 0
}

type Pet struct {
	ID        string    `json:"id"`
	SpeciesID string    `json:"speciesId"`
	Name      string    `json:"name"`
	PaletteID string    `json:"paletteId"`
	XP        int       `json:"xp"`
	AdoptedAt time.Time `json:"adoptedAt"`
}

// Snapshot excludes internal event hashes and daily settlement buckets.
type Snapshot struct {
	SchemaVersion   int    `json:"schemaVersion"`
	RuleVersion     int    `json:"ruleVersion"`
	Revision        uint64 `json:"revision"`
	Pet             *Pet   `json:"pet"`
	Level           int    `json:"level"`
	LevelXP         int    `json:"levelXP"`
	LevelXPRequired int    `json:"levelXPRequired"`
	TodayXP         int    `json:"todayXP"`
	DailyXPMax      int    `json:"dailyXPMax"`
	HatchCost       int    `json:"hatchCost"`
	CanHatch        bool   `json:"canHatch"`
}

// Card is the allowlisted payload for an explicitly enabled social profile.
// It excludes XP, private pet ID, activity times and settlement state. User ID
// and profile visibility belong to the authenticated social service.
type Card struct {
	SpeciesID string `json:"speciesId"`
	Name      string `json:"name"`
	PaletteID string `json:"paletteId"`
	Level     int    `json:"level,omitempty"`
	// Read compatibility for already stored cards; new projections never emit stages.
	Stage string `json:"stage,omitempty"`
}

func (c Card) Valid() bool {
	legacy := c.Stage == "child" || c.Stage == "growing" || c.Stage == "adult"
	return validName(c.Name) && validAppearance(c.SpeciesID, c.PaletteID) && (c.Stage == "" || legacy) && ((c.Level >= 1 && c.Level <= MaxLevel) || (c.Level == 0 && legacy))
}

func (s Snapshot) PublicCard() (Card, error) {
	if s.Pet == nil {
		return Card{}, fmt.Errorf("尚未领养伙伴")
	}
	return Card{SpeciesID: s.Pet.SpeciesID, Name: s.Pet.Name, PaletteID: s.Pet.PaletteID, Level: LevelOf(s.Pet.XP)}, nil
}

func validAppearance(species, palette string) bool {
	_, known := FindSpecies(species)
	return known && (palette == "mint" || palette == "amber" || palette == "lavender")
}
func validName(name string) bool {
	if strings.TrimSpace(name) != name || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 24 {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

type Outcome struct {
	XPGained     int  `json:"xpGained"`
	Duplicate    bool `json:"duplicate"`
	Capped       bool `json:"capped"`
	LevelChanged bool `json:"levelChanged"`
}
