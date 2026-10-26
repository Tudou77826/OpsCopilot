package ops

import (
	"strings"
	"testing"

	"opscopilot/pkg/core/security"
	"opscopilot/pkg/remote"
)

func oncePolicy(t *testing.T) *security.WhitelistManager {
	t.Helper()
	policy, err := security.NewWhitelistManager(t.TempDir() + "\\whitelist.json")
	if err != nil {
		t.Fatalf("NewWhitelistManager: %v", err)
	}
	return policy
}

func onceConfig() *remote.ConnectConfig {
	return &remote.ConnectConfig{
		Host:         "10.0.0.8",
		User:         "operator",
		Password:     "login-secret",
		RootPassword: "root-secret",
	}
}

func TestPrepareOnceQuotesDirectoryAndKeepsPasswordsOnStdin(t *testing.T) {
	cfg := onceConfig()
	policy := oncePolicy(t)

	for _, tc := range []struct {
		name, elevation, wantPassword, wantCommand string
	}{
		{"su", "su", cfg.RootPassword, "su - 'target' -c"},
		{"sudo", "sudo", cfg.Password, "sudo -S -p '' -u 'target' -- sh -c"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request, err := PrepareOnce(cfg, "ls -la", OnceOptions{Directory: "/srv/app with space", User: "target", Elevation: tc.elevation}, policy)
			if err != nil {
				t.Fatalf("PrepareOnce: %v", err)
			}
			if request.Input != tc.wantPassword+"\n" {
				t.Fatalf("Input = %q, want password only on stdin", request.Input)
			}
			if strings.Contains(request.Command, tc.wantPassword) {
				t.Fatalf("password leaked into command: %q", request.Command)
			}
			if !strings.Contains(request.Command, QuoteShell("cd '/srv/app with space' && (\nls -la\n)")) || !strings.Contains(request.Command, tc.wantCommand) {
				t.Fatalf("command = %q, missing quoted directory/wrapper", request.Command)
			}
		})
	}
}

func TestPrepareOnceRejectsNonReadOnlyCommand(t *testing.T) {
	_, err := PrepareOnce(onceConfig(), "rm -rf /", OnceOptions{}, oncePolicy(t))
	if err == nil || !strings.Contains(err.Error(), ErrPolicy.Error()) {
		t.Fatalf("PrepareOnce error = %v, want read-only policy rejection", err)
	}
}
