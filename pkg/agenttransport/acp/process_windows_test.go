//go:build windows

package acp

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeBatchLauncher(t *testing.T, executable string) string {
	t.Helper()
	// npm-style shim under a path with spaces, where the /S rule would strip
	// the quotes around the script.
	directory := filepath.Join(t.TempDir(), "with spaces")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(directory, "codeagent.cmd")
	if err := os.WriteFile(script, []byte("@ECHO off\r\n\""+executable+"\" %*\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return script
}

func TestBatchLauncherConnectsUnderSpacedPath(t *testing.T) {
	if err := os.Setenv("OPSCOPILOT_ACP_ANNOUNCE_CHILD", "1"); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Unsetenv("OPSCOPILOT_ACP_ANNOUNCE_CHILD") }()
	const spaced = "argument with spaces"
	if err := os.Setenv("OPSCOPILOT_ACP_ANNOUNCE_ARG", spaced); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Unsetenv("OPSCOPILOT_ACP_ANNOUNCE_ARG") }()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	launcher := writeBatchLauncher(t, executable)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c := &Conversation{}
	defer c.Close()
	// The session path and the runtime check must launch the shim alike. The
	// spaced argument is not last: losing the outer closing quote would merge
	// it with what follows, and the child then refuses the handshake.
	arguments, _ := json.Marshal([]string{"-test.run=TestAnnounceChild", spaced, "tail"})
	config := ManagedConfig{AgentKind: "codeagent", Executable: launcher, ArgumentsJSON: string(arguments)}
	if err := c.Prepare(ctx, ctx, config); err != nil {
		t.Fatalf("batch launcher under a spaced path failed: %v", err)
	}
	if _, err := ProbeStandardAgent(ctx, launcher, []string{"-test.run=TestAnnounceChild", spaced, "tail"}); err != nil {
		t.Fatalf("runtime check rejected the batch launcher: %v", err)
	}
}

func TestPrepareCommandRoutesBatchLaunchersOnly(t *testing.T) {
	launcher := `C:\Program Files\agent\codeagent.cmd`
	command := exec.Command(launcher, "acp")
	if err := prepareCommand(command); err != nil {
		t.Fatal(err)
	}
	expected := `""` + launcher + `" "acp""`
	if command.Path == launcher || command.Args[0] != command.Path || !strings.HasSuffix(command.SysProcAttr.CmdLine, " /d /s /c "+expected) {
		t.Fatalf("batch launcher was not routed through cmd.exe: path=%s args=%v cmdline=%s", command.Path, command.Args, command.SysProcAttr.CmdLine)
	}
	if filepath.Base(command.Path) != "cmd.exe" || !command.SysProcAttr.HideWindow {
		t.Fatalf("unexpected interpreter or visible console: %+v", command.SysProcAttr)
	}
	for _, argument := range []string{`a"b`, "100%", "a!b", "a\r\nb"} {
		if err := prepareCommand(exec.Command(launcher, "acp", argument)); err == nil || !strings.Contains(err.Error(), "不支持的字符") {
			t.Fatalf("argument %q was handed to cmd.exe: %v", argument, err)
		}
	}
	application := `C:\Program Files\agent\agent.exe`
	command = exec.Command(application, "acp", `a"b`)
	if err := prepareCommand(command); err != nil {
		t.Fatalf("ordinary executable was treated as a batch launcher: %v", err)
	}
	if command.Path != application || command.SysProcAttr.CmdLine != "" {
		t.Fatalf("ordinary executable command was rewritten: %+v", command.SysProcAttr)
	}
}
