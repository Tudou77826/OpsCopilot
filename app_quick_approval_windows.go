//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"opscopilot/pkg/assistantcontrol"
)

var quickUser32 = syscall.NewLazyDLL("user32.dll")
var quickMessageBox = quickUser32.NewProc("MessageBoxW")
var quickForeground = quickUser32.NewProc("GetForegroundWindow")
var quickWindowPID = quickUser32.NewProc("GetWindowThreadProcessId")
var quickFindWindow = quickUser32.NewProc("FindWindowW")
var quickPostMessage = quickUser32.NewProc("PostMessageW")

const quickApprovalTitle = "OpsCopilot · 快速审批"

func (a *App) startQuickApproval(ctx context.Context) {
	go a.watchQuickApprovals(ctx)
}

func quickAppIsForeground() bool {
	window, _, _ := quickForeground.Call()
	if window == 0 {
		return false
	}
	var pid uint32
	quickWindowPID.Call(window, uintptr(unsafe.Pointer(&pid)))
	return pid == uint32(os.Getpid())
}

func (a *App) watchQuickApprovals(ctx context.Context) {
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	var activeID string
	var lastPresented string
	var cancel context.CancelFunc
	finished := make(chan string, 1)
	defer func() {
		if cancel != nil {
			cancel()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-finished:
			if id == activeID {
				activeID, cancel = "", nil
			}
		case <-ticker.C:
			state := a.assistantCoordinator().Snapshot()
			if state.Prompt == nil || state.Prompt.ID != lastPresented {
				lastPresented = ""
			}
			if activeID != "" {
				if state.Phase != "waiting" || state.Prompt == nil || state.Prompt.ID != activeID {
					cancel()
				}
				continue
			}
			if state.Phase != "waiting" || state.Prompt == nil || state.Prompt.Kind != "approval" || state.Prompt.ID == lastPresented || quickAppIsForeground() {
				continue
			}
			activeID = state.Prompt.ID
			lastPresented = activeID
			popupCtx, popupCancel := context.WithCancel(ctx)
			cancel = popupCancel
			prompt, turnID := *state.Prompt, state.TurnID
			go func() {
				defer func() { finished <- prompt.ID }()
				choice := showQuickApproval(popupCtx, prompt)
				if choice != "" {
					_ = a.assistantCoordinator().Respond(turnID, prompt.ID, assistantcontrol.Answer{Selected: []string{choice}})
				}
			}()
		}
	}
}

func showQuickApproval(ctx context.Context, p assistantcontrol.Prompt) string {
	if ctx.Err() != nil {
		return ""
	}
	var content strings.Builder
	content.WriteString(p.Title)
	if p.Description != "" {
		content.WriteString("\n\n" + p.Description)
	}
	for _, detail := range p.Details {
		fmt.Fprintf(&content, "\n%s：%s", detail.Label, detail.Value)
	}
	if p.Command != "" {
		content.WriteString("\n\n" + p.Command)
	}
	content.WriteString("\n\n选择“是”允许本次，选择“否”拒绝。")
	message, err := syscall.UTF16PtrFromString(content.String())
	if err != nil {
		return ""
	}
	title, _ := syscall.UTF16PtrFromString(quickApprovalTitle)
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			// A closed/stopped prompt must not leave an actionable stale dialog.
			for i := 0; i < 20; i++ {
				window, _, _ := quickFindWindow.Call(0, uintptr(unsafe.Pointer(title)))
				if window != 0 {
					var pid uint32
					quickWindowPID.Call(window, uintptr(unsafe.Pointer(&pid)))
					if pid == uint32(os.Getpid()) {
						// A Yes/No message box does not accept WM_CLOSE; select No
						// internally, then discard it because the context is cancelled.
						quickPostMessage.Call(window, 0x0111, 7, 0)
						return
					}
				}
				select {
				case <-done:
					return
				case <-time.After(50 * time.Millisecond):
				}
			}
		case <-done:
		}
	}()
	// Yes/No, default to No, warning icon, foreground and topmost.
	result, _, _ := quickMessageBox.Call(0, uintptr(unsafe.Pointer(message)), uintptr(unsafe.Pointer(title)), 0x00000004|0x00000030|0x00000100|0x00010000|0x00040000)
	close(done)
	if ctx.Err() != nil {
		return ""
	}
	switch result {
	case 6:
		return "allow"
	case 7:
		return "reject"
	}
	return ""
}
