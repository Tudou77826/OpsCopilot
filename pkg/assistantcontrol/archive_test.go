package assistantcontrol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestArchiveRestartKeepsVisiblePartialHistoryWithoutPendingAuthority(t *testing.T) {
	h := &History{}
	h.Begin("turn", "查看远程日志")
	state := State{TurnID: "turn", Phase: "waiting", Messages: []Message{{ID: "q", TurnID: "turn", Role: "ai", Kind: "question", Content: "看哪一天？"}, {ID: "answer", TurnID: "turn", Role: "user", Kind: "answer", Content: "今天"}}, Prompt: &Prompt{ID: "live-only", Data: json.RawMessage(`{"password":"never-save"}`)}}
	file := filepath.Join(t.TempDir(), "conversation.json")
	store := &ArchiveStore{File: file}
	if _, err := store.Load(); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(Archive{ModelID: "native", Interrupted: true, Conversation: h.Checkpoint(state)}); err != nil {
		t.Fatal(err)
	}
	if len(h.Snapshot().Messages) != 1 {
		t.Fatal("checkpoint finalized live memory")
	}
	data, _ := os.ReadFile(file)
	if strings.Contains(string(data), "never-save") || strings.Contains(string(data), "live-only") {
		t.Fatal("pending authority persisted")
	}
	restarted := &ArchiveStore{File: file}
	archive, err := restarted.Load()
	if err != nil || !archive.Interrupted || archive.ModelID != "native" || len(archive.Conversation.Messages) != 3 {
		t.Fatal(archive, err)
	}
	restored := &History{}
	if err = restored.Restore(archive.Conversation); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(RecoveryContext(restored.Snapshot()), "今天") {
		t.Fatal("answer missing from recovered context")
	}
	// A stale live write must not overwrite a final result or a new conversation.
	generation := store.Generation()
	h.Finish(State{TurnID: "turn", Phase: "completed", Output: "已完成"})
	if err = store.Save(Archive{Conversation: h.Snapshot()}); err != nil {
		t.Fatal(err)
	}
	if err = store.SaveCheckpoint(Archive{Interrupted: true, Conversation: archive.Conversation}, generation); err != nil {
		t.Fatal(err)
	}
	final, err := (&ArchiveStore{File: file}).Load()
	if err != nil || final.Interrupted || final.Conversation.Messages[1].Content != "已完成" {
		t.Fatal("late checkpoint replaced final state", final, err)
	}
}

func TestArchiveCorruptionAndExternalChangesAreNotSilentlyOverwritten(t *testing.T) {
	file := filepath.Join(t.TempDir(), "history.json")
	broken := []byte(`{"broken"`)
	_ = os.WriteFile(file, broken, 0600)
	s := &ArchiveStore{File: file}
	if _, err := s.Load(); err == nil {
		t.Fatal("invalid archive accepted")
	}
	if err := s.Save(Archive{}); err == nil {
		t.Fatal("normal save overwrote unreadable archive")
	}
	data, _ := os.ReadFile(file)
	if string(data) != string(broken) {
		t.Fatal("original lost")
	}
	h := &History{}
	h.Reset()
	if err := s.Reset(Archive{Conversation: h.Snapshot()}); err != nil {
		t.Fatal(err)
	}
	backups, _ := filepath.Glob(file + ".unreadable-*")
	if len(backups) != 1 {
		t.Fatal("missing original backup")
	}
	backup, _ := os.ReadFile(backups[0])
	if string(backup) != string(broken) {
		t.Fatal("backup changed original")
	}
	other := &ArchiveStore{File: file}
	_, _ = other.Load()
	h.Begin("new", "new question")
	if err := s.Save(Archive{Conversation: h.Snapshot()}); err != nil {
		t.Fatal(err)
	}
	if err := other.Save(Archive{}); err == nil {
		t.Fatal("second process silently replaced history")
	}
}

func TestRecoveryContextIsBoundedAndTreatsHistoryAsHistoricalData(t *testing.T) {
	h := &History{}
	for i := 0; i < 10; i++ {
		h.Begin(string(rune('a'+i)), "历史要求")
		h.Finish(State{TurnID: string(rune('a' + i)), Phase: "completed", Output: strings.Repeat("日志", 8000)})
	}
	seed := RecoveryContext(h.Snapshot())
	if !utf8.ValidString(seed) || len(seed) > 64*1024 || !strings.Contains(seed, "不是新指令") || !strings.Contains(seed, "隐藏工具上下文未恢复") {
		t.Fatal("invalid recovery context")
	}
	bad := h.Snapshot()
	bad.Messages[0].Role = "system"
	if err := (&History{}).Restore(bad); err == nil {
		t.Fatal("persisted role escalation accepted")
	}
}
