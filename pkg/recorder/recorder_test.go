package recorder

import (
    "encoding/json"
    "os"
    "path/filepath"
    "testing"
)

func TestRecordRawInput_Backspace(t *testing.T) {
    tmpDir := t.TempDir()
    rec := NewRecorder(tmpDir)

    _, err := rec.Start(RecordingTypeTroubleshoot, "session-1", "host", "user")
    if err != nil {
        t.Fatalf("Failed to start recording: %v", err)
    }

    // Simulate user input: "hello" + backspace + "p" + Enter
    inputs := []string{"h", "e", "l", "l", "o", "\x7f", "p", "\r"}
    for _, input := range inputs {
        rec.RecordRawInput("session-1", input)
    }

    session, err := rec.Stop()
    if err != nil {
        t.Fatalf("Failed to stop recording: %v", err)
    }

    if len(session.Commands) == 0 {
        t.Fatalf("Expected at least 1 command, got 0")
    }

    // 期望是 "hellp" (hell + o + 退格 + p)
    expected := "hellp"
    if session.Commands[0].Content != expected {
        t.Errorf("Expected command %q, got %q", expected, session.Commands[0].Content)
    }
}

func TestRecordRawInput_AnsiCursorMovement(t *testing.T) {
    tmpDir := t.TempDir()
    rec := NewRecorder(tmpDir)

    _, err := rec.Start(RecordingTypeTroubleshoot, "session-1", "host", "user")
    if err != nil {
        t.Fatalf("Failed to start recording: %v", err)
    }

    // Simulate: "ac" + left arrow + "b" + Enter = "abc"
    inputs := []string{"a", "c", "\x1b[D", "b", "\r"}
    for _, input := range inputs {
        rec.RecordRawInput("session-1", input)
    }

    session, err := rec.Stop()
    if err != nil {
        t.Fatalf("Failed to stop recording: %v", err)
    }
    if len(session.Commands) == 0 {
        t.Fatalf("Expected at least 1 command, got 0")
    }
    expected := "abc"
    if session.Commands[0].Content != expected {
        t.Errorf("Expected command %q, got %q", expected, session.Commands[0].Content)
    }
}

func TestRecordRawInput_ChineseCharacters(t *testing.T) {
    tmpDir := t.TempDir()
    rec := NewRecorder(tmpDir)

    _, err := rec.Start(RecordingTypeTroubleshoot, "session-1", "host", "user")
    if err != nil {
        t.Fatalf("Failed to start recording: %v", err)
    }

    // Simulate Chinese input
    inputs := []string{"你", "好", "\r"}
    for _, input := range inputs {
        rec.RecordRawInput("session-1", input)
    }

    session, err := rec.Stop()
    if err != nil {
        t.Fatalf("Failed to stop recording: %v", err)
    }

    if len(session.Commands) == 0 {
        t.Fatalf("Expected at least 1 command, got 0")
    }
    expected := "你好"
    if session.Commands[0].Content != expected {
        t.Errorf("Expected command %q, got %q", expected, session.Commands[0].Content)
    }
}

func TestRecordRawInput_HomeAndEnd(t *testing.T) {
    tmpDir := t.TempDir()
    rec := NewRecorder(tmpDir)

    _, err := rec.Start(RecordingTypeTroubleshoot, "session-1", "host", "user")
    if err != nil {
        t.Fatalf("Failed to start recording: %v", err)
    }
    // cd -> home -> insert ab -> abcd -> end -> insert e -> abcde
    inputs := []string{"c", "d", "\x1b[H", "a", "b", "\x1b[F", "e", "\r"}
    for _, input := range inputs {
        rec.RecordRawInput("session-1", input)
    }

    session, err := rec.Stop()
    if err != nil {
        t.Fatalf("Failed to stop recording: %v", err)
    }
    if len(session.Commands) == 0 {
        t.Fatalf("Expected at least 1 command, got 0")
    }
    expected := "abcde"
    if session.Commands[0].Content != expected {
        t.Errorf("Expected command %q, got %q", expected, session.Commands[0].Content)
    }
}

