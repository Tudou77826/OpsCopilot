package acp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestManagedCapabilitiesFailBeforeConfiguringIncompatibleRuntime(t *testing.T) {
	for _, tc := range []struct {
		name, capabilities, missing string
	}{
		{"current", `{"mcpCapabilities":{"http":true},"_meta":{"chrys":{"apiVersion":1,"methods":["profiles/models/write","profiles/agents/write","settings/reload","future/method"]}}}`, ""},
		{"legacy", `{}`, "apiVersion"},
		{"future", `{"_meta":{"chrys":{"apiVersion":2}}}`, "apiVersion"},
		{"no-http", `{"_meta":{"chrys":{"apiVersion":1}}}`, "HTTP MCP"},
		{"missing-reload", `{"mcpCapabilities":{"http":true},"_meta":{"chrys":{"apiVersion":1,"methods":["profiles/models/write","profiles/agents/write"]}}}`, "settings/reload"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var result InitializeResult
			if err := json.Unmarshal([]byte(`{"protocolVersion":1,"agentCapabilities":`+tc.capabilities+`}`), &result); err != nil {
				t.Fatal(err)
			}
			err := result.requireManagedCapabilities()
			if tc.missing == "" && err != nil || tc.missing != "" && (err == nil || !strings.Contains(err.Error(), tc.missing)) {
				t.Fatal(err)
			}
		})
	}
}
