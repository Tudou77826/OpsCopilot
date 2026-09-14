// Package installguard coordinates cooperating runtimes and the updater for one
// installation. It never discovers, reuses or terminates another Ops process.
package installguard

import (
	"errors"
	"opscopilot/pkg/filetxn"
	"os"
	"path/filepath"
	"sync"
)

var ErrBusy = errors.New("其他 Ops 实例仍在运行或正在升级，请先结束活动并退出桌面 Ops、停用 Teams 中的 Ops 插件后重试")

const RuntimeFile = ".ops-install-runtime.lock"
const AdmissionFile = ".ops-install-admission"

type Lease struct {
	mu   sync.Mutex
	file *os.File
	root string
}

func open(root string) (*os.File, string, error) {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, "", err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, "", err
	}
	f, err := os.OpenFile(filepath.Join(root, RuntimeFile), os.O_CREATE|os.O_RDWR, 0600)
	return f, root, err
}

func AcquireRuntime(root string) (*Lease, error) {
	f, root, err := open(root)
	if err != nil {
		return nil, err
	}
	gate, err := filetxn.Lock(filepath.Join(root, AdmissionFile))
	if err != nil {
		f.Close()
		return nil, ErrBusy
	}
	defer gate()
	if err = lock(f, false); err != nil {
		f.Close()
		return nil, ErrBusy
	}
	return &Lease{file: f, root: root}, nil
}

func (l *Lease) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		unlock(l.file)
		l.file.Close()
		l.file = nil
	}
}

// CheckUpdate excludes the caller's lease, without admitting a new runtime
// during the check. Phase two must still acquire its own exclusive lease.
func (l *Lease) CheckUpdate() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return ErrBusy
	}
	gate, err := filetxn.Lock(filepath.Join(l.root, AdmissionFile))
	if err != nil {
		return ErrBusy
	}
	defer gate()
	unlock(l.file)
	exclusive := lock(l.file, true)
	if exclusive == nil {
		unlock(l.file)
	}
	if err = lock(l.file, false); err != nil {
		return err
	}
	if exclusive != nil {
		return ErrBusy
	}
	return nil
}

// AcquireUpdate holds both admission and exclusive runtime locks across all
// file replacement and rollback, so a new instance cannot enter mid-update.
func AcquireUpdate(root string) (func(), error) {
	f, root, err := open(root)
	if err != nil {
		return nil, err
	}
	gate, err := filetxn.Lock(filepath.Join(root, AdmissionFile))
	if err != nil {
		f.Close()
		return nil, ErrBusy
	}
	if err = lock(f, true); err != nil {
		gate()
		f.Close()
		return nil, ErrBusy
	}
	var once sync.Once
	return func() { once.Do(func() { unlock(f); f.Close(); gate() }) }, nil
}
