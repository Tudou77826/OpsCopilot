package acp

import (
	"context"
	"encoding/json"
	"testing"

	"opscopilot/pkg/assistantcontrol"
)

func TestManagedProfileEnablesNativeToolsWithoutApprovalBypass(t *testing.T) {
	profile := managedProfile("http://127.0.0.1:1234/mcp")
	approval := profile["approval"].(map[string]any)
	if approval["default"] != "require" || approval["user_can_override"] != false {
		t.Fatal(approval)
	}
	overrides := approval["overrides"].(map[string]string)
	builtins := profile["tools"].(map[string]any)["builtins"].([]string)
	if len(builtins) != 5 {
		t.Fatal("native local tool categories missing", builtins)
	}
	for _, name := range []string{"run_skill_script", "shell", "filesystem.write"} {
		if overrides[name] != "" {
			t.Fatal("unexpected executable/read bypass", name)
		}
	}
	for _, name := range []string{"load_skill", "read_skill_resource"} {
		if overrides[name] != "skip" {
			t.Fatal("progressive skill access unavailable", name)
		}
	}
	c := &assistantcontrol.Coordinator{}
	turn, _ := c.Begin(context.Background(), assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	bridge := &Interactions{Turn: turn, SessionID: "bound", DenyPermissions: true}
	result, err := bridge.Request(context.Background(), "session/request_permission", json.RawMessage(`{"sessionId":"bound","options":[{"optionId":"allow","kind":"allow_once"}]}`))
	if err != nil || c.Snapshot().Prompt != nil {
		t.Fatal(result, err)
	}
	data, _ := json.Marshal(result)
	if string(data) != `{"outcome":{"outcome":"cancelled"}}` {
		t.Fatal(string(data))
	}
}