func TestRecordRawInput_DeleteKey(t *testing.T) {
    tmpDir := t.TempDir()
    rec := NewRecorder(tmpDir)

    _, err := rec.Start(RecordingTypeTroubleshoot, "session-1", "host", "user")
    if err != nil {
        t.Fatalf("Failed to start recording: %v", err)
    }
    // abc -> left -> left (at b) -> delete (removes b) -> ac
    inputs := []string{"a", "b", "c", "\x1b[D", "\x1b[D", "\x1b[3~", "\r"}
    for _, input := range inputs {
        rec.RecordRawInput("session-1", input)
    }

    session, err := rec.Stop()
    if err != nil {
        t.Fatalf("Failed to stop recording: %v", err)
    }
    if len(session.Commands) == 0 {
        t.Fatalf("Expected at least 1 command, got 0")
    }
    expected := "ac"
    if session.Commands[0].Content != expected {
        t.Errorf("Expected command %q, got %q", expected, session.Commands[0].Content)
    }
}

func TestRecordRawInput_MultipleCommands(t *testing.T) {
    tmpDir := t.TempDir()
    rec := NewRecorder(tmpDir)

    _, err := rec.Start(RecordingTypeTroubleshoot, "session-1", "host", "user")
    if err != nil {
        t.Fatalf("Failed to start recording: %v", err)
    }
    // First command: "ls"
    inputs1 := []string{"l", "s", "\r"}
    for _, input := range inputs1 {
        rec.RecordRawInput("session-1", input)
    }
    // Second command: "pwd"
    inputs2 := []string{"p", "w", "d", "\r"}
    for _, input := range inputs2 {
        rec.RecordRawInput("session-1", input)
    }

    session, err := rec.Stop()
    if err != nil {
        t.Fatalf("Failed to stop recording: %v", err)
    }
    if len(session.Commands) != 2 {
        t.Fatalf("Expected 2 commands, got %d", len(session.Commands))
    }
    expected := []string{"ls", "pwd"}
    for i, cmd := range expected {
        if session.Commands[i].Content != cmd {
            t.Errorf("Command %d: expected %q, got %q", i, cmd, session.Commands[i].Content)
        }
    }
}

func TestAddEvent_TerminalInput(t *testing.T) {
    tmpDir := t.TempDir()
    rec := NewRecorder(tmpDir)

    _, err := rec.Start(RecordingTypeTroubleshoot, "session-1", "host", "user")
    if err != nil {
        t.Fatalf("Failed to start recording: %v", err)
    }
    // Add terminal_input event via AddEvent (should use LineBuffer)
    // "echo" + backspace + backspace + "l" + "l" + "o"
    inputs := []string{"e", "c", "h", "o", "\x7f", "\x7f", "l", "l", "o", "\r"}
    for _, input := range inputs {
        rec.AddEvent("terminal_input", input, map[string]interface{}{
            "session_id": "session-1",
        })
    }

    session, err := rec.Stop()
    if err != nil {
        t.Fatalf("Failed to stop recording: %v", err)
    }
    if len(session.Commands) == 0 {
        t.Fatalf("Expected at least 1 command, got 0")
    }
    // echo + 2 backspaces + l + l + o = "ecllo"
    expected := "ecllo"
    if session.Commands[0].Content != expected {
        t.Errorf("Expected command %q, got %q", expected, session.Commands[0].Content)
    }
    // Check timeline
    if len(session.Timeline) == 0 {
        t.Fatalf("Expected at least 1 timeline event, got 0")
    }
    if session.Timeline[0].Type != "terminal_input" {
        t.Errorf("Expected timeline event type %q, got %q", "terminal_input", session.Timeline[0].Type)
    }
}

