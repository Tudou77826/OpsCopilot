package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"opscopilot/pkg/config"
	"opscopilot/pkg/remote"
	"opscopilot/pkg/session"
)

type testQuickCommandConnection struct{ remote.Connection }

func (testQuickCommandConnection) ConnectionIPs() (string, string) {
	return "192.0.2.10", "198.51.100.20"
}

type testQuickCommandInput struct{ io.Writer }

func (testQuickCommandInput) Close() error { return nil }

func TestQuickCommandTemplateDispatch(t *testing.T) {
	a := &App{configMgr: config.NewManagerWithDir(t.TempDir()), sessionMgr: session.NewManager()}
	if err := a.configMgr.Load(); err != nil {
		t.Fatal(err)
	}
	if err := a.configMgr.AddQuickCommand(config.QuickCommand{
		ID: "template", Content: "{{local_ip}} {{remote_ip}} {{selected_text}} {{date}}", UseVariables: true,
	}); err != nil {
		t.Fatal(err)
	}
	var input bytes.Buffer
	a.sessionMgr.Register(session.Session{
		ID: "tab", ConnectionID: "new-connection", Conn: testQuickCommandConnection{},
		Stdin: testQuickCommandInput{Writer: &input},
	})
	for _, attempt := range []struct{ connection, selection, want string }{
		{"old-connection", "one", "连接已改变"},
		{"new-connection", "", "选中一行文本"},
		{"new-connection", "one\ntwo", "控制字符"},
	} {
		_, err := a.DispatchQuickCommandTemplate("template", "tab", attempt.connection, attempt.selection)
		if err == nil || !strings.Contains(err.Error(), attempt.want) || input.Len() != 0 {
			t.Fatalf("invalid dispatch wrote %q, returned %v", input.String(), err)
		}
	}
	if _, err := a.DispatchQuickCommand("template", []string{"tab"}); err == nil || input.Len() != 0 {
		t.Fatalf("legacy dispatch accepted template: %v", err)
	}
	if _, err := a.DispatchQuickCommandTemplate("template", "tab", "new-connection", "{{date}}"); err != nil {
		t.Fatal(err)
	}
	want := "192.0.2.10 198.51.100.20 {{date}} " + time.Now().Format("2006-01-02")
	if input.String() != want {
		t.Fatalf("terminal received %q, want %q", input.String(), want)
	}
}

func TestInstallQuickCommandExamplesOnce(t *testing.T) {
	dir := t.TempDir()
	a := &App{configMgr: config.NewManagerWithDir(dir)}
	if err := a.configMgr.Load(); err != nil {
		t.Fatal(err)
	}
	if err := a.configMgr.AddQuickCommand(config.QuickCommand{ID: "legacy", Content: "echo {{date}}"}); err != nil {
		t.Fatal(err)
	}
	if err := a.installQuickCommandExamples(); err != nil {
		t.Fatal(err)
	}
	commands := a.LoadQuickCommands()
	if len(commands) != 3 || commands[0].UseVariables || !commands[1].UseVariables || !commands[2].UseVariables {
		t.Fatalf("unexpected shortcuts: %+v", commands)
	}
	if !a.configMgr.DeleteQuickCommand(commands[1].ID) {
		t.Fatal("delete the first example")
	}
	a.configMgr = config.NewManagerWithDir(dir)
	if err := a.configMgr.Load(); err != nil {
		t.Fatal(err)
	}
	if err := a.installQuickCommandExamples(); err != nil {
		t.Fatal(err)
	}
	if got := a.LoadQuickCommands(); len(got) != 2 {
		t.Fatalf("deleted example reappeared: %+v", got)
	}
}
