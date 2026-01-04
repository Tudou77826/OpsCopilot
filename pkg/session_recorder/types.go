package session_recorder

import "time"

// TimelineEvent represents a single event in the troubleshooting timeline
type TimelineEvent struct {
	Timestamp time.Time              `json:"timestamp"`
	Type      string                 `json:"type"` // e.g., "user_query", "ai_suggestion", "terminal_action"
	Content   string                 `json:"content"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// TroubleshootingSession represents a complete investigation session
type TroubleshootingSession struct {
	ID         string          `json:"id"`
	StartTime  time.Time       `json:"start_time"`
	EndTime    time.Time       `json:"end_time"`
	Problem    string          `json:"problem"`
	Context    []string        `json:"context"` // List of loaded documents/contexts
	Timeline   []TimelineEvent `json:"timeline"`
	RootCause  string          `json:"root_cause"`
	Conclusion string          `json:"conclusion"`
}
