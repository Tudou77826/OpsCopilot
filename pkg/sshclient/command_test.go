package sshclient

import (
	"context"
	"strings"
	"testing"
	"time"

	"opscopilot/internal/sshtest"
	"opscopilot/pkg/remote"
)

func commandFixture(t *testing.T, options sshtest.CommandOptions) (*Client, *sshtest.CommandServer) {
	t.Helper()
	server := sshtest.NewCommandServer(t, options)
	client, err := NewClient(server.Config)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client, server
}

func pythonCommand(script string) string {
	// internal/sshtest validates this command shape and runs it with Python.
	return "exec python3 -u -c '" + strings.ReplaceAll(script, "'", "'\\''") + "' a b c d"
}

func TestRunCommandCapturesStdoutStderrAndNonZeroExitOnce(t *testing.T) {
	client, server := commandFixture(t, sshtest.CommandOptions{})
	request := remote.CommandRequest{Command: pythonCommand("import sys;sys.stdout.write('stdout-fixture');sys.stderr.write('stderr-fixture');sys.exit(7)")}
	result, err := client.RunCommand(context.Background(), request)
	if err == nil {
		t.Fatal("RunCommand returned nil error for non-zero exit")
	}
	if !result.Started || result.ExitCode == nil || *result.ExitCode == 0 || !result.CompletionKnown {
		t.Fatalf("result = %#v, want started non-zero exit with known completion", result)
	}
	if result.Stdout != "stdout-fixture" || result.Stderr != "stderr-fixture" {
		t.Fatalf("captured output stdout=%q stderr=%q", result.Stdout, result.Stderr)
	}
	if result.StdoutBytes != int64(len(result.Stdout)) || result.StderrBytes != int64(len(result.Stderr)) {
		t.Fatalf("captured byte counts = %d/%d", result.StdoutBytes, result.StderrBytes)
	}
	if got := server.Commands.Load(); got != 1 {
		t.Fatalf("fixture executed %d commands, want exactly one", got)
	}
}

func TestRunCommandTimeoutCancelsStalledSession(t *testing.T) {
	client, _ := commandFixture(t, sshtest.CommandOptions{Stall: true})
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	result, err := client.RunCommand(ctx, remote.CommandRequest{Command: pythonCommand("import time;time.sleep(60)")})
	if err == nil || ctx.Err() == nil {
		t.Fatalf("RunCommand err=%v ctx=%v, want timeout", err, ctx.Err())
	}
	if result.StdoutBytes != 0 || result.StderrBytes != 0 {
		t.Fatalf("stalled command produced output: %#v", result)
	}
}

func TestRunCommandKeepsBoundedHeadAndTailAndMarksTruncated(t *testing.T) {
	client, _ := commandFixture(t, sshtest.CommandOptions{})
	request := remote.CommandRequest{
		Command:      pythonCommand("import sys;sys.stdout.write('HEAD-'+'x'*1800+'-TAIL')"),
		CaptureBytes: 1024,
	}
	result, err := client.RunCommand(context.Background(), request)
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	if !result.Truncated || result.StdoutBytes <= int64(len(result.Stdout)) {
		t.Fatalf("result = %#v, want truncated bounded output", result)
	}
	if !strings.HasPrefix(result.Stdout, "HEAD-") || !strings.Contains(result.Stdout, "-TAIL") {
		t.Fatalf("head/tail not retained: %q", result.Stdout)
	}
}
