package acp

import (
	"slices"
	"strings"
	"testing"

	"opscopilot/pkg/agenttransport/mcp"
)

func TestManagedProfileExposesReadOnlyApplicationReferences(t *testing.T) {
	profile := managedProfile("http://127.0.0.1/fixture")
	tools := profile["tools"].(map[string]any)["mcp"].([]any)[0].(map[string]any)["allowed_tools"].([]string)
	approval := profile["approval"].(map[string]any)["overrides"].(map[string]string)
	instructions := profile["instructions"].(string)
	for _, name := range mcp.ReferenceToolNames() {
		if !slices.Contains(tools, name) || approval[name] == "skip" || !strings.Contains(instructions, name) {
			t.Fatal("tool omitted from managed runtime", name)
		}
	}
	if !strings.Contains(instructions, "not current facts") || !strings.Contains(instructions, "does not approve execution") {
		t.Fatal("reference material gained implicit authority")
	}
}
