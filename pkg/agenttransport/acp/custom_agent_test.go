package acp

import (
	"context"
	"opscopilot/pkg/assistantcontrol"
	"testing"
	"time"
)

func TestCustomAgentUsesStdioWithoutHTTP(t *testing.T) {
	config := testConversationConfig(t)
	t.Setenv("ACP_CONVERSATION_INCOMPATIBLE", "1")
	t.Setenv("ACP_TEST_STDIO", "1")
	config.AgentKind, config.ArgumentsJSON = "custom", `["acp"]`
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conversation := &Conversation{}
	defer conversation.Close()
	coordinator := &assistantcontrol.Coordinator{}
	turn, err := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer turn.Finish("completed", "")
	if err := conversation.Run(ctx, config, turn, &managedFixtureTools{}, "hello"); err != nil {
		t.Fatal(err)
	}
	if coordinator.Snapshot().Output == "" {
		t.Fatal("no response")
	}
}
