//go:build windows

package dbworkspace

import (
	"os/exec"
	"syscall"
)

func hideJDBCWindow(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }
