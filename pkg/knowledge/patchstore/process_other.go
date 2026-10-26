//go:build !windows

package patchstore

import "os/exec"

func hideWindow(*exec.Cmd) {}
