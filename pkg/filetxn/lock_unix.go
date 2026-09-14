//go:build !windows

package filetxn

import (
	"golang.org/x/sys/unix"
	"os"
)

func tryLock(f *os.File) error      { return unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB) }
func unlock(f *os.File)             { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN) }
func replace(from, to string) error { return os.Rename(from, to) }
