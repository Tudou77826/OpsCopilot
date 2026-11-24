package acp

// NewRecoveredConversation starts a fresh runtime with bounded historical prose
// on its first prompt. When resumeSessionID is nonempty and the rebuilt agent
// declares loadSession, session/load is attempted first; native history then
// replaces the prose context. It never resumes pending work or tool approvals.
func NewRecoveredConversation(context, resumeSessionID string) *Conversation {
	return &Conversation{recoveredContext: context, resumeSessionID: resumeSessionID}
}
