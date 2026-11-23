package main

import "opscopilot/pkg/agenttransport/acp"

// AssistantAgentCommands is a read-only snapshot of commands advertised by
// the active ACP session. It does not execute or authorize a command.
func (a *App) AssistantAgentCommands() []acp.AgentCommand {
	a.assistantConversationMu.Lock()
	defer a.assistantConversationMu.Unlock()
	if a.assistantConversation == nil {
		return []acp.AgentCommand{}
	}
	return a.assistantConversation.SessionFeatures().Commands
}
