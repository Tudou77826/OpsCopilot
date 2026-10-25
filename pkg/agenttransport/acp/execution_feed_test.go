package acp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"opscopilot/pkg/assistantcontrol"
)

func TestSkillReadsHaveCompactReceiptsWithoutDuplicatingMCP(t *testing.T) {
	c := &assistantcontrol.Coordinator{}
	turn, _ := c.Begin(context.Background(), assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	feed := &executionFeed{}
	feed.observe(turn, json.RawMessage(`{"update":{"toolCallId":"skill","kind":"other","title":"load_skill","status":"completed","rawInput":{"skill_name":"payments"}}}`))
	feed.observe(turn, json.RawMessage(`{"update":{"toolCallId":"mcp","kind":"other","title":"knowledge_list","status":"completed"}}`))
	messages := turn.Snapshot().Messages
	if len(messages) != 1 || messages[0].Operation.Action != "payments" || messages[0].Operation.Kind != "reference" {
		t.Fatal("Skill read hidden or MCP duplicated")
	}
}

func TestExecutionFeedMergesNativeObservationsButNotMCPOrThinking(t *testing.T) {
	c := &assistantcontrol.Coordinator{}
	turn, _ := c.Begin(context.Background(), assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	f := &executionFeed{}
	observe := func(update string) { f.observe(turn, json.RawMessage(`{"update":`+update+`}`)) }
	observe(`{"toolCallId":"host","kind":"other","title":"terminal_execute","rawInput":{"command":"ignored-host-duplicate"},"status":"in_progress"}`)
	observe(`{"toolCallId":"load","kind":"other","title":"load_skill","status":"completed"}`)
	observe(`{"toolCallId":"local","kind":"execute","title":"printf x","rawInput":{"command":"printf x"},"status":"in_progress"}`)
	observe(`{"toolCallId":"local","status":"completed","content":[{"type":"content","content":{"type":"text","text":"x\npassword=do-not-save"}}]}`)
	observe(`{"toolCallId":"local","status":"in_progress","rawOutput":"late-output"}`)
	got := turn.Snapshot().Messages
	if len(got) != 1 || got[0].Operation.Action != "printf x" || got[0].Operation.Status != "completed" || !strings.Contains(got[0].Operation.Result, "x\n") || strings.Contains(got[0].Operation.Result, "do-not-save") || strings.Contains(got[0].Operation.Result, "late-output") {
		t.Fatal(got)
	}
	_ = c.Stop(turn.ID())
	observe(`{"toolCallId":"late","kind":"execute","title":"not executed","status":"completed"}`)
	if len(turn.Snapshot().Messages) != 1 {
		t.Fatal("late callback escaped turn fence")
	}
}
