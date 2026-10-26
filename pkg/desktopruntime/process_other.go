//go:build !windows

package desktopruntime

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}
