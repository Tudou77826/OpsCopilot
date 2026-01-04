package session_recorder

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRecorderLifecycle(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "recorder_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	recorder := NewRecorder(tmpDir)

	// 1. Start Session
	problem := "Database connection timeout"
	context := []string{"db_docs.md"}
	session := recorder.StartSession(problem, context)

	if session.ID == "" {
		t.Error("Session ID should not be empty")
	}
	if session.Problem != problem {
		t.Errorf("Expected problem %s, got %s", problem, session.Problem)
	}

	// 2. Add Events
	err = recorder.AddEvent("user_query", "Check db status", nil)
	if err != nil {
		t.Errorf("Failed to add event: %v", err)
	}
	
	err = recorder.AddEvent("terminal_action", "systemctl status postgresql", map[string]interface{}{"exit_code": 0})
	if err != nil {
		t.Errorf("Failed to add event: %v", err)
	}

	// Verify events in memory
	current := recorder.GetCurrentSession()
	if len(current.Timeline) != 2 {
		t.Errorf("Expected 2 events, got %d", len(current.Timeline))
	}

	// 3. Stop Session
	rootCause := "Network firewall rule"
	err = recorder.StopSession(rootCause)
	if err != nil {
		t.Fatalf("Failed to stop session: %v", err)
	}

	// 4. Verify Persistence
	filename := filepath.Join(tmpDir, "session_"+session.ID+".json")
	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("Failed to read saved session file: %v", err)
	}

	var savedSession TroubleshootingSession
	err = json.Unmarshal(content, &savedSession)
	if err != nil {
		t.Fatalf("Failed to unmarshal saved session: %v", err)
	}

	if savedSession.ID != session.ID {
		t.Errorf("Saved session ID mismatch")
	}
	if savedSession.RootCause != rootCause {
		t.Errorf("Saved root cause mismatch")
	}
	if len(savedSession.Timeline) != 2 {
		t.Errorf("Saved timeline length mismatch")
	}
}
