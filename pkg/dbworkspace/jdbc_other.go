//go:build !windows

package dbworkspace

import "os/exec"

func hideJDBCWindow(cmd *exec.Cmd) {}
