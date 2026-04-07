package script

import (
	"opscopilot/pkg/recorder"
	"strings"
	"testing"
)

// --- SubstituteVariables tests ---

func TestSubstituteVariables_Basic(t *testing.T) {
	vars := map[string]string{
		"port":         "8080",
		"service_name": "nginx",
	}

	result := SubstituteVariables("systemctl restart ${service_name}", vars)
	if result != "systemctl restart nginx" {
		t.Errorf("expected 'systemctl restart nginx', got '%s'", result)
	}
}

func TestSubstituteVariables_Multiple(t *testing.T) {
	vars := map[string]string{
		"port": "3306",
		"host": "db.example.com",
	}

	result := SubstituteVariables("mysql -h ${host} -P ${port}", vars)
	if result != "mysql -h db.example.com -P 3306" {
		t.Errorf("expected 'mysql -h db.example.com -P 3306', got '%s'", result)
	}
}

func TestSubstituteVariables_NotFound(t *testing.T) {
	vars := map[string]string{
		"port": "8080",
	}

	result := SubstituteVariables("echo ${unknown_var}", vars)
	if result != "echo ${unknown_var}" {
		t.Errorf("expected original placeholder preserved, got '%s'", result)
	}
}

func TestSubstituteVariables_NilVars(t *testing.T) {
	result := SubstituteVariables("echo ${port}", nil)
	if result != "echo ${port}" {
		t.Errorf("expected original, got '%s'", result)
	}
}

func TestSubstituteVariables_NoVariables(t *testing.T) {
	vars := map[string]string{"port": "8080"}
	result := SubstituteVariables("ls -la", vars)
	if result != "ls -la" {
		t.Errorf("expected 'ls -la', got '%s'", result)
	}
}

func TestSubstituteVariables_EmptyValue(t *testing.T) {
	vars := map[string]string{
		"port": "",
	}
	result := SubstituteVariables("netstat -tlnp | grep ${port}", vars)
	if result != "netstat -tlnp | grep " {
		t.Errorf("expected 'netstat -tlnp | grep ', got '%s'", result)
	}
}

func TestSubstituteVariables_Adjacent(t *testing.T) {
	vars := map[string]string{
		"a": "hello",
		"b": "world",
	}
	result := SubstituteVariables("${a}${b}", vars)
	if result != "helloworld" {
		t.Errorf("expected 'helloworld', got '%s'", result)
	}
}

func TestSubstituteVariables_InvalidName(t *testing.T) {
	vars := map[string]string{"port": "8080"}
	result := SubstituteVariables("echo ${123bad}", vars)
	if result != "echo ${123bad}" {
		t.Errorf("expected original (invalid var name), got '%s'", result)
	}
}

// --- MigrateCommandsToSteps tests ---

func TestMigrateCommandsToSteps(t *testing.T) {
	s := &Script{
		Commands: []ScriptCommand{
			{RecordedCommand: recorderRecordedCommand(0, "ls -la"), Comment: "list files", Delay: 0, Enabled: true},
			{RecordedCommand: recorderRecordedCommand(1, "pwd"), Comment: "", Delay: 100, Enabled: true},
			{RecordedCommand: recorderRecordedCommand(2, "echo disabled"), Comment: "", Delay: 0, Enabled: false},
		},
	}

	s.MigrateCommandsToSteps()

	if len(s.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(s.Steps))
	}
	if s.Steps[0].Command != "ls -la" {
		t.Errorf("expected command 'ls -la', got '%s'", s.Steps[0].Command)
	}
	if s.Steps[0].Comment != "list files" {
		t.Errorf("expected comment 'list files', got '%s'", s.Steps[0].Comment)
	}
	if s.Steps[1].Delay != 100 {
		t.Errorf("expected delay 100, got %d", s.Steps[1].Delay)
	}
	if s.Steps[2].Enabled {
		t.Error("expected step 2 to be disabled")
	}
}

func TestMigrateCommandsToSteps_SkipIfStepsExist(t *testing.T) {
	s := &Script{
		Steps: []ScriptStep{{Command: "existing", Enabled: true}},
		Commands: []ScriptCommand{
			{RecordedCommand: recorderRecordedCommand(0, "old command"), Enabled: true},
		},
	}

	s.MigrateCommandsToSteps()

	if len(s.Steps) != 1 {
		t.Fatalf("expected 1 step (existing), got %d", len(s.Steps))
	}
	if s.Steps[0].Command != "existing" {
		t.Errorf("expected existing step preserved, got '%s'", s.Steps[0].Command)
	}
}

