package acp

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestStandardProbeChecksProtocolAndSession(t *testing.T) {
	config := testConversationConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	report, err := ProbeStandardAgent(ctx, config.Executable, []string{"acp"})
	if err != nil || report.ProtocolVersion != 1 || !report.HTTPMCP || report.Checks[3].Status != "passed" {
		t.Fatalf("probe = %+v, err = %v", report, err)
	}
}

func TestStandardProbeAcceptsAgentWithoutHTTPMCP(t *testing.T) {
	config := testConversationConfig(t)
	t.Setenv("ACP_CONVERSATION_INCOMPATIBLE", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	report, err := ProbeStandardAgent(ctx, config.Executable, []string{"acp"})
	if err != nil || report.HTTPMCP || report.Checks[2].Status != "skipped" || report.Checks[3].Status != "passed" {
		t.Fatalf("probe = %+v, err = %v", report, err)
	}
}

func TestLiveOpenCodeStandardProbe(t *testing.T) {
	executable := os.Getenv("OPSCOPILOT_OPENCODE_TEST_EXE")
	if executable == "" {
		t.Skip("opt-in OpenCode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	report, err := ProbeStandardAgent(ctx, executable, []string{"acp"})
	if err != nil || report.Name != "OpenCode" {
		t.Fatalf("probe = %+v, err = %v", report, err)
	}
	t.Logf("%s %s ACP %d HTTP MCP %v model options %d", report.Name, report.Version, report.ProtocolVersion, report.HTTPMCP, report.ModelOptions)
}
