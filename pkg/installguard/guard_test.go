package installguard

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"
)

// Use real cooperating processes, not just multiple handles in one process.
func startRuntimePeer(t *testing.T, root string) func() {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLeaseHelper$")
	cmd.Env = append(os.Environ(), "OPS_INSTALLGUARD_TEST_ROOT="+root)
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			in.Close()
			cancel()
			_ = cmd.Wait()
		}
	})
	scanner := bufio.NewScanner(out)
	if !scanner.Scan() || scanner.Text() != "ready" {
		t.Fatal("runtime peer did not become ready")
	}
	return func() {
		if stopped {
			return
		}
		stopped = true
		in.Close()
		if err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestThreeProcessesMustAllLeaveBeforeUpdate(t *testing.T) {
	root := t.TempDir()
	own, err := AcquireRuntime(root)
	if err != nil {
		t.Fatal(err)
	}
	defer own.Close()
	stopB, stopC := startRuntimePeer(t, root), startRuntimePeer(t, root)
	if !errors.Is(own.CheckUpdate(), ErrBusy) {
		t.Fatal("three processes admitted an update")
	}
	stopB()
	if !errors.Is(own.CheckUpdate(), ErrBusy) {
		t.Fatal("one remaining peer was ignored")
	}
	stopC()
	if err := own.CheckUpdate(); err != nil {
		t.Fatal(err)
	}
	// A preflight is not an update lease: a newly started peer must still block phase two.
	stopLate := startRuntimePeer(t, root)
	own.Close()
	if release, err := AcquireUpdate(root); err == nil {
		release()
		t.Fatal("phase two ignored a peer started after preflight")
	}
	stopLate()
	release, err := AcquireUpdate(root)
	if err != nil {
		t.Fatal(err)
	}
	release()
}

func TestRuntimeProtectionIsScopedToInstallation(t *testing.T) {
	own, err := AcquireRuntime(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer own.Close()
	other, err := AcquireUpdate(t.TempDir())
	if err != nil {
		t.Fatal("independent installation was blocked", err)
	}
	defer other()
	if err := own.CheckUpdate(); err != nil {
		t.Fatal(err)
	}
}

func TestLeaseHelper(t *testing.T) {
	root := os.Getenv("OPS_INSTALLGUARD_TEST_ROOT")
	if root == "" {
		return
	}
	lease, err := AcquireRuntime(root)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	fmt.Println("ready")
	_, _ = io.Copy(io.Discard, os.Stdin)
}

func TestRuntimeAndUpdateExclusion(t *testing.T) {
	root := t.TempDir()
	first, err := AcquireRuntime(root)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if err = first.CheckUpdate(); err != nil {
		t.Fatal(err)
	}
	second, err := AcquireRuntime(root)
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(first.CheckUpdate(), ErrBusy) {
		t.Fatal("other runtime not detected")
	}
	if release, err := AcquireUpdate(root); err == nil {
		release()
		t.Fatal("updated live installation")
	}
	second.Close()
	if err = first.CheckUpdate(); err != nil {
		t.Fatal(err)
	}
	first.Close()
	release, err := AcquireUpdate(root)
	if err != nil {
		t.Fatal(err)
	}
	if lease, err := AcquireRuntime(root); err == nil {
		lease.Close()
		release()
		t.Fatal("started during replacement")
	}
	release()
	release()
	next, err := AcquireRuntime(root)
	if err != nil {
		t.Fatal(err)
	}
	next.Close()
}

func TestOtherProcessAndCrashRelease(t *testing.T) {
	root := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestLeaseHelper$")
	cmd.Env = append(os.Environ(), "OPS_INSTALLGUARD_TEST_ROOT="+root)
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	scanner := bufio.NewScanner(out)
	if !scanner.Scan() || scanner.Text() != "ready" {
		t.Fatal("child not ready")
	}
	own, err := AcquireRuntime(root)
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(own.CheckUpdate(), ErrBusy) {
		t.Fatal("cross-process runtime missed")
	}
	own.Close()
	if release, err := AcquireUpdate(root); err == nil {
		release()
		t.Fatal("cross-process lock missed")
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	release, err := AcquireUpdate(root)
	if err != nil {
		t.Fatal("crash leaked lock", err)
	}
	release()
}
