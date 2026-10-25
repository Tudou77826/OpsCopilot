package assistantcontrol

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestOperationReceiptUpdatesWithoutAliasingAndDoesNotRestoreAuthority(t *testing.T) {
	c := &Coordinator{}
	turn, _ := c.Begin(context.Background(), Target{}, nil)
	op := Operation{Kind: "file", WorkspaceID: "old-workspace", ItemID: "task", Status: "running", Detail: "private-native-handle", Action: "sample.log → local.log", Result: "7 字节"}
	turn.ObserveOperation("task", "正在传输", op)
	before := c.Snapshot()
	op.Status = "done"
	turn.ObserveOperation("task", "传输完成", op)
	after := c.Snapshot()
	if len(after.Messages) != 1 || before.Messages[0].Operation.Status != "running" || after.Messages[0].Operation.Status != "done" {
		t.Fatal("receipt alias or duplicate", before, after)
	}
	turn.Finish("completed", "")
	turn.ObserveOperation("task", "late mutation", Operation{})
	h := &History{}
	h.Begin(turn.ID(), "下载测试文件")
	h.Finish(c.Snapshot())
	store := &ArchiveStore{File: filepath.Join(t.TempDir(), "history.json")}
	_, _ = store.Load()
	if err := store.Save(Archive{Conversation: h.Snapshot()}); err != nil {
		t.Fatal(err)
	}
	archive, err := (&ArchiveStore{File: store.File}).Load()
	if err != nil || archive.Conversation.Messages[1].Operation.ItemID != "task" {
		t.Fatal(archive, err)
	}
	seed := RecoveryContext(archive.Conversation)
	if strings.Contains(seed, "old-workspace") || strings.Contains(seed, "private-native-handle") || !strings.Contains(seed, "传输完成") {
		t.Fatal("native receipt leaked into recovered context", seed)
	}
	if !strings.Contains(seed, "sample.log") || !strings.Contains(seed, "7 字节") {
		t.Fatal("visible execution evidence lost on recovery")
	}
}

func TestStoppedOperationDoesNotClaimUnsubmittedOrCompleted(t *testing.T) {
	c := &Coordinator{}
	turn, _ := c.Begin(context.Background(), Target{}, nil)
	turn.ObserveOperation("sql", "SQL 处理中", Operation{Kind: "database", Status: "running"})
	_ = c.Stop(turn.ID())
	m := c.Snapshot().Messages[0]
	if m.Operation.Status != "unknown" || !strings.Contains(m.Operation.Next, "不要直接重复") {
		t.Fatal(m)
	}
	turn.ObserveOperation("sql", "late complete", Operation{Status: "completed"})
	if c.Snapshot().Messages[0].Operation.Status != "unknown" {
		t.Fatal("late result escaped stop fence")
	}
}
