//go:build !windows

package acp

import "os/exec"

func hideWindow(*exec.Cmd) {}
