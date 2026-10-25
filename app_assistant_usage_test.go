package main

import "testing"

func TestAssistantContextUsageDoesNotStartRuntime(t *testing.T) {
	a := &App{}
	if usage := a.AssistantContextUsage(); usage.Known || usage.Used != 0 || usage.Size != 0 {
		t.Fatal(usage)
	}
	if a.assistantConversation != nil {
		t.Fatal("gauge read started iCode")
	}
}
