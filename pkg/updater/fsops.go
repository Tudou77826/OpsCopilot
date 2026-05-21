package updater

import (
	"io"
	"os"
	"time"
)

// Filesystem abstracts file operations for testability.
type Filesystem interface {
	ReadDir(dirname string) ([]os.DirEntry, error)
	Rename(oldpath, newpath string) error
	CopyFile(src, dst string) error
	Stat(name string) (os.FileInfo, error)
	Remove(name string) error
	MkdirAll(path string, perm os.FileMode) error
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm os.FileMode) error
}

// OSFS is the production filesystem implementation.
type OSFS struct{}

func (OSFS) ReadDir(dirname string) ([]os.DirEntry, error) { return os.ReadDir(dirname) }
func (OSFS) Rename(oldpath, newpath string) error           { return os.Rename(oldpath, newpath) }
func (OSFS) Stat(name string) (os.FileInfo, error)          { return os.Stat(name) }
func (OSFS) Remove(name string) error                        { return os.Remove(name) }
func (OSFS) MkdirAll(path string, perm os.FileMode) error    { return os.MkdirAll(path, perm) }
func (OSFS) ReadFile(name string) ([]byte, error)             { return os.ReadFile(name) }
func (OSFS) WriteFile(name string, data []byte, perm os.FileMode) error { return os.WriteFile(name, data, perm) }

func (OSFS) CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

// ProcessWaiter abstracts waiting for a process to exit.
type ProcessWaiter interface {
	Wait(pid int, timeout time.Duration) error
}