func TestMigrateCommandsToSteps_EmptyCommands(t *testing.T) {
	s := &Script{
		Commands: []ScriptCommand{},
	}

	s.MigrateCommandsToSteps()

	if len(s.Steps) != 0 {
		t.Errorf("expected 0 steps for empty commands, got %d", len(s.Steps))
	}
}

// --- SyncStepsToCommands tests ---

func TestSyncStepsToCommands(t *testing.T) {
	s := &Script{
		Steps: []ScriptStep{
			{Command: "ls", Comment: "list", Delay: 0, Enabled: true},
			{Command: "pwd", Enabled: true},
			{Command: "whoami", Enabled: false},
		},
	}

	s.SyncStepsToCommands()

	if len(s.Commands) != 3 {
		t.Fatalf("expected 3 commands, got %d", len(s.Commands))
	}

	if s.Commands[0].Content != "ls" {
		t.Errorf("expected 'ls', got '%s'", s.Commands[0].Content)
	}
	if s.Commands[0].Comment != "list" {
		t.Errorf("expected comment 'list', got '%s'", s.Commands[0].Comment)
	}
	if s.Commands[0].Index != 0 {
		t.Errorf("expected index 0, got %d", s.Commands[0].Index)
	}
	if s.Commands[1].Index != 1 {
		t.Errorf("expected index 1, got %d", s.Commands[1].Index)
	}
	if s.Commands[2].Index != 2 {
		t.Errorf("expected index 2, got %d", s.Commands[2].Index)
	}
}

func TestSyncStepsToCommands_Empty(t *testing.T) {
	s := &Script{
		Steps: []ScriptStep{},
	}
	s.SyncStepsToCommands()
	// Should not panic, Commands should remain nil
	if len(s.Commands) != 0 {
		t.Errorf("expected 0 commands, got %d", len(s.Commands))
	}
}

// --- Export tests ---

func TestExportStepsToBash_Basic(t *testing.T) {
	steps := []ScriptStep{
		{Command: "ls -la", Comment: "list files", Enabled: true},
		{Command: "sleep 1", Delay: 500, Enabled: true},
		{Command: "echo disabled", Enabled: false},
	}

	var sb strings.Builder
	ExportStepsToBash(steps, &sb)

	output := sb.String()
	if !strings.Contains(output, "# list files\nls -la") {
		t.Errorf("expected comment + command, got: %s", output)
	}
	if !strings.Contains(output, "sleep 0.5") {
		t.Errorf("expected sleep 0.5, got: %s", output)
	}
	if !strings.Contains(output, "# echo disabled (disabled)") {
		t.Errorf("expected disabled comment, got: %s", output)
	}
}

// --- Execution tests ---

type mockSender struct {
	commands []string
}

func (m *mockSender) SendCommand(sessionID string, command string) error {
	m.commands = append(m.commands, command)
	return nil
}

func TestExecuteSteps_Basic(t *testing.T) {
	steps := []ScriptStep{
		{Command: "ls", Enabled: true},
		{Command: "pwd", Enabled: true},
	}

	sender := &mockSender{}
	ctx := NewPlaybackContext(nil)

	err := ExecuteSteps(steps, ctx, sender, "test-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sender.commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(sender.commands))
	}
	if sender.commands[0] != "ls\n" {
		t.Errorf("expected 'ls\\n', got '%s'", sender.commands[0])
	}
}

func TestExecuteSteps_Disabled(t *testing.T) {
	steps := []ScriptStep{
		{Command: "ls", Enabled: true},
		{Command: "pwd", Enabled: false},
		{Command: "whoami", Enabled: true},
	}

	sender := &mockSender{}
	ctx := NewPlaybackContext(nil)

	err := ExecuteSteps(steps, ctx, sender, "test-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sender.commands) != 2 {
		t.Fatalf("expected 2 commands (disabled skipped), got %d", len(sender.commands))
	}
}

func TestExecuteSteps_VariableSubstitution(t *testing.T) {
	steps := []ScriptStep{
		{Command: "systemctl restart ${service}", Enabled: true},
	}

	sender := &mockSender{}
	ctx := NewPlaybackContext(map[string]string{
		"service": "nginx",
	})

	err := ExecuteSteps(steps, ctx, sender, "test-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sender.commands[0] != "systemctl restart nginx\n" {
		t.Errorf("expected 'systemctl restart nginx\\n', got '%s'", sender.commands[0])
	}
}

// --- Helper ---

func recorderRecordedCommand(index int, content string) recorder.RecordedCommand {
	return recorder.RecordedCommand{Index: index, Content: content}
}
