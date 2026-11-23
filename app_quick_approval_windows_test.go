//go:build windows

package main

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"opscopilot/pkg/assistantcontrol"
)

func TestQuickApprovalNativeRejectAndCancellation(t *testing.T) {
	if os.Getenv("OPSCOPILOT_TEST_NATIVE_APPROVAL") != "1" {
		t.Skip("opt-in native desktop dialog")
	}
	title, _ := syscall.UTF16PtrFromString(quickApprovalTitle)
	if window, _, _ := quickFindWindow.Call(0, uintptr(unsafe.Pointer(title))); window != 0 {
		t.Skip("another approval dialog is open")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		for ctx.Err() == nil {
			window, _, _ := quickFindWindow.Call(0, uintptr(unsafe.Pointer(title)))
			if window != 0 {
				var pid uint32
				quickWindowPID.Call(window, uintptr(unsafe.Pointer(&pid)))
				if pid == uint32(os.Getpid()) {
					quickPostMessage.Call(window, 0x0111, 7, 0)
					return
				}
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()
	if got := showQuickApproval(ctx, assistantcontrol.Prompt{Title: "测试审批", Command: "echo test"}); got != "reject" {
		t.Fatalf("unexpected choice %q", got)
	}
	go func() {
		for ctx.Err() == nil {
			window, _, _ := quickFindWindow.Call(0, uintptr(unsafe.Pointer(title)))
			if window != 0 {
				var pid uint32
				quickWindowPID.Call(window, uintptr(unsafe.Pointer(&pid)))
				if pid == uint32(os.Getpid()) {
					quickPostMessage.Call(window, 0x0111, 6, 0)
					return
				}
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()
	if got := showQuickApproval(ctx, assistantcontrol.Prompt{Title: "测试允许", Command: "echo test"}); got != "allow" {
		t.Fatalf("unexpected choice %q", got)
	}
	ctx2, cancel2 := context.WithCancel(context.Background())
	done := make(chan string, 1)
	go func() { done <- showQuickApproval(ctx2, assistantcontrol.Prompt{Title: "测试失效"}) }()
	time.Sleep(100 * time.Millisecond)
	cancel2()
	select {
	case got := <-done:
		if got != "" {
			t.Fatalf("cancelled dialog granted %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled dialog remained open")
	}
}
