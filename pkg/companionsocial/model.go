// Package companionsocial defines account-bound profiles, medals and rankings. Persistence,
// authentication and transport must be supplied by a social server, not a
// desktop installation ID, telemetry service or Git author name.
package companionsocial

import (
	"opscopilot/pkg/companion"
	"regexp"
)

type UserID string

var stableID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

type Profile struct {
	UserID      UserID          `json:"userId"`
	DisplayName string          `json:"displayName"`
	Companion   *companion.Card `json:"companion,omitempty"`
	Medals      []Medal         `json:"medals"`
}

type PublicProfile struct {
	Profile
	Achievements AchievementSummary `json:"achievements"`
}
