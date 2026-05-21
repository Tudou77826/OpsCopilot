//go:build !windows

package updater

import (
	"fmt"
	"time"
)

// WindowsProcessWaiter is a no-op waiter for non-Windows platforms.
type WindowsProcessWaiter struct{}

func (WindowsProcessWaiter) Wait(pid int, timeout time.Duration) error {
	return fmt.Errorf("self-update not supported on this platform")
}
