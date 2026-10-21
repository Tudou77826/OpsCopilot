//go:build linux

// This disposable lab bridge runs a genuine Linux PTY and interactive shell.
// It is not a remote installation or a replacement SSH client.
package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("expected an absolute lab shell path")
	}
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return err
	}
	defer master.Close()
	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		return err
	}
	n, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		return err
	}
	slave, err := os.OpenFile("/dev/pts/"+strconv.Itoa(n), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return err
	}
	defer slave.Close()
	if err := unix.IoctlSetWinsize(int(master.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Row: 30, Col: 180}); err != nil {
		return err
	}
	cmd := exec.Command(os.Args[1], "-i")
	cmd.Env = append(os.Environ(), "TERM=xterm", "PS1=LAB> ", "ENV=/dev/null", "HISTFILE=/dev/null")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		return err
	}
	slave.Close()
	defer func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }()
	// Parent death (SSH closure) kills the shell group; normal shutdown drains
	// the master first so final output cannot be mistaken for discarded bytes.
	inputDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(master, os.Stdin)
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGHUP)
		close(inputDone)
	}()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sig)
	go func() {
		select {
		case <-sig:
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGHUP)
		case <-inputDone:
		}
	}()
	_, _ = io.Copy(os.Stdout, master)
	_ = cmd.Wait()
	return nil
}
