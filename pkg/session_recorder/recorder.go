package session_recorder

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Recorder struct {
	currentSession *TroubleshootingSession
	storagePath    string
	mu             sync.Mutex
}

func NewRecorder(storagePath string) *Recorder {
	return &Recorder{
		storagePath: storagePath,
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
	return session
}

// AddEvent adds an event to the current session's timeline
func (r *Recorder) AddEvent(eventType, content string, metadata map[string]interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.currentSession == nil {
		return fmt.Errorf("no active session")
	}

	event := TimelineEvent{
		Timestamp: time.Now(),
		Type:      eventType,
		Content:   content,
		Metadata:  metadata,
	}
	r.currentSession.Timeline = append(r.currentSession.Timeline, event)
	return nil
}

// StopSession ends the current session and saves it
func (r *Recorder) StopSession(rootCause string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.currentSession == nil {
		return fmt.Errorf("no active session")
	}

	r.currentSession.EndTime = time.Now()
	r.currentSession.RootCause = rootCause
	
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
