package acp

import (
	"context"
	"encoding/json"
	"testing"

	"opscopilot/pkg/assistantcontrol"
)

func TestContextUsageTracksCurrentWindowIncludingCompaction(t *testing.T) {
	c := &Conversation{}
	if c.ContextUsage().Known {
		t.Fatal("new session has invented usage")
	}
	c.observeContextUsage(json.RawMessage(`{"update":{"sessionUpdate":"usage_update","used":400,"size":1000}}`))
	if c.ContextUsage() != (ContextUsage{Known: true, Used: 400, Size: 1000}) {
		t.Fatal(c.ContextUsage())
	}
	// Compaction can lower the window gauge; it is not a cumulative counter.
	c.observeContextUsage(json.RawMessage(`{"update":{"sessionUpdate":"usage_update","used":100,"size":1000}}`))
	if c.ContextUsage().Used != 100 {
		t.Fatal("usage did not decrease after compaction")
	}
	for _, bad := range []string{
		`{"update":{"sessionUpdate":"usage_update","used":-1,"size":1000}}`,
		`{"update":{"sessionUpdate":"usage_update","used":5,"size":-1}}`,
		`{"update":{"sessionUpdate":"usage_update","used":1.2,"size":1000}}`,
		`{"update":{"sessionUpdate":"usage_update","used":"5","size":1000}}`,
		`{"update":{"sessionUpdate":"usage_update","used":9007199254740992,"size":1000}}`,
		`{"update":{"sessionUpdate":"usage_update","size":1000}}`,
		`{"update":{"sessionUpdate":"usage_update","used":null,"size":1000}}`,
		`{"update":{"sessionUpdate":"agent_message_chunk","used":50,"size":1000}}`,
	} {
		c.observeContextUsage(json.RawMessage(bad))
		if c.ContextUsage().Used != 100 {
			t.Fatal("invalid usage overwrote gauge", bad)
		}
	}
	c.observeContextUsage(json.RawMessage(`{"update":{"sessionUpdate":"usage_update","used":0,"size":1000}}`))
	if !c.ContextUsage().Known || c.ContextUsage().Used != 0 {
		t.Fatal("known empty context lost")
	}
	c.observeContextUsage(json.RawMessage(`{"update":{"sessionUpdate":"usage_update","used":12,"size":0}}`))
	if c.ContextUsage().Known {
		t.Fatal("unknown window size shown as known")
	}
}

func TestContextUsageRejectsForeignAndStoppedSessions(t *testing.T) {
	c := &Conversation{}
	c.router.observeUsage = c.observeContextUsage
	coordinator := &assistantcontrol.Coordinator{}
	turn, _ := coordinator.Begin(context.Background(), assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	if err := c.router.bind(turn, &managedFixtureTools{}, "main"); err != nil {
		t.Fatal(err)
	}
	defer c.router.clear()
	for _, session := range []string{"foreign", "main"} {
		update, _ := json.Marshal(map[string]any{"sessionId": session, "update": map[string]any{"sessionUpdate": "usage_update", "used": 25, "size": 100}})
		c.router.Notify(context.Background(), "session/update", update)
		if c.ContextUsage().Known != (session == "main") {
			t.Fatal("foreign session updated gauge")
		}
	}
	turn.Finish("stopped", "")
	c.router.Notify(context.Background(), "session/update", json.RawMessage(`{"sessionId":"main","update":{"sessionUpdate":"usage_update","used":90,"size":100}}`))
	if c.ContextUsage().Used != 25 {
		t.Fatal("stopped turn changed gauge")
	}
}

func TestContextUsageClearsOnModelChangeAndClose(t *testing.T) {
	c := &Conversation{}
	c.observeContextUsage(json.RawMessage(`{"update":{"sessionUpdate":"usage_update","used":120,"size":100}}`))
	if c.ContextUsage().Used != 120 {
		t.Fatal("over-capacity usage discarded")
	}
	if err := c.SwitchLocalModel(context.Background(), LocalModel{}); err != nil {
		t.Fatal(err)
	}
	if c.ContextUsage().Known {
		t.Fatal("old model usage survived switch")
	}
	c.observeContextUsage(json.RawMessage(`{"update":{"sessionUpdate":"usage_update","used":50,"size":100}}`))
	c.Close()
	if c.ContextUsage().Known {
		t.Fatal("closed session usage survived")
	}
}
