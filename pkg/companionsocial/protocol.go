package companionsocial

import (
	"opscopilot/pkg/companion"
	"time"
)

type Member struct {
	UserID         UserID `json:"userId"`
	OrganizationID string `json:"organizationId"`
	DisplayName    string `json:"displayName"`
	Participating  bool   `json:"participating"`
}
type SyncBatch struct {
	SourceID string               `json:"sourceId"`
	Receipts []AchievementReceipt `json:"receipts"`
}
type SyncAck struct {
	Receipts []AchievementReceipt `json:"receipts"`
	Summary  AchievementSummary   `json:"summary"`
}
type ProfileUpdate struct {
	Selected  []MedalKind     `json:"selected"`
	Companion *companion.Card `json:"companion,omitempty"`
	Revision  uint64          `json:"revision"`
	SourceID  string          `json:"sourceId"`
}
type Snapshot struct {
	OrganizationID string
	Epoch          uint64
	ExpiresAt      time.Time
	Kind           MedalKind
	Candidates     []RankingCandidate
}
