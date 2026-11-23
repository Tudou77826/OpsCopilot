package config

import (
	"encoding/json"
	"testing"
)

func TestAssistantRuntimePreservesCustomProfiles(t *testing.T) {
	value := AssistantRuntimeConfig{Kind: "custom", Executable: "agent-b", Arguments: []string{"acp"}, CustomAgentID: "b", PresetExecutables: map[string]string{"icode": "icode-path"}, CustomAgents: []AssistantCustomAgent{
		{ID: "a", Name: "A", Executable: "agent-a", Arguments: []string{"serve"}},
		{ID: "b", Name: "B", Executable: "agent-b", Arguments: []string{"acp"}},
	}}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var restored AssistantRuntimeConfig
	if err = json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.CustomAgentID != "b" || len(restored.CustomAgents) != 2 || restored.CustomAgents[0].Executable != "agent-a" || restored.PresetExecutables["icode"] != "icode-path" {
		t.Fatal("agent profiles were not preserved")
	}
}
