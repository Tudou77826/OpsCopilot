package assistantcontrol

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestTimelinePreservesSegmentsQuestionAnswerAndRestore(t *testing.T) {
	c := &Coordinator{}
	turn, _ := c.Begin(context.Background(), Target{}, nil)
	defer turn.Finish("completed", "")
	turn.AppendSegment("先看", "first")
	turn.AppendSegment("日志", "first")
	turn.SetActivity("local")
	done := make(chan error, 1)
	go func() {
		_, err := turn.Request(context.Background(), Prompt{Kind: "input", Title: "哪一天？", Choices: []Choice{{ID: "today", Label: "今天"}}})
		done <- err
	}()
	s := awaitPrompt(t, c)
	if !s.Prompt.Expires.IsZero() {
		t.Fatal("question has an automatic deadline")
	}
	select {
	case <-done:
		t.Fatal("unanswered question continued")
	case <-time.After(20 * time.Millisecond):
	}
	if err := c.Respond(turn.ID(), s.Prompt.ID, Answer{Selected: []string{"today"}, Text: "只看上午"}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	turn.AppendSegment("检查结果", "second")
	s = c.Snapshot()
	if len(s.Messages) != 4 || s.Messages[0].Content != "先看日志" || s.Messages[1].Kind != "question" || s.Messages[2].Content != "今天\n只看上午" || s.Messages[3].Content != "检查结果" {
		t.Fatal(s.Messages)
	}
	s.Messages[0].Content = "changed"
	if c.Snapshot().Messages[0].Content != "先看日志" {
		t.Fatal("snapshot aliases messages")
	}
	h := &History{}
	h.Begin(turn.ID(), "看日志")
	s = c.Snapshot()
	s.Phase = "completed"
	h.Finish(s)
	for _, message := range h.Snapshot().Messages {
		if message.Time.IsZero() {
			t.Fatal("missing timestamp")
		}
	}
	if len(h.Snapshot().Messages) != 5 {
		t.Fatal(h.Snapshot())
	}
}

func TestExpiredApprovalRecordsNoSelectionAndRejectsLateReply(t *testing.T) {
	c := &Coordinator{}
	turn, _ := c.Begin(context.Background(), Target{}, nil)
	defer turn.Finish("completed", "")
	p := approval()
	p.Expires = time.Now().Add(30 * time.Millisecond)
	done := make(chan error, 1)
	go func() { _, err := turn.Request(context.Background(), p); done <- err }()
	s := awaitPrompt(t, c)
	if err := <-done; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if err := c.Respond(turn.ID(), s.Prompt.ID, Answer{Selected: []string{"allow"}}); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
	for _, message := range c.Snapshot().Messages {
		if message.Kind == "answer" {
			t.Fatal("invented answer")
		}
	}
	if !strings.Contains(c.Snapshot().Messages[1].Content, "未作选择或授权") {
		t.Fatal(c.Snapshot())
	}
}
