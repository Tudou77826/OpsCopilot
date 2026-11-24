package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"opscopilot/pkg/assistantcontrol"
)

// TestRetryChild simulates an agent that cold-start flakiness kills once:
// the first launch exits before the handshake (and leaves a marker), the
// second launch answers initialize. In the parent test process the guard
// variable is absent, so this returns immediately.
func TestRetryChild(t *testing.T) {
	if os.Getenv("OPSCOPILOT_ACP_RETRY_CHILD") != "1" {
		return
	}
	marker := os.Getenv("OPSCOPILOT_ACP_RETRY_MARKER")
	if marker == "" {
		os.Exit(7)
	}
	if _, err := os.Stat(marker); err == nil {
		scanner := bufio.NewScanner(os.Stdin)
		if !scanner.Scan() {
			os.Exit(7)
		}
		var request struct {
			ID     json.Number `json:"id"`
			Method string      `json:"method"`
		}
		if json.Unmarshal(scanner.Bytes(), &request) != nil || request.Method != "initialize" {
			os.Exit(7)
		}
		fmt.Printf("{\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":{\"protocolVersion\":1,\"agentInfo\":{\"name\":\"retry-child\",\"version\":\"0.0.1\"}}}\n", request.ID)
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	if err := os.WriteFile(marker, []byte("attempted"), 0600); err != nil {
		os.Exit(7)
	}
	os.Exit(7)
}

func TestLaunchStandardAgentRetriesOnceAfterColdStartFailure(t *testing.T) {
	marker := t.TempDir() + "/retry-marker"
	if err := os.Setenv("OPSCOPILOT_ACP_RETRY_MARKER", marker); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c := &Conversation{}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	defer func() {
		c.Close()
		_ = os.Unsetenv("OPSCOPILOT_ACP_RETRY_MARKER")
	}()
	process := ProcessConfig{Executable: executable, Args: []string{"-test.run=^TestRetryChild$"}, Directory: t.TempDir(), Environment: append(os.Environ(), "OPSCOPILOT_ACP_RETRY_CHILD=1")}
	client, capabilities, err := c.launchStandardAgent(ctx, ctx, "Agent", process)
	if err != nil {
		t.Fatalf("retry did not absorb the first failed handshake: %v", err)
	}
	defer client.Close()
	if capabilities.ProtocolVersion != 1 || capabilities.AgentInfo.Name != "retry-child" {
		t.Fatalf("unexpected handshake result: %+v", capabilities)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("first attempt never ran: %v", err)
	}
	if c.client != client {
		t.Fatal("conversation is not bound to the successful client")
	}
}

func TestLaunchStandardAgentSurfacesDiagnosticsWhenBothAttemptsFail(t *testing.T) {
	// Empty marker: the child cannot record an attempt and exits before the
	// handshake on every try, which is the "deterministically broken agent" case.
	if err := os.Setenv("OPSCOPILOT_ACP_RETRY_MARKER", ""); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Unsetenv("OPSCOPILOT_ACP_RETRY_MARKER")
	}()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c := &Conversation{}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	process := ProcessConfig{Executable: executable, Args: []string{"-test.run=^TestRetryChild$"}, Directory: t.TempDir(), Environment: append(os.Environ(), "OPSCOPILOT_ACP_RETRY_CHILD=1")}
	_, _, err = c.launchStandardAgent(ctx, ctx, "OpenCode", process)
	if err == nil {
		t.Fatal("expected failure after both attempts")
	}
	message := err.Error()
	if !strings.Contains(message, "已自动重试一次") || !strings.Contains(message, "退出码") {
		t.Fatalf("failure message lacks retry note or diagnostics: %s", message)
	}
}

// TestAnnounceChild emulates CodeAgent's ACP surface: the service address it
// prints to stdout at startup, then initialize and session/new. In the parent
// test process the guard variable is absent, so this returns immediately.
func TestAnnounceChild(t *testing.T) {
	if os.Getenv("OPSCOPILOT_ACP_ANNOUNCE_CHILD") != "1" {
		return
	}
	// A launcher that loses argument boundaries must fail the handshake, not
	// quietly start a differently configured agent.
	if expected := os.Getenv("OPSCOPILOT_ACP_ANNOUNCE_ARG"); expected != "" && !slices.Contains(os.Args, expected) {
		os.Exit(9)
	}
	fmt.Println("[ACP] Server started: http://127.0.0.1:7725/")
	out := bufio.NewWriter(os.Stdout)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 64<<10), maxMessageBytes)
	for scanner.Scan() {
		var request struct {
			ID     json.Number `json:"id"`
			Method string      `json:"method"`
		}
		if json.Unmarshal(scanner.Bytes(), &request) != nil || request.Method == "" {
			continue
		}
		var reply string
		switch request.Method {
		case "initialize":
			reply = `{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":1,"agentCapabilities":{},"agentInfo":{"name":"announce-child","version":"0.0.1"}}}`
		case "session/new":
			reply = `{"jsonrpc":"2.0","id":%s,"result":{"sessionId":"ses_announce"}}`
		case "session/prompt":
			reply = `{"jsonrpc":"2.0","id":%s,"result":{"stopReason":"end_turn"}}`
		default:
			continue
		}
		fmt.Fprintf(out, reply+"\n", request.ID)
		out.Flush()
	}
	os.Exit(0)
}

func TestStandardSessionSurvivesStartupAnnouncement(t *testing.T) {
	if err := os.Setenv("OPSCOPILOT_ACP_ANNOUNCE_CHILD", "1"); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Unsetenv("OPSCOPILOT_ACP_ANNOUNCE_CHILD") }()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"-test.run=^TestAnnounceChild$"}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c := &Conversation{}
	defer c.Close()
	config := ManagedConfig{AgentKind: "codeagent", Executable: executable, ArgumentsJSON: `["-test.run=^TestAnnounceChild$"]`}
	if err := c.Prepare(ctx, ctx, config); err != nil {
		t.Fatalf("startup announcement broke the session: %v", err)
	}
	if c.sessionID != "ses_announce" {
		t.Fatalf("session was not created: %q", c.sessionID)
	}
	if _, err := ProbeStandardAgent(ctx, executable, args); err != nil {
		t.Fatalf("startup announcement broke the runtime check: %v", err)
	}
}

func TestStandardAgentLaterTurnSkipsICodeProfile(t *testing.T) {
	if err := os.Setenv("OPSCOPILOT_ACP_ANNOUNCE_CHILD", "1"); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Unsetenv("OPSCOPILOT_ACP_ANNOUNCE_CHILD") }()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	coordinator := &assistantcontrol.Coordinator{}
	c := &Conversation{}
	defer c.Close()
	config := ManagedConfig{AgentKind: "codeagent", Executable: executable, ArgumentsJSON: `["-test.run=^TestAnnounceChild$"]`, SkillsDirectory: filepath.Join(t.TempDir(), "skills")}
	// The model picker prepares the session before the first message, so the
	// very first turn already finds a running client.
	if err := c.Prepare(ctx, ctx, config); err != nil {
		t.Fatal(err)
	}
	for _, prompt := range []string{"first", "second"} {
		turn, err := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		turn.SetApprovalMode("auto")
		err = c.Run(ctx, config, turn, &managedFixtureTools{}, prompt)
		turn.Finish("completed", "")
		if err != nil {
			t.Fatalf("turn %q failed: %v", prompt, err)
		}
	}
}
