package main

import (
	"fmt"
	"path/filepath"
	"time"

	"opscopilot/pkg/agenttransport/acp"
	"opscopilot/pkg/assistantcontrol"
)

func (a *App) ensureAssistantHistoryLocked() error {
	if a.assistantHistoryLoaded {
		return a.assistantHistoryLoadError
	}
	a.assistantHistoryLoaded = true
	if a.configMgr == nil {
		return nil
	}
	a.assistantArchive = &assistantcontrol.ArchiveStore{File: filepath.Join(a.configMgr.Directory(), "assistant-conversation.json")}
	archive, err := a.assistantArchive.Load()
	if err != nil {
		a.assistantHistoryLoadError = err
		return err
	}
	if archive.Version == 0 {
		return nil
	}
	if err = a.assistantHistory.Restore(archive.Conversation); err != nil {
		a.assistantHistoryLoadError = err
		return err
	}
	a.assistantSelectedModel = acp.LocalModel{ID: archive.ModelID}
	if len(archive.Conversation.Messages) > 0 {
		a.assistantRecovered = true
		a.assistantRecoveryNote = "已恢复本地对话记录。助手仅参考近期可见历史；当前连接和操作结果会重新核实，上次任务不会自动继续。"
	}
	if archive.Interrupted {
		a.assistantHistory.Note(archive.Conversation.ID+":interrupted", "上次任务在程序退出前未结束。已保存的输出可能不完整，未完成的操作、提问和授权均已结束；不会自动续跑，请重新说明需要继续的内容。")
		_ = a.saveAssistantHistoryLocked(false, nil)
	}
	return nil
}

func (a *App) assistantSnapshotLocked() assistantcontrol.ConversationSnapshot {
	s := a.assistantHistory.Snapshot()
	s.RecoveryNote = a.assistantRecoveryNote
	if a.assistantSaveWarning != "" {
		s.RecoveryNote = a.assistantSaveWarning
	}
	return s
}

func (a *App) saveAssistantHistoryLocked(interrupted bool, state *assistantcontrol.State) error {
	if a.assistantArchive == nil {
		return nil
	}
	s := a.assistantHistory.Snapshot()
	if state != nil {
		s = a.assistantHistory.Checkpoint(*state)
	}
	err := a.assistantArchive.Save(assistantcontrol.Archive{ModelID: a.assistantSelectedModel.ID, Interrupted: interrupted, Conversation: s})
	if err != nil {
		a.assistantSaveWarning = "对话记录尚未保存，退出后可能无法恢复：" + err.Error()
	} else {
		a.assistantSaveWarning = ""
	}
	return err
}

func (a *App) checkpointAssistant(turn *assistantcontrol.Turn, stop <-chan struct{}) {
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for {
		select {
		case <-stop:
			return
		case <-timer.C:
		}
		a.assistantConversationMu.Lock()
		state := turn.Snapshot()
		if !a.assistantRunning || a.assistantCoordinator().Snapshot().TurnID != turn.ID() {
			a.assistantConversationMu.Unlock()
			return
		}
		store := a.assistantArchive
		if store == nil {
			a.assistantConversationMu.Unlock()
			return
		}
		archive := assistantcontrol.Archive{ModelID: a.assistantSelectedModel.ID, Interrupted: true, Conversation: a.assistantHistory.Checkpoint(state)}
		generation := store.Generation()
		a.assistantConversationMu.Unlock()
		err := store.SaveCheckpoint(archive, generation)
		a.assistantConversationMu.Lock()
		if a.assistantRunning && a.assistantCoordinator().Snapshot().TurnID == turn.ID() {
			if err != nil {
				a.assistantSaveWarning = "对话记录尚未保存，退出后可能无法恢复：" + err.Error()
			} else {
				a.assistantSaveWarning = ""
			}
		}
		a.assistantConversationMu.Unlock()
	}
}

func (a *App) resetAssistantHistoryLocked() error {
	if a.assistantArchive == nil {
		a.assistantHistory.Reset()
		return nil
	}
	// Save the empty replacement before clearing the visible/model conversation.
	next := &assistantcontrol.History{}
	next.Reset()
	if err := a.assistantArchive.Reset(assistantcontrol.Archive{ModelID: a.assistantSelectedModel.ID, Conversation: next.Snapshot()}); err != nil {
		return fmt.Errorf("新建对话未完成: %w", err)
	}
	a.assistantHistoryLoadError = nil
	a.assistantSaveWarning = ""
	_ = a.assistantHistory.Restore(next.Snapshot())
	return nil
}
