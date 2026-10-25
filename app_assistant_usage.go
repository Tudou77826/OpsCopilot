package main

import "opscopilot/pkg/agenttransport/acp"

// AssistantContextUsage reads local transport state only; it never starts iCode,
// contacts a model, acquires a tab, or restores stale persisted usage.
func (a *App) AssistantContextUsage() acp.ContextUsage {
	a.assistantConversationMu.Lock()
	defer a.assistantConversationMu.Unlock()
	if a.assistantConversation == nil {
		return acp.ContextUsage{}
	}
	return a.assistantConversation.ContextUsage()
}
