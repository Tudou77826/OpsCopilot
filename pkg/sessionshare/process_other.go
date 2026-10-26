//go:build !windows

package sessionshare

import "os/exec"

func hideWindow(*exec.Cmd) {}
