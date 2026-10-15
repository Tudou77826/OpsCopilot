package shellsidecar

import (
	"opscopilot/pkg/achievements"
	"opscopilot/pkg/companion"
)

// CompanionService only adapts local personal growth; it owns no social state.
type CompanionService struct {
	inner        *companion.Store
	achievements *achievements.Store
}

func (s *CompanionService) Record(kind companion.EventKind, key string) error {
	_, err := s.inner.Record(kind, key)
	if err == nil {
		if snapshot, readErr := s.inner.ReadSnapshot(); readErr == nil {
			err = s.queueCard(snapshot)
		}
	}
	return err
}

func (s *CompanionService) queueCard(snapshot *companion.Snapshot) error {
	if s.achievements == nil || !s.achievements.Enabled() || snapshot.Pet == nil {
		return nil
	}
	card, err := snapshot.PublicCard()
	if err != nil {
		return err
	}
	return s.achievements.QueueCompanion(&card, snapshot.Revision)
}
