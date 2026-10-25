package acp

// NewRecoveredConversation starts a fresh runtime with bounded historical prose
// on its first prompt. It never loads a native session or resumes pending work.
func NewRecoveredConversation(context string) *Conversation {
	return &Conversation{recoveredContext: context}
}
