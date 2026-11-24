//go:build windows

package main

import (
	"context"
	"time"

	"golang.org/x/sys/windows"
)

var (
	modUser32Watch      = windows.NewLazySystemDLL("user32.dll")
	procForegroundWatch = modUser32Watch.NewProc("GetForegroundWindow")
)

// quickAppIsForeground reports whether the desktop app owns the foreground
// window, in which case the in-app card is already visible to the user.
func quickAppIsForeground() bool {
	window, _, _ := procForegroundWatch.Call()
	if window == 0 {
		return false
	}
	var pid uint32
	windows.GetWindowThreadProcessId(windows.HWND(window), &pid)
	return pid == uint32(windows.GetCurrentProcessId())
}

func (a *App) startQuickApproval(ctx context.Context) {
	go a.watchQuickApprovals(ctx)
}

// watchQuickApprovals tracks waiting approval prompts without ever granting a
// decision: each new prompt surfaces one transient toast, and the taskbar
// badge always shows how many prompts are waiting. The in-app card remains
// the only surface that answers a prompt.
func (a *App) watchQuickApprovals(ctx context.Context) {
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	var announced string
	var badgeCount int
	for {
		select {
		case <-ctx.Done():
			updateApprovalBadge(0)
			return
		case <-ticker.C:
			state := a.assistantCoordinator().Snapshot()
			prompt := state.Prompt
			waiting := 0
			if state.Phase == "waiting" && prompt != nil && prompt.Kind == "approval" {
				waiting = 1
			}
			if waiting != badgeCount && updateApprovalBadge(waiting) {
				badgeCount = waiting
			}
			if waiting == 0 || prompt.ID == announced || quickAppIsForeground() {
				continue
			}
			announced = prompt.ID
			go showApprovalToast(*prompt)
		}
	}
}
