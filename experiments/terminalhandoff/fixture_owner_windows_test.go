//go:build windows

package terminalhandoff

import (
	"os"
	"strconv"
	"testing"

	"golang.org/x/sys/windows"
)

// A process handle follows the original owner even if Windows later reuses its PID.
func fixtureOwner(raw string) (func() bool, func(), error) {
	if raw == "" {
		return func() bool { return true }, func() {}, nil
	}
	pid, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return nil, nil, err
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return nil, nil, err
	}
	return func() bool {
		status, err := windows.WaitForSingleObject(handle, 0)
		return err == nil && status == uint32(windows.WAIT_TIMEOUT)
	}, func() { _ = windows.CloseHandle(handle) }, nil
}

func TestFixtureOwnerCurrentProcess(t *testing.T) {
	alive, closeOwner, err := fixtureOwner(strconv.Itoa(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	defer closeOwner()
	if !alive() {
		t.Fatal("current fixture owner reported as exited")
	}
	if _, _, err := fixtureOwner("invalid"); err == nil {
		t.Fatal("invalid owner PID accepted")
	}
}
