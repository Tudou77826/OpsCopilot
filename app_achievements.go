package main

import (
	"context"
	"fmt"
	"opscopilot/pkg/achievements"
	social "opscopilot/pkg/companionsocial"
	"opscopilot/pkg/config"
	"path/filepath"
	"strings"
	"time"
)

func (a *App) recordKnowledgeRead(root, relative, content string) {
	if a.achievements == nil || strings.TrimSpace(content) == "" {
		return
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return
	}
	ref := filepath.ToSlash(filepath.Join(abs, filepath.Clean(relative)))
	if filepath.VolumeName(abs) != "" {
		ref = strings.ToLower(ref)
	}
	_ = a.achievements.Record(social.MedalKnowledgeRead, "knowledge:"+ref, true)
}

func (a *App) DispatchQuickCommand(commandID string, sessionIDs []string) (achievements.DispatchResult, error) {
	var content string
	for _, cmd := range a.configMgr.Config.QuickCommands {
		if cmd.ID == commandID {
			content = cmd.Content
			break
		}
	}
	content = strings.TrimRight(content, "\r\n")
	if strings.TrimSpace(content) == "" {
		return achievements.DispatchResult{}, fmt.Errorf("快捷指令不存在或内容为空")
	}
	return achievements.Dispatch(a.achievements, commandID, sessionIDs, func(id string) error {
		if err := a.SendCommand(id, content); err != nil {
			return err
		}
		a.recordInput(id, content)
		return nil
	})
}

func (a *App) achievementContext() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}
func (a *App) AchievementStatus() (achievements.Status, error) {
	if a.achievements == nil {
		return achievements.Status{}, achievements.ErrDisabled
	}
	if err := a.recoverAchievements(); err != nil {
		return achievements.Status{}, err
	}
	return a.achievements.Status()
}
func (a *App) AchievementCatalog() []social.MedalDefinition { return social.MedalCatalog() }
func (a *App) SocialConnect(baseURL, credential string, claimLocal bool) (achievements.Status, error) {
	if a.social == nil {
		return achievements.Status{}, achievements.ErrDisabled
	}
	if err := a.recoverAchievements(); err != nil {
		return achievements.Status{}, err
	}
	return a.social.Connect(a.achievementContext(), baseURL, credential, claimLocal)
}
func (a *App) SocialParticipation(enabled bool) error {
	if a.social == nil {
		return achievements.ErrDisabled
	}
	return a.social.Participation(a.achievementContext(), enabled)
}
func (a *App) SocialSync() (achievements.Status, error) {
	if a.social == nil {
		return achievements.Status{}, achievements.ErrDisabled
	}
	if err := a.recoverAchievements(); err != nil {
		return achievements.Status{}, err
	}
	return a.social.Sync(a.achievementContext())
}
func (a *App) SocialRankings(kind social.MedalKind, offset, limit int, snapshotID string) (social.RankingPage, error) {
	if a.social == nil {
		return social.RankingPage{}, achievements.ErrDisabled
	}
	return a.social.Rankings(a.achievementContext(), kind, offset, limit, snapshotID)
}
func (a *App) SocialProfile(userID string) (social.PublicProfile, error) {
	if a.social == nil {
		return social.PublicProfile{}, achievements.ErrDisabled
	}
	return a.social.Profile(a.achievementContext(), userID)
}
func (a *App) SocialShowcase(selected []social.MedalKind) (achievements.Status, error) {
	if a.social == nil {
		return achievements.Status{}, achievements.ErrDisabled
	}
	if err := a.recoverAchievements(); err != nil {
		return achievements.Status{}, err
	}
	return a.social.ShowcaseSelection(a.achievementContext(), selected)
}
func (a *App) achievementDay() {
	if a.achievements != nil {
		_ = a.achievements.Record(social.MedalActiveDay, time.Now().UTC().Format("2006-01-02"), true)
	}
}
func (a *App) recoverAchievements() error {
	if a.achievements == nil || !a.achievements.Enabled() {
		return nil
	}
	if a.companion != nil {
		if snapshot, err := a.companion.ReadSnapshot(); err == nil {
			a.queueCompanionCard(snapshot)
		} else {
			return err
		}
	}
	return a.achievements.Repair(&achievements.Sources{
		Scripts: a.scriptMgr.StorageDirectory(), Recordings: a.coreRecorder.StorageDirectory(), Knowledge: a.resolveKnowledgeBase(),
		QuickCommands: func() (map[string]bool, error) {
			m := config.NewManagerWithDir(a.configMgr.Directory())
			m.SetReadOnly(true)
			if err := m.Load(); err != nil {
				return nil, err
			}
			items := map[string]bool{}
			for _, cmd := range m.Config.QuickCommands {
				items[cmd.ID] = strings.TrimSpace(cmd.Content) != ""
			}
			return items, nil
		},
	})
}
