package assistantcontrol

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestTurnClockSeparatesToolUnionConfirmationAndResponse(t *testing.T) {
	start := time.Unix(100, 0)
	c := turnClock{enabled: true, last: start, activeTools: map[string]bool{}}
	c.advance(start.Add(5 * time.Second))
	c.activeTools["a"] = true
	c.advance(start.Add(6 * time.Second))
	c.activeTools["b"] = true
	c.advance(start.Add(7 * time.Second))
	c.waiting = true
	c.advance(start.Add(17 * time.Second))
	c.waiting = false
	delete(c.activeTools, "a")
	delete(c.activeTools, "b")
	c.advance(start.Add(20 * time.Second))
	if c.response != 8*time.Second || c.tools != 2*time.Second || c.confirmation != 10*time.Second {
		t.Fatalf("overlap or confirmation counted twice: %+v", c)
	}
	if !strings.Contains(c.summary(start.Add(20*time.Second)), "本轮 20.00 秒") {
		t.Fatal("incorrect total")
	}
}

func TestTimingFinalizesOnceAndCannotBeChangedByLateTools(t *testing.T) {
	coordinator := &Coordinator{}
	turn, _ := coordinator.Begin(context.Background(), Target{}, nil)
	turn.EnableTiming()
	turn.TrackTool("one", "in_progress")
	turn.TrackTool("one", "completed")
	turn.AppendText("结果")
	turn.Finish("completed", "")
	before := turn.Snapshot()
	turn.TrackTool("one", "in_progress")
	turn.Finish("stopped", "")
	after := turn.Snapshot()
	if len(before.Messages) != 2 || len(after.Messages) != 2 || after.Messages[1].Kind != "timing" || before.Messages[1].Content != after.Messages[1].Content {
		t.Fatal("timing missing, duplicated or changed")
	}
	if after.Output != "结果" {
		t.Fatal("timing changed model output")
	}
}
