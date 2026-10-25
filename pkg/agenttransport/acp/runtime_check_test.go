package acp

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestInspectRuntimeUsesManagedStartupWithoutPrompt(t *testing.T) {
	config := testConversationConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	report := InspectRuntime(ctx, config.Executable)
	if !report.Ready || report.ProtocolVersion != 1 || len(report.Checks) != 6 {
		t.Fatal(report)
	}
	for _, check := range report.Checks {
		if check.Status != "passed" {
			t.Fatal(check)
		}
	}
	// The test peer only responds to startup calls and prompts. No prompt is
	// necessary for readiness, and this path has no user model configuration.
}

func TestInspectRuntimeReportsLaunchFailureAndSkipsLaterStages(t *testing.T) {
	report := InspectRuntime(context.Background(), filepath.Join(t.TempDir(), "missing.exe"))
	if report.Ready || report.Checks[1].Status != "failed" || report.Checks[2].Status != "skipped" {
		t.Fatal(report)
	}
	if !strings.Contains(report.Checks[1].Detail, "无法启动") {
		t.Fatal(report)
	}
}

func TestInspectRuntimeIdentifiesIncompatibleRuntimeAfterHandshake(t *testing.T) {
	config := testConversationConfig(t)
	t.Setenv("ACP_CONVERSATION_INCOMPATIBLE", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	report := InspectRuntime(ctx, config.Executable)
	if report.Ready || report.Version != "test" || report.Checks[2].Status != "passed" || report.Checks[3].Status != "failed" || report.Checks[4].Status != "skipped" {
		t.Fatal(report)
	}
}

func TestLiveICodeRuntimeInspection(t *testing.T) {
	executable := os.Getenv("OPSCOPILOT_CHRYS_TEST_EXE")
	if executable == "" {
		t.Skip("opt-in local iCode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	report := InspectRuntime(ctx, executable)
	if !report.Ready {
		t.Fatalf("runtime check: %+v", report)
	}
	t.Logf("runtime=%s version=%s protocol=%d extension=%d checks=%d", report.Name, report.Version, report.ProtocolVersion, report.ExtensionAPIVersion, len(report.Checks))
}

func TestLiveICodeProfileWritesRemainInsideIsolatedConfig(t *testing.T) {
	executable := os.Getenv("OPSCOPILOT_CHRYS_TEST_EXE")
	if executable == "" {
		t.Skip("opt-in installed or source iCode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c := &Conversation{}
	defer c.Close()
	if err := c.start(ctx, ctx, ManagedConfig{Executable: executable, BaseURL: "http://127.0.0.1:1/v1", Model: "runtime-check"}); err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Path string `json:"path"`
	}
	profile := map[string]any{"id": "isolation-check", "name": "Isolation check", "provider": "mock", "model_id": "fixture"}
	if err := c.client.callExtension(ctx, "profiles/models/write", map[string]any{"profile": profile}, &saved); err != nil {
		t.Fatal(err)
	}
	configRoot := filepath.Join(c.directory, "home", ".chrys")
	if runtime.GOOS == "windows" {
		configRoot = filepath.Join(c.directory, "appdata", "chrys")
	}
	relative, err := filepath.Rel(configRoot, saved.Path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		t.Fatalf("profile escaped isolated config: %q (%v)", relative, err)
	}
	if _, err = os.Stat(saved.Path); err != nil {
		t.Fatal("profile was not written", err)
	}
}
