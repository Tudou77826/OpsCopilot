//go:build windows

package updater

import (
	"fmt"
	"time"

	"golang.org/x/sys/windows"
)

const (
	waitObject0 = 0x00000000
	waitTimeout = 0x00000102
)

// WindowsProcessWaiter waits for a process to exit using Windows API.
type WindowsProcessWaiter struct{}

func (WindowsProcessWaiter) Wait(pid int, timeout time.Duration) error {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return pollProcessExit(pid, timeout)
	}
	defer windows.CloseHandle(handle)

	ms := uint32(timeout / time.Millisecond)
	if ms == 0 {
		ms = uint32(timeout.Milliseconds())
	}
	event, err := windows.WaitForSingleObject(handle, ms)
	if err != nil {
		return fmt.Errorf("WaitForSingleObject error: %w", err)
	}
	switch event {
	case waitObject0:
		return nil
	case waitTimeout:
		return fmt.Errorf("timeout waiting for process %d", pid)
	default:
		return fmt.Errorf("WaitForSingleObject returned %d for pid %d", event, pid)
	}
}

// pollProcessExit falls back to polling if OpenProcess fails.
func pollProcessExit(pid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
		if err != nil {
			return nil
		}
		event, _ := windows.WaitForSingleObject(handle, 1000)
		windows.CloseHandle(handle)
		if event == waitObject0 {
			return nil
		}
	}
	return fmt.Errorf("timeout waiting for process %d", pid)
}