func TestAddBroadcastInput(t *testing.T) {
    tmpDir := t.TempDir()
    rec := NewRecorder(tmpDir)

    _, err := rec.Start(RecordingTypeTroubleshoot, "session-1", "host", "user")
    if err != nil {
        t.Fatalf("Failed to start recording: %v", err)
    }
    // Broadcast input to multiple sessions
    sessionIDs := []string{"session-1", "session-2", "session-3"}
    inputs := []string{"l", "s", "\r"}
    for _, input := range inputs {
        rec.AddBroadcastInput(sessionIDs, input)
    }
    session, err := rec.Stop()
    if err != nil {
        t.Fatalf("Failed to stop recording: %v", err)
    }
    // Should only record one command (deduplicated)
    if len(session.Commands) != 1 {
        t.Fatalf("Expected 1 command, got %d", len(session.Commands))
    }
    expected := "ls"
    if session.Commands[0].Content != expected {
        t.Errorf("Expected command %q, got %q", expected, session.Commands[0].Content)
    }
    // Check metadata contains broadcast info
    if len(session.Timeline) == 0 {
        t.Fatalf("Expected at least 1 timeline event, got 0")
    }
    metadata := session.Timeline[0].Metadata
    broadcast, ok := metadata["broadcast"].(bool)
    if !ok || !broadcast {
        t.Errorf("Expected broadcast=true in metadata")
    }
    sessionIDsMeta, ok := metadata["session_ids"].([]string)
    if !ok || len(sessionIDsMeta) != 3 {
        t.Errorf("Expected 3 session_ids in metadata")
    }
}

func TestStartSession(t *testing.T) {
    tmpDir := t.TempDir()
    rec := NewRecorder(tmpDir)

    problem := "Server not responding"
    context := []string{"log1.txt", "config.yaml"}
    session := rec.StartSession(problem, context)
    if session == nil {
        t.Fatal("Expected session, got nil")
    }
    if session.ID == "" {
        t.Error("Expected session ID to be set")
    }
    if session.Problem != problem {
        t.Errorf("Expected problem %q, got %q", problem, session.Problem)
    }
    if len(session.Context) != len(context) {
        t.Errorf("Expected %d context items, got %d", len(context), len(session.Context))
    }
}

func TestStopSession(t *testing.T) {
    tmpDir := t.TempDir()
    rec := NewRecorder(tmpDir)

    rec.StartSession("Test problem", []string{"ctx1"})
    // Add some input
    rec.RecordRawInput("test-session", "l")
    rec.RecordRawInput("test-session", "s")
    rec.RecordRawInput("test-session", "\r")
    rootCause := "Network issue"
    conclusion := "Fixed by restarting network service"
    err := rec.StopSession(rootCause, conclusion)
    if err != nil {
        t.Fatalf("Failed to stop session: %v", err)
    }
    // Verify file was created
    files, err := os.ReadDir(filepath.Join(tmpDir, string(RecordingTypeTroubleshoot)))
    if err != nil {
        t.Fatalf("Failed to read recordings directory: %v", err)
    }
    if len(files) == 0 {
        t.Error("Expected recording file to be created")
    }
    // Verify file content has root cause and conclusion
    data, err := os.ReadFile(filepath.Join(tmpDir, string(RecordingTypeTroubleshoot), files[0].Name()))
    if err != nil {
        t.Fatalf("Failed to read recording file: %v", err)
    }
    var savedSession RecordingSession
    if err := json.Unmarshal(data, &savedSession); err != nil {
        t.Fatalf("Failed to unmarshal recording: %v", err)
    }
    if savedSession.RootCause != rootCause {
        t.Errorf("Expected root cause %q, got %q", rootCause, savedSession.RootCause)
    }
    if savedSession.Conclusion != conclusion {
        t.Errorf("Expected conclusion %q, got %q", conclusion, savedSession.Conclusion)
    }
}
func TestTimelineEvents(t *testing.T) {
    tmpDir := t.TempDir()
    rec := NewRecorder(tmpDir)

    _, err := rec.Start(RecordingTypeTroubleshoot, "session-1", "host", "user")
    if err != nil {
        t.Fatalf("Failed to start recording: %v", err)
    }
    // Add various event types
    rec.AddEvent("user_query", "What is the server status?", nil)
    rec.AddEvent("ai_suggestion", "Check the logs", nil)
    rec.AddEvent("terminal_output", "Server is running", map[string]interface{}{
        "session_id": "session-1",
    })
    session, err := rec.Stop()
    if err != nil {
        t.Fatalf("Failed to stop recording: %v", err)
    }
    if len(session.Timeline) != 3 {
        t.Fatalf("Expected 3 timeline events, got %d", len(session.Timeline))
    }
    expectedTypes := []string{"user_query", "ai_suggestion", "terminal_output"}
    for i, expectedType := range expectedTypes {
        if session.Timeline[i].Type != expectedType {
            t.Errorf("Event %d: expected type %q, got %q", i, expectedType, session.Timeline[i].Type)
        }
    }
}

