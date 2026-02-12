package mcp

import (
	"context"
	"testing"
	"time"
)

// TestEchoServer creates a simple echo server for testing
// In production, this would be an actual MCP server
func TestStdioClient_StartAndStop(t *testing.T) {
	// For Windows testing, we can use cmd.exe with a simple echo command
	// This verifies the client can start and stop a process

	client := NewClient()

	// Test starting a non-existent server should fail
	err := client.Start(context.Background(), "non-existent-server-xyz123")
	if err == nil {
		t.Error("Expected error when starting non-existent server, got nil")
	}

	// Test IsReady should be false after failed start
	if client.IsReady() {
		t.Error("Expected IsReady to be false after failed start")
	}

	// Test stop when not started should not error
	err = client.Stop(context.Background())
	if err != nil {
		t.Errorf("Expected no error when stopping non-started client, got %v", err)
	}
}

func TestStdioClient_ConcurrentAccess(t *testing.T) {
	client := NewClient()

	// Test concurrent access to IsReady
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			client.IsReady()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should not panic or deadlock
}

func TestStdioClient_StartWithInvalidPath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := NewClient()

	// Test with empty path
	err := client.Start(ctx, "")
	if err == nil {
		t.Error("Expected error with empty server path")
	}

	// Verify not ready
	if client.IsReady() {
		t.Error("Client should not be ready with empty path")
	}
}

func TestNewClient(t *testing.T) {
	client := NewClient()

	if client == nil {
		t.Error("NewClient should return a non-nil client")
	}

	// Verify initial state
	if client.IsReady() {
		t.Error("New client should not be ready")
	}
}

func TestStdioClient_ListToolsWhenNotStarted(t *testing.T) {
	client := NewClient()

	// ListTools should fail when client not started
	_, err := client.ListTools(context.Background())
	if err == nil {
		t.Error("Expected error when calling ListTools on non-started client")
	}
}

func TestStdioClient_CallToolWhenNotStarted(t *testing.T) {
	client := NewClient()

	// CallTool should fail when client not started
	_, err := client.CallTool(context.Background(), "test_tool", nil)
	if err == nil {
		t.Error("Expected error when calling CallTool on non-started client")
	}
}

