package assistantcontrol

import (
	"fmt"
	"strings"
	"testing"
)

func TestHistoryRestoresOriginalMessagesAndPartialStoppedOutput(t *testing.T) {
	h := &History{}
	h.Begin("one", "原问题")
	h.Finish(State{TurnID: "one", Output: "已经取得的输出", Phase: "stopped", Message: "已停止"})
	h.Finish(State{TurnID: "one", Output: "迟到结果", Phase: "completed"})
	s := h.Snapshot()
	if s.ID == "" || len(s.Messages) != 2 || s.Messages[1].Phase != "stopped" || s.Messages[1].Content != "已经取得的输出" {
		t.Fatal(s)
	}
	s.Messages[0].Content = "修改快照"
	if h.Snapshot().Messages[0].Content != "原问题" {
		t.Fatal("snapshot aliases stored history")
	}
	h.Reset()
	if next := h.Snapshot(); next.ID == s.ID || len(next.Messages) != 0 || next.Truncated || next.Messages == nil {
		t.Fatal(next)
	}
}

func TestHistoryKeepsFailureSeparateFromObservedPartialAnswer(t *testing.T) {
	h := &History{}
	h.Begin("failed", "问题")
	h.Finish(State{TurnID: "stale", Phase: "failed", Message: "旧失败"})
	h.Finish(State{TurnID: "failed", Phase: "failed", Output: "已观察到的结果", Message: "连接中断"})
	s := h.Snapshot()
	if len(s.Messages) != 2 || s.Messages[1].Phase != "failed" || !strings.Contains(s.Messages[1].Content, "已观察到的结果") || !strings.Contains(s.Messages[1].Content, "连接中断") {
		t.Fatal(s)
	}
}

func TestHistoryBoundsMemoryAndEvictsWholeOldTurns(t *testing.T) {
	for _, size := range []int{8, 128 * 1024} {
		h := &History{}
		for i := 0; i < 100; i++ {
			id := fmt.Sprint(i)
			h.Begin(id, "question")
			h.Finish(State{TurnID: id, Phase: "completed", Output: strings.Repeat("x", size)})
		}
		s := h.Snapshot()
		if !s.Truncated || h.bytes > historyMaxBytes || len(s.Messages) > historyMaxMessages || s.Messages[0].Role != "user" || s.Messages[len(s.Messages)-1].TurnID != "99" {
			t.Fatalf("size=%d messages=%d bytes=%d", size, len(s.Messages), h.bytes)
		}
	}
}
