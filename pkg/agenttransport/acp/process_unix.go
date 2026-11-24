//go:build !windows

package acp

import "os/exec"

func prepareCommand(*exec.Cmd) error { return nil }
