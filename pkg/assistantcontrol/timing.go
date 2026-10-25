package assistantcontrol

import (
	"fmt"
	"time"
)

// Host-observed wall time, not provider inference latency. The response bucket
// includes startup, transport and generation; user confirmation has priority over
// an active tool so long approvals are never counted as model/tool execution.
type turnClock struct {
	enabled                       bool
	last                          time.Time
	response, tools, confirmation time.Duration
	waiting                       bool
	activeTools                   map[string]bool
}

func (c *turnClock) advance(now time.Time) {
	if !c.enabled {
		return
	}
	elapsed := now.Sub(c.last)
	if elapsed < 0 {
		return
	}
	if c.waiting {
		c.confirmation += elapsed
	} else if len(c.activeTools) > 0 {
		c.tools += elapsed
	} else {
		c.response += elapsed
	}
	c.last = now
}
func (c *turnClock) summary(now time.Time) string {
	c.advance(now)
	return fmt.Sprintf("本轮 %.2f 秒 · 助手响应 %.2f 秒 · 工具执行 %.3f 秒 · 等待确认 %.2f 秒", (c.response + c.tools + c.confirmation).Seconds(), c.response.Seconds(), c.tools.Seconds(), c.confirmation.Seconds())
}

func (t *Turn) EnableTiming() {
	t.owner.mu.Lock()
	defer t.owner.mu.Unlock()
	if t.owner.current == t && active(t.state.Phase) && !t.timing.enabled {
		t.timing = turnClock{enabled: true, last: time.Now(), activeTools: map[string]bool{}}
	}
}
func (t *Turn) TrackTool(id, status string) {
	t.owner.mu.Lock()
	defer t.owner.mu.Unlock()
	if t.owner.current != t || !active(t.state.Phase) || t.ctx.Err() != nil || !t.timing.enabled || id == "" || len(id) > 512 {
		return
	}
	t.timing.advance(time.Now())
	if status == "completed" || status == "failed" {
		delete(t.timing.activeTools, id)
	} else if status == "pending" || status == "in_progress" {
		if len(t.timing.activeTools) < 128 {
			t.timing.activeTools[id] = true
		}
	}
}
func (t *Turn) timingConfirmation(waiting bool) {
	t.timing.advance(time.Now())
	t.timing.waiting = waiting
}
