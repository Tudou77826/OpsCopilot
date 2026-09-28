package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"opscopilot/internal/plugincontract"
	"opscopilot/internal/shellsidecar"
	"opscopilot/pkg/installguard"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"opscopilot/pkg/updater"
)

//go:embed all:frontend/dist
var assets embed.FS

// Retained in official binaries so hosts can reject old executables without
// executing an unsupported flag (which used to launch the desktop UI).
const teamsPluginMarker = plugincontract.Marker

var desktopInstallationLease *installguard.Lease

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "--plugin-info" {
		_ = json.NewEncoder(os.Stdout).Encode(plugincontract.Info(Version))
		return
	}
	if len(os.Args) >= 2 && os.Args[1] == "--teams-plugin" {
		exe, err := os.Executable()
		if err == nil {
			var lease *installguard.Lease
			lease, err = installguard.AcquireRuntime(filepath.Dir(exe))
			if err == nil {
				defer lease.Close()
			}
		}
		if err == nil {
			err = os.Chdir(filepath.Dir(exe))
		}
		if err == nil {
			err = shellsidecar.Run(Version, os.Args[2:], filepath.Dir(exe))
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

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
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	desktopInstallationLease, err = installguard.AcquireRuntime(filepath.Dir(exe))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	defer desktopInstallationLease.Close()
	// CLI 模式：带子命令时进入命令行入口，不启动 GUI
	// 子命令包括 exec / diagnose / file / knowledge；不带子命令则正常启动图形界面
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "exec", "diagnose", "file", "knowledge", "-h", "--help", "help":
			os.Exit(runCLI(os.Args[1:]))
		default:
			// 未识别参数不能落入 GUI 启动：脚本/agent 调用写错子命令时会表现为
			// 无输出且永不退出（调用方只能等超时）。打印用法并报错退出。
			fmt.Fprintf(os.Stderr, "未知参数或子命令: %s\n\n", os.Args[1])
			printCLIUsage()
			os.Exit(1)
		}
	}

	// Installed and portable desktop entry points share the exe-relative defaults.
	if exe, err := os.Executable(); err == nil {
		if err = os.Chdir(filepath.Dir(exe)); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
	}
	app := NewApp()

	err = wails.Run(&options.App{
		Title:  "OpsCopilot",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnBeforeClose:    app.beforeClose,
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop:     true,
			DisableWebViewDrop: true,
		},
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
