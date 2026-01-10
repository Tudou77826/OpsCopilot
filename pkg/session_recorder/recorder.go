package session_recorder

import (
	"encoding/json"
	"fmt"
	"opscopilot/pkg/terminal"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Recorder struct {
	currentSession *TroubleshootingSession
	storagePath    string
	lineBuffers    map[string]*terminal.LineBuffer
	mu             sync.Mutex
}

func NewRecorder(storagePath string) *Recorder {
	return &Recorder{
		storagePath: storagePath,
		lineBuffers: make(map[string]*terminal.LineBuffer),
	}
}

// StartSession initializes a new troubleshooting session
func (r *Recorder) StartSession(problem string, context []string) *TroubleshootingSession {
	r.mu.Lock()
	defer r.mu.Unlock()

	session := &TroubleshootingSession{
		ID:        uuid.New().String(),
		StartTime: time.Now(),
		Problem:   problem,
		Context:   context,
		Timeline:  make([]TimelineEvent, 0),
	}
	r.currentSession = session
	// Reset line buffers for new session
	r.lineBuffers = make(map[string]*terminal.LineBuffer)
	return session
}

// AddEvent adds an event to the current session's timeline
func (r *Recorder) AddEvent(eventType, content string, metadata map[string]interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.currentSession == nil {
		return fmt.Errorf("no active session")
	}

	if eventType == "terminal_input" {
		sessionID, ok := metadata["session_id"].(string)
		if !ok || sessionID == "" {
			// If no session ID, treat as raw event (fallback)
			r.appendEvent(eventType, content, metadata)
			return nil
		}

		lb, exists := r.lineBuffers[sessionID]
		if !exists {
			lb = terminal.NewLineBuffer()
			r.lineBuffers[sessionID] = lb
		}

		// Handle input through line buffer
		// We iterate through the content (which might be a chunk of chars)
		if committedLine, committed := lb.Handle(content); committed {
			// If line is committed (Enter pressed), record the full line
			if committedLine != "" {
				r.appendEvent(eventType, committedLine, metadata)
			}
		}
		// If not committed, we just updated the buffer state, nothing to record yet.
		return nil
	}

	// For non-input events, add directly
	r.appendEvent(eventType, content, metadata)
	return nil
}

// AddBroadcastInput handles broadcast input for multiple sessions, deduplicating the recording
func (r *Recorder) AddBroadcastInput(sessionIDs []string, content string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.currentSession == nil {
		return fmt.Errorf("no active session")
	}

	// Map to group sessions by the line they committed
	committedLines := make(map[string][]string)

	for _, sessionID := range sessionIDs {
		lb, exists := r.lineBuffers[sessionID]
		if !exists {
			lb = terminal.NewLineBuffer()
			r.lineBuffers[sessionID] = lb
		}

		if line, ok := lb.Handle(content); ok {
			if line != "" {
				committedLines[line] = append(committedLines[line], sessionID)
			}
		}
	}

	// Record events for unique committed lines
	for line, sids := range committedLines {
		metadata := map[string]interface{}{
			"session_ids": sids,
			"broadcast":   true,
		}
		r.appendEvent("terminal_input", line, metadata)
	}

	return nil
}

func (r *Recorder) appendEvent(eventType, content string, metadata map[string]interface{}) {
	event := TimelineEvent{
		Timestamp: time.Now(),
		Type:      eventType,
		Content:   content,
		Metadata:  metadata,
	}
	r.currentSession.Timeline = append(r.currentSession.Timeline, event)
}

// StopSession ends the current session and saves it
func (r *Recorder) StopSession(rootCause string, conclusion string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.currentSession == nil {
		return fmt.Errorf("no active session")
	}

	r.currentSession.EndTime = time.Now()
	r.currentSession.RootCause = rootCause
	r.currentSession.Conclusion = conclusion

	// Save to disk
	if err := r.saveSessionLocked(); err != nil {
		return err
	}

	// Session is stopped but kept in memory until next StartSession or explicit clear
	return nil
}

// saveSessionLocked saves the current session to a JSON file
func (r *Recorder) saveSessionLocked() error {
	if r.currentSession == nil {
		return nil
	}

	// Ensure storage directory exists
	if err := os.MkdirAll(r.storagePath, 0755); err != nil {
		return fmt.Errorf("failed to create storage directory: %w", err)
	}

	filename := filepath.Join(r.storagePath, fmt.Sprintf("session_%s.json", r.currentSession.ID))
	data, err := json.MarshalIndent(r.currentSession, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}

	return nil
}

// GetCurrentSession returns the current session (thread-safe)
func (r *Recorder) GetCurrentSession() *TroubleshootingSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.currentSession
}

// UpdateTimeline updates the timeline of the current session
func (r *Recorder) UpdateTimeline(events []TimelineEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.currentSession == nil {
		return fmt.Errorf("no active session")
	}

	r.currentSession.Timeline = events
	return nil
}
