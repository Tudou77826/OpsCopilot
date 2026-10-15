package main

import (
	"fmt"
	"log/slog"
	"opscopilot/pkg/companion"
)

// Personal companion bindings share the local domain with the Teams sidecar.
func (a *App) CompanionSnapshot() (*companion.Snapshot, error) {
	if a.companion == nil {
		return nil, companion.ErrDisabled
	}
	return a.companion.ReadSnapshot()
}
func (a *App) CompanionHatch(revision uint64) (*companion.Snapshot, error) {
	if a.companion == nil {
		return nil, companion.ErrDisabled
	}
	snapshot, err := a.companion.Hatch(revision)
	if err == nil {
		a.queueCompanionCard(snapshot)
	}
	return snapshot, err
}
func (a *App) CompanionRename(name string, revision uint64) (*companion.Snapshot, error) {
	if a.companion == nil {
		return nil, companion.ErrDisabled
	}
	snapshot, err := a.companion.Rename(name, revision)
	if err == nil {
		a.queueCompanionCard(snapshot)
	}
	return snapshot, err
}
func (a *App) queueCompanionCard(snapshot *companion.Snapshot) {
	if a.achievements == nil || !a.achievements.Enabled() || snapshot == nil || snapshot.Pet == nil {
		return
	}
	card, err := snapshot.PublicCard()
	if err == nil {
		err = a.achievements.QueueCompanion(&card, snapshot.Revision)
	}
	if err != nil {
		slog.Warn("伙伴公开展示待重试", "error", err)
	}
}
func (a *App) recordCompanion(kind companion.EventKind, key string) {
	if a.companion == nil || !a.companion.Enabled() {
		return
	}
	a.achievementDay()
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("伙伴成长结算异常", "kind", kind, "error", fmt.Sprint(r))
			}
		}()
		if _, err := a.companion.Record(kind, key); err != nil {
			slog.Warn("伙伴成长未结算", "kind", kind, "error", err)
		} else if snapshot, err := a.companion.ReadSnapshot(); err == nil {
			a.queueCompanionCard(snapshot)
		}
	}()
}
