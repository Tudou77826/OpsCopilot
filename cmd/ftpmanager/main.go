package main

import (
	"embed"
	"os"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:static
var assets embed.FS

func main() {
	// Parse command line flags
	ipcTokenFile := ""
	sessionID := ""
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--ipc-token-file=") {
			ipcTokenFile = strings.TrimPrefix(arg, "--ipc-token-file=")
		} else if strings.HasPrefix(arg, "--session=") {
			sessionID = strings.TrimPrefix(arg, "--session=")
		}
	}

	// Create FTP manager app
	app := NewFTPApp(ipcTokenFile, sessionID)

	// Start Wails
	err := wails.Run(&options.App{
		Title:  "OpsCopilot 文件管理器",
		Width:     1024,
		Height:    768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 32, G: 38, B: 54, A: 1},
		OnStartup: app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
