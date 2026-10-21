package acp

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestStartupDiagnosticRecognizesFragmentsWithoutExposingStderr(t *testing.T) {
	d := &startupDiagnostic{}
	for _, part := range []string{"Traceback secret=do-not-display\nModuleNotFoundError: No module named '", "bs4", "'"} {
		_, _ = d.Write([]byte(part))
	}
	got := d.summary()
	if !strings.Contains(got, "bs4") || strings.Contains(got, "secret") || strings.Contains(got, "Traceback") {
		t.Fatal(got)
	}
	unknown := &startupDiagnostic{}
	_, _ = unknown.Write([]byte("password=do-not-display\n" + strings.Repeat("x", 5000) + "ModuleNotFoundError: No module named 'wrong'\n"))
	if unknown.summary() != "" || len(unknown.line) > 4096 {
		t.Fatal("unrecognized/oversized stderr leaked")
	}
}

func TestStartupDependencyChild(t *testing.T) {
	if os.Getenv("OPSCOPILOT_ACP_DEPENDENCY_CHILD") != "1" {
		return
	}
	fmt.Fprintln(os.Stderr, "Traceback secret=must-not-leak")
	fmt.Fprintln(os.Stderr, "ModuleNotFoundError: No module named 'bs4'")
	os.Exit(23)
}

func TestProcessReportsMissingDependencyBeforeHandshake(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := Start(ctx, ProcessConfig{Executable: executable, Args: []string{"-test.run=^TestStartupDependencyChild$"}, Directory: t.TempDir(), Environment: append(os.Environ(), "OPSCOPILOT_ACP_DEPENDENCY_CHILD=1")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err = client.Initialize(ctx); err == nil {
		t.Fatal("broken child completed handshake")
	}
	if err = client.WaitProcess(ctx); err != nil {
		t.Fatal(err)
	}
	if got := client.StartupDiagnostic(); !strings.Contains(got, "bs4") || strings.Contains(got, "secret") {
		t.Fatal(got)
	}
}
