package acp

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestStandardModelSwitchRequiresAdvertisedSelection(t *testing.T) {
	cfg := testConversationConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c := &Conversation{}
	defer c.Close()
	if err := c.start(ctx, ctx, cfg); err != nil {
		t.Fatal(err)
	}
	c.features.update(json.RawMessage(`{"configOptions":[{"id":"model","name":"Model","category":"model","type":"select","currentValue":"first","options":[{"value":"first","name":"First"},{"value":"second","name":"Second"}]}]}`))
	if err := c.SwitchStandardModel(ctx, "missing"); err == nil {
		t.Fatal("unadvertised model accepted")
	}
	if err := c.SwitchStandardModel(ctx, "second"); err != nil {
		t.Fatal(err)
	}
	if got := c.SessionFeatures(); len(got.Options) != 1 || got.Options[0].Current != "second" {
		t.Fatalf("options = %+v", got)
	}
}

func TestLiveOpenCodeStandardModelSwitch(t *testing.T) {
	exe := os.Getenv("OPSCOPILOT_OPENCODE_TEST_EXE")
	if exe == "" {
		t.Skip("opt-in OpenCode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	dir := t.TempDir()
	client, err := Start(ctx, ProcessConfig{Executable: exe, Args: []string{"acp"}, Directory: dir, Environment: probeEnvironment(dir)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	c := &Conversation{client: client}
	defer c.Close()
	if _, err = client.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	var raw json.RawMessage
	if err = client.Call(ctx, "session/new", map[string]any{"cwd": dir, "mcpServers": []any{}}, &raw); err != nil {
		t.Fatal(err)
	}
	var session struct {
		SessionID string `json:"sessionId"`
	}
	if err = json.Unmarshal(raw, &session); err != nil || session.SessionID == "" {
		t.Fatal("missing session id", err)
	}
	c.sessionID = session.SessionID
	c.features.update(raw)
	var target string
	for _, option := range c.SessionFeatures().Options {
		if option.Category == "model" {
			for _, choice := range option.Choices {
				if choice.Value != option.Current {
					target = choice.Value
					break
				}
			}
		}
	}
	if target == "" {
		t.Skip("agent reported fewer than two models")
	}
	if err = c.SwitchStandardModel(ctx, target); err != nil {
		t.Fatal(err)
	}
	for _, option := range c.SessionFeatures().Options {
		if option.Category == "model" && option.Current == target {
			return
		}
	}
	t.Fatal("agent did not confirm selected model")
}
