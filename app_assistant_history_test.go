package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"opscopilot/pkg/agenttransport/acp"
	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/config"
	"opscopilot/pkg/filetxn"
)

func TestAssistantRestartRestoresHistoryAndModelButNeverCoordinatorOrQueue(t *testing.T) {
	dir := t.TempDir()
	mgr := config.NewManagerWithDir(dir)
	if err := mgr.Load(); err != nil {
		t.Fatal(err)
	}
	a := &App{ctx: context.Background(), configMgr: mgr}
	a.assistantConversationMu.Lock()
	if err := a.ensureAssistantHistoryLocked(); err != nil {
		t.Fatal(err)
	}
	a.assistantSelectedModel = acp.LocalModel{ID: "chosen", Environment: `{"secret":"never-save"}`, Profile: `{"api_key":"never-save"}`}
	a.assistantHistory.Begin("old", "检查业务日志")
	state := assistantcontrol.State{TurnID: "old", Phase: "waiting", Messages: []assistantcontrol.Message{{ID: "old:text", TurnID: "old", Role: "ai", Kind: "text", Content: "已找到今天的日志"}}}
	if err := a.saveAssistantHistoryLocked(true, &state); err != nil {
		t.Fatal(err)
	}
	a.assistantConversationMu.Unlock()
	b := &App{ctx: context.Background(), configMgr: config.NewManagerWithDir(dir)}
	snapshot, err := b.RestoreAssistantConversation()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Messages) != 3 || snapshot.Messages[1].Content != "已找到今天的日志" || !strings.Contains(snapshot.RecoveryNote, "不会自动继续") {
		t.Fatal(snapshot)
	}
	if b.assistantSelectedModel.ID != "chosen" || b.assistantSelectedModel.Profile != "" || b.assistantConversation != nil || b.AssistantControlState().Phase != "idle" {
		t.Fatal("recovery restored runtime or authority")
	}
	archive, err := (&assistantcontrol.ArchiveStore{File: filepath.Join(dir, "assistant-conversation.json")}).Load()
	if err != nil || archive.Interrupted {
		t.Fatal("interruption was not acknowledged", err)
	}
	if err = b.NewAssistantConversation(); err != nil {
		t.Fatal(err)
	}
	c := &App{ctx: context.Background(), configMgr: config.NewManagerWithDir(dir)}
	cleared, err := c.RestoreAssistantConversation()
	if err != nil || len(cleared.Messages) != 0 || cleared.RecoveryNote != "" || c.assistantSelectedModel.ID != "chosen" {
		t.Fatal("new chat was not persisted", cleared, err)
	}
}

func TestHistoryDiskCheckpointDoesNotBlockManualStop(t *testing.T) {
	a := newLifecycleApp()
	a.configMgr = config.NewManagerWithDir(t.TempDir())
	if err := a.configMgr.Load(); err != nil {
		t.Fatal(err)
	}
	a.assistantConversationMu.Lock()
	if err := a.ensureAssistantHistoryLocked(); err != nil {
		t.Fatal(err)
	}
	turn, _ := a.assistantCoordinator().Begin(context.Background(), assistantcontrol.Target{}, nil)
	a.assistantHistory.Begin(turn.ID(), "test question")
	a.assistantRunning = true
	if err := a.saveAssistantHistoryLocked(true, nil); err != nil {
		t.Fatal(err)
	}
	generation := a.assistantArchive.Generation()
	a.assistantConversationMu.Unlock()
	release, err := filetxn.Lock(a.assistantArchive.File)
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() { defer close(done); a.checkpointAssistant(turn, stop) }()
	defer func() {
		close(stop)
		release()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("checkpoint did not exit")
		}
	}()
	deadline := time.Now().Add(3 * time.Second)
	for a.assistantArchive.Generation() == generation && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if a.assistantArchive.Generation() == generation {
		t.Fatal("checkpoint did not start")
	}
	stopped := make(chan error, 1)
	go func() { stopped <- a.StopAssistantTurn(turn.ID()) }()
	select {
	case err = <-stopped:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("disk I/O blocked stop")
	}
}