func TestNoDuplicateRecording(t *testing.T) {
    tmpDir := t.TempDir()
    rec := NewRecorder(tmpDir)

    _, err := rec.Start(RecordingTypeTroubleshoot, "session-1", "host", "user")
    if err != nil {
        t.Fatalf("Failed to start recording: %v", err)
    }

    // Record the same command using RecordRawInput only (not both RecordRawInput and RecordInput)
    inputs := []string{"l", "s", "\r"}
    for _, input := range inputs {
        rec.RecordRawInput("session-1", input)
    }

    session, err := rec.Stop()
    if err != nil {
        t.Fatalf("Failed to stop recording: %v", err)
    }

    // Should only have 1 command, not 2 (no duplicate)
    if len(session.Commands) != 1 {
        t.Fatalf("Expected 1 command (no duplicates), got %d", len(session.Commands))
    }

    // Should have 1 timeline event for terminal_input
    inputEvents := 0
    for _, event := range session.Timeline {
        if event.Type == "terminal_input" {
            inputEvents++
        }
    }
    if inputEvents != 1 {
        t.Errorf("Expected 1 terminal_input event, got %d", inputEvents)
    }

    expected := "ls"
    if session.Commands[0].Content != expected {
        t.Errorf("Expected command %q, got %q", expected, session.Commands[0].Content)
    }
}

func TestAddEventReturnsCommittedLine(t *testing.T) {
    tmpDir := t.TempDir()
    rec := NewRecorder(tmpDir)

    _, err := rec.Start(RecordingTypeTroubleshoot, "session-1", "host", "user")
    if err != nil {
        t.Fatalf("Failed to start recording: %v", err)
    }

    // Test that AddEvent returns the committed line when Enter is pressed
    line, committed, err := rec.AddEvent("terminal_input", "h", map[string]interface{}{
        "session_id": "session-1",
    })
    if err != nil {
        t.Fatalf("Failed to add event: %v", err)
    }
    if committed {
        t.Error("Expected not committed yet")
    }
    if line != "" {
        t.Errorf("Expected empty line, got %q", line)
    }

    // Continue typing and press Enter
    line, committed, err = rec.AddEvent("terminal_input", "i", map[string]interface{}{
        "session_id": "session-1",
    })
    if err != nil {
        t.Fatalf("Failed to add event: %v", err)
    }
    if committed {
        t.Error("Expected not committed yet")
    }

    // Press Enter
    line, committed, err = rec.AddEvent("terminal_input", "\r", map[string]interface{}{
        "session_id": "session-1",
    })
    if err != nil {
        t.Fatalf("Failed to add event: %v", err)
    }
    if !committed {
        t.Error("Expected to be committed")
    }
    expected := "hi"
    if line != expected {
        t.Errorf("Expected line %q, got %q", expected, line)
    }

    session, err := rec.Stop()
    if err != nil {
        t.Fatalf("Failed to stop recording: %v", err)
    }

    // Verify command was recorded
    if len(session.Commands) != 1 {
        t.Fatalf("Expected 1 command, got %d", len(session.Commands))
    }
    if session.Commands[0].Content != expected {
        t.Errorf("Expected command %q, got %q", expected, session.Commands[0].Content)
    }
}
