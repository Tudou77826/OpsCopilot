package sessionmanager

import (
	"opscopilot/pkg/sshclient"
	"os"
	"path/filepath"
	"testing"
)

func TestNewManager(t *testing.T) {
	m := NewManager()
	if m.Sessions == nil {
		t.Error("Sessions should be initialized")
	}
	if m.filePath != "sessions.json" {
		t.Errorf("Expected default filePath 'sessions.json', got %s", m.filePath)
	}
}

func TestUpsertSession(t *testing.T) {
	// Setup temporary file
	tmpFile := filepath.Join(os.TempDir(), "test_sessions.json")
	defer os.Remove(tmpFile)

	m := NewManager()
	m.filePath = tmpFile

	config := sshclient.ConnectConfig{
		Host: "192.168.1.1",
		User: "root",
		Port: 22,
	}

	// Test 1: Add new session (Root)
	err := m.Upsert(config, "")
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	if len(m.Sessions) != 1 {
		t.Fatalf("Expected 1 session, got %d", len(m.Sessions))
	}
	if m.Sessions[0].Name != "192.168.1.1" {
		t.Errorf("Expected name '192.168.1.1', got %s", m.Sessions[0].Name)
	}
	if m.Sessions[0].Type != TypeSession {
		t.Errorf("Expected type 'session', got %s", m.Sessions[0].Type)
	}

	// Test 2: Update existing session
	config.User = "admin"
	err = m.Upsert(config, "")
	if err != nil {
		t.Fatalf("Upsert update failed: %v", err)
	}
	if len(m.Sessions) != 1 {
		t.Fatalf("Expected 1 session after update, got %d", len(m.Sessions))
	}
	if m.Sessions[0].Config.User != "admin" {
		t.Errorf("Expected user 'admin', got %s", m.Sessions[0].Config.User)
	}

	// Test 3: Move to Group
	err = m.Upsert(config, "Prod")
	if err != nil {
		t.Fatalf("Upsert move failed: %v", err)
	}
	
	// Should have 1 folder in root, and session inside it
	if len(m.Sessions) != 1 {
		t.Fatalf("Expected 1 node (folder) in root, got %d", len(m.Sessions))
	}
	folder := m.Sessions[0]
	if folder.Type != TypeFolder || folder.Name != "Prod" {
		t.Errorf("Expected folder 'Prod', got %s (%s)", folder.Name, folder.Type)
	}
	if len(folder.Children) != 1 {
		t.Fatalf("Expected 1 child in folder, got %d", len(folder.Children))
	}
	if folder.Children[0].Config.Host != "192.168.1.1" {
		t.Errorf("Expected session in folder")
	}
}

func TestDeleteSession(t *testing.T) {
	tmpFile := filepath.Join(os.TempDir(), "test_sessions_delete.json")
	defer os.Remove(tmpFile)

	m := NewManager()
	m.filePath = tmpFile

	config := sshclient.ConnectConfig{Host: "1.1.1.1"}
	m.Upsert(config, "")

	id := m.Sessions[0].ID
	
	err := m.DeleteSession(id)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if len(m.Sessions) != 0 {
		t.Errorf("Expected 0 sessions, got %d", len(m.Sessions))
	}
}

func TestRenameSession(t *testing.T) {
	tmpFile := filepath.Join(os.TempDir(), "test_sessions_rename.json")
	defer os.Remove(tmpFile)

	m := NewManager()
	m.filePath = tmpFile

	config := sshclient.ConnectConfig{Host: "1.1.1.1"}
	m.Upsert(config, "")

	id := m.Sessions[0].ID
	
	err := m.RenameSession(id, "NewName")
	if err != nil {
		t.Fatalf("Rename failed: %v", err)
	}

	if m.Sessions[0].Name != "NewName" {
		t.Errorf("Expected name 'NewName', got %s", m.Sessions[0].Name)
	}
}

func TestPersistence(t *testing.T) {
	tmpFile := filepath.Join(os.TempDir(), "test_sessions_persist.json")
	defer os.Remove(tmpFile)

	m1 := NewManager()
	m1.filePath = tmpFile
	m1.Upsert(sshclient.ConnectConfig{Host: "1.1.1.1"}, "GroupA")

	// Load with new manager
	m2 := NewManager()
	m2.filePath = tmpFile
	err := m2.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(m2.Sessions) != 1 {
		t.Fatalf("Expected 1 folder loaded, got %d", len(m2.Sessions))
	}
	if m2.Sessions[0].Name != "GroupA" {
		t.Errorf("Expected group 'GroupA', got %s", m2.Sessions[0].Name)
	}
	if len(m2.Sessions[0].Children) != 1 {
		t.Errorf("Expected 1 child in group")
	}
}

func TestRecursiveDelete(t *testing.T) {
    tmpFile := filepath.Join(os.TempDir(), "test_sessions_recursive_delete.json")
	defer os.Remove(tmpFile)

	m := NewManager()
	m.filePath = tmpFile

    // Create Group -> Session
	m.Upsert(sshclient.ConnectConfig{Host: "1.1.1.1"}, "GroupA")
    
    // Find Group ID
    groupID := m.Sessions[0].ID

    // Delete Group
    err := m.DeleteSession(groupID)
    if err != nil {
        t.Fatalf("Delete group failed: %v", err)
    }

    if len(m.Sessions) != 0 {
        t.Errorf("Expected root empty after group delete, got %d", len(m.Sessions))
    }
}
