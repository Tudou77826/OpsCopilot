//go:build !windows

package installguard

import (
	"golang.org/x/sys/unix"
	"os"
)

func lock(f *os.File, exclusive bool) error {
	flags := unix.LOCK_SH | unix.LOCK_NB
	if exclusive {
		flags = unix.LOCK_EX | unix.LOCK_NB
	}
	return unix.Flock(int(f.Fd()), flags)
}
func unlock(f *os.File) { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN) }
