package ai

import (
	"encoding/json"
	"testing"
)

func TestValidateNoRetrievedContent(t *testing.T) {
	input := `{"steps":[{"step":1,"title":"test"}],"commands":[{"command":"ls -la","description":"list files"}],"summary":"test"}`

	result := validateTroubleshootResponse(input, nil)

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}

	commands, ok := resp["commands"].([]interface{})
	if !ok {
		t.Fatal("commands should be an array")
	}
	if len(commands) != 0 {
		t.Errorf("expected 0 commands when no content retrieved, got %d", len(commands))
	}

	steps, ok := resp["steps"].([]interface{})
	if !ok {
		t.Fatal("steps should be an array")
	}
	if len(steps) != 0 {
		t.Errorf("expected 0 steps when no content retrieved, got %d", len(steps))
	}
}

func TestValidateEmptyRetrievedContent(t *testing.T) {
	input := `{"steps":[{"step":1,"title":"test"}],"commands":[{"command":"ls -la","description":"list"}],"summary":"test"}`
	rc := NewRetrievedContent() // empty, no lines recorded

	result := validateTroubleshootResponse(input, rc)

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}

	commands := resp["commands"].([]interface{})
	if len(commands) != 0 {
		t.Errorf("expected 0 commands with empty retrieved, got %d", len(commands))
	}
}

func TestValidateCommandsSourced(t *testing.T) {
	input := `{"steps":[],"commands":[{"command":"systemctl status nginx","description":"check nginx","source":"nginx.md#L42"}],"summary":"found"}`
	rc := NewRetrievedContent()
	rc.FilesRead["nginx.md"] = true

	result := validateTroubleshootResponse(input, rc)

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}

	commands := resp["commands"].([]interface{})
	if len(commands) != 1 {
		t.Fatalf("expected 1 command with valid source, got %d", len(commands))
	}
}

func TestValidateCommandsUngrounded(t *testing.T) {
	input := `{"steps":[],"commands":[{"command":"rm -rf /","description":"dangerous","risk":"High"}],"summary":"test"}`
	rc := NewRetrievedContent()
	rc.Lines["somefile.md:1"] = "safe diagnostic command"
	rc.FilesRead["somefile.md"] = true

	result := validateTroubleshootResponse(input, rc)

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}

	commandsRaw := resp["commands"]
	if commandsRaw == nil {
		// nil is acceptable (means empty)
		return
	}
	commands, ok := commandsRaw.([]interface{})
	if !ok || len(commands) != 0 {
		t.Errorf("expected 0 commands (ungrounded should be stripped), got %v", commandsRaw)
	}
}

func TestValidateCommandsFuzzyMatch(t *testing.T) {
	// Two-token match: "ping -c" must appear in retrieved content
	input := `{"steps":[],"commands":[{"command":"ping -c 4 192.168.1.1","description":"check connectivity","risk":"Low"}],"summary":"test"}`
	rc := NewRetrievedContent()
	rc.Lines["network.md:5"] = "使用 ping -c 检查网络连通性"
	rc.FilesRead["network.md"] = true

	result := validateTroubleshootResponse(input, rc)

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}

	commands := resp["commands"].([]interface{})
	if len(commands) != 1 {
		t.Errorf("expected 1 command (two-token match on 'ping -c'), got %d", len(commands))
	}
}

func TestValidateCommandsFuzzyNoMatch(t *testing.T) {
	// "rm -rf" does not appear in doc, should be stripped even though "rm" might
	input := `{"steps":[],"commands":[{"command":"rm -rf /tmp/test","description":"cleanup","risk":"High"}],"summary":"test"}`
	rc := NewRetrievedContent()
	rc.Lines["network.md:5"] = "use ping -c to test connectivity and rm old logs"
	rc.FilesRead["network.md"] = true

	result := validateTroubleshootResponse(input, rc)

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}

	commandsRaw := resp["commands"]
	if commandsRaw == nil {
		return // stripped correctly
	}
	commands, ok := commandsRaw.([]interface{})
	if !ok || len(commands) != 0 {
		t.Errorf("expected 0 commands (rm -rf not in doc), got %v", commandsRaw)
	}
}

func TestValidateMixedCommands(t *testing.T) {
	input := `{"steps":[],"commands":[
		{"command":"ping -c 4 <HOST>","description":"check network","risk":"Low"},
		{"command":"rm -rf /","description":"dangerous","risk":"High"},
		{"command":"systemctl status nginx","description":"check nginx","source":"ops.md#L10"}
	],"summary":"test"}`
	rc := NewRetrievedContent()
	rc.Lines["network.md:3"] = "ping -c 4 192.168.1.1"
	rc.FilesRead["network.md"] = true
	rc.FilesRead["ops.md"] = true

	result := validateTroubleshootResponse(input, rc)

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}

	commands := resp["commands"].([]interface{})
	if len(commands) != 2 {
		t.Errorf("expected 2 commands (ping by fuzzy, systemctl by source, rm stripped), got %d", len(commands))
	}
}
