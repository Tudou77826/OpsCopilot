package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"opscopilot/pkg/desktopruntime"
	"opscopilot/pkg/updater"
)

type DesktopRuntimeStatus struct {
	Phase   string `json:"phase"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

func resolveDesktopJDBCRuntime() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return desktopruntime.Resolve(filepath.Dir(exe))
}

var desktopRuntimeState = struct {
	sync.RWMutex
	status  DesktopRuntimeStatus
	running bool
}{status: DesktopRuntimeStatus{Phase: "idle"}}

// GetDesktopRuntimeStatus also works when startup events preceded UI mounting.
func (a *App) GetDesktopRuntimeStatus() DesktopRuntimeStatus {
	desktopRuntimeState.RLock()
	defer desktopRuntimeState.RUnlock()
	return desktopRuntimeState.status
}

// RetryDesktopRuntime uses the same local payload; no Java download is needed.
func (a *App) RetryDesktopRuntime() { a.prepareDesktopRuntime(a.ctx) }

func (a *App) prepareDesktopRuntime(ctx context.Context) {
	desktopRuntimeState.Lock()
	if desktopRuntimeState.running {
		desktopRuntimeState.Unlock()
		return
	}
	desktopRuntimeState.running = true
	desktopRuntimeState.Unlock()
	go func() {
		defer func() { desktopRuntimeState.Lock(); desktopRuntimeState.running = false; desktopRuntimeState.Unlock() }()
		set := func(phase, message, detail string) {
			s := DesktopRuntimeStatus{Phase: phase, Message: message, Detail: detail}
			desktopRuntimeState.Lock()
			desktopRuntimeState.status = s
			desktopRuntimeState.Unlock()
			runtime.EventsEmit(ctx, "desktop-runtime-status", s)
		}
		exe, err := os.Executable()
		if err == nil && desktopruntime.ExpectedSHA256 != "" {
			err = desktopruntime.Ensure(ctx, filepath.Dir(exe), desktopruntime.ExpectedSHA256, func(message string) { set("preparing", message, "") })
		}
		if err != nil {
			slog.Error("desktop JDBC runtime preparation failed", "error", err)
			set("error", "数据库组件准备失败；其他功能仍可使用", err.Error())
			// Keep updater backups and result for a successful retry.
			return
		}
		phase := "ready"
		message := "数据库组件已就绪"
		if desktopruntime.ExpectedSHA256 == "" {
			phase = "unmanaged"
			message = ""
		}
		set(phase, message, "")
		if a.serviceCenter != nil {
			go a.serviceCenter.Refresh(ctx)
		}
		if result, err := updater.CleanupAfterUpdate(filepath.Dir(exe), updater.OSFS{}); err != nil {
			slog.Warn("post-update cleanup failed", "error", err)
		} else if result != nil {
			runtime.EventsEmit(ctx, "update-completed", result)
		}
	}()
}
