// opscopilot-cli is the headless host. It never imports Wails or embeds assets.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"opscopilot/pkg/opscli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	code := opscli.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
