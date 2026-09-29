package main

import (
	"context"
	"opscopilot/pkg/servicecenter"
	"path/filepath"
	"time"
)

// The CLI uses desktop consent, never opens a dialog, and never sends arguments or output.
// Network availability is best effort; it cannot change a command's exit status.
func beginCLIUsage(env cliEnv, prefix string) (func(string), func(string)) {
	c := servicecenter.NewClient(filepath.Join(env.binDir, "service-center.json"), Version)
	settings := c.Settings()
	if settings.Choice != "standard" || settings.NeedsConsent || settings.BaseURL == "" {
		return func(string) {}, func(string) {}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	c.Refresh(ctx)
	cancel()
	done := c.BeginUsage(prefix)
	installation := c.Identity()
	observe := func(kind string) { c.CountForInstallation(kind, installation) }
	return func(outcome string) {
		// Completion needs enough time to recheck consent and persist the outcome
		// under load; keep the CLI's telemetry delay bounded.
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		c.Refresh(ctx) // Renew policy readiness after long running commands.
		done(outcome)
		c.Refresh(ctx)
	}, observe
}

func cliHelpRequested(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}
