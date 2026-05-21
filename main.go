package main

import (
	"context"
	"embed"
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"opscopilot/pkg/updater"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Self-update mode: launched by the parent process to apply an update.
	// This runs before any Wails/UI initialization.
	if len(os.Args) >= 3 && os.Args[1] == "--self-update" {
		if err := runSelfUpdate(os.Args[2]); err != nil {
			fmt.Fprintf(os.Stderr, "self-update failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Normal application startup.
	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "OpsCopilot",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnBeforeClose:    app.beforeClose,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}

// runSelfUpdate is the Phase 2 entry point for applying an update.
func runSelfUpdate(manifestPath string) error {
	m, err := updater.ReadManifest(manifestPath)
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}

	return updater.SelfUpdate(context.Background(), m, updater.OSFS{}, updater.WindowsProcessWaiter{})
}
