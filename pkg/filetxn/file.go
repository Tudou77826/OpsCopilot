// Package filetxn serializes cooperating local processes and atomically replaces
// JSON files. The lock is OS-owned: a crashed process cannot leave a stale lock.
package filetxn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"time"
)

var ErrConflict = errors.New("配置已被其他窗口修改，请重新加载后再保存")

// ReadBounded rejects devices and oversized/corrupt files before allocating the
// contents. LimitReader also bounds files that grow after the size check.
func ReadBounded(path string, limit int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if limit < 1 || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("文件类型无效或超过读取上限")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("文件超过读取上限")
	}
	return b, nil
}

// TryLock obtains an OS-owned lock without waiting. It also reports filesystem
// errors; callers must not interpret a failed acquisition as permission to act.
func TryLock(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = tryLock(f); err != nil {
		f.Close()
		return nil, err
	}
	return func() { unlock(f); f.Close() }, nil
}

func Lock(path string) (func(), error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return LockContext(ctx, path)
}

// LockContext waits only for this caller's deadline. The OS owns the lock,
// so cancellation closes the waiting handle without changing the lock file.
func LockContext(ctx context.Context, path string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	// Windows can spend several seconds syncing and replacing the file under
	// concurrent processes. Wait for the OS-owned lock instead of dropping edits.
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err = ctx.Err(); err != nil {
			f.Close()
			return nil, err
		}
		if err = tryLock(f); err == nil {
			return func() { unlock(f); f.Close() }, nil
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, fmt.Errorf("配置文件正在使用: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

// Write must be called with the corresponding lock held.
func Write(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".ops-write-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if info, e := os.Stat(path); e == nil {
		if e = f.Chmod(info.Mode().Perm()); e != nil {
			f.Close()
			return e
		}
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return replace(name, path)
}

func Read(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return b, err
}

// Merge applies only edits relative to base. Unrelated external edits and
// unknown keys survive; competing edits of the same leaf fail, never win silently.
func Merge(path string, base, next []byte) ([]byte, error) {
	release, err := Lock(path)
	if err != nil {
		return nil, err
	}
	defer release()
	current, err := Read(path)
	if err != nil {
		return nil, err
	}
	decode := func(b []byte) (any, error) {
		if len(b) == 0 {
			return nil, nil
		}
		var v any
		e := json.Unmarshal(b, &v)
		return v, e
	}
	b, err := decode(base)
	if err != nil {
		return nil, err
	}
	n, err := decode(next)
	if err != nil {
		return nil, err
	}
	c, err := decode(current)
	if err != nil {
		return nil, err
	}
	merged, err := merge(b, n, c)
	if err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return nil, err
	}
	if reflect.DeepEqual(merged, c) {
		return current, nil
	}
	if err = Write(path, data); err != nil {
		return nil, err
	}
	return data, nil
}

func merge(base, next, current any) (any, error) {
	if reflect.DeepEqual(base, next) {
		return current, nil
	}
	n, nok := next.(map[string]any)
	c, cok := current.(map[string]any)
	b, bok := base.(map[string]any)
	if nok && cok && (bok || base == nil) {
		out := map[string]any{}
		for k, v := range c {
			out[k] = v
		}
		for k, v := range n {
			m, e := merge(b[k], v, c[k])
			if e != nil {
				return nil, ErrConflict
			}
			out[k] = m
		}
		return out, nil
	}
	if reflect.DeepEqual(base, current) || reflect.DeepEqual(next, current) {
		return next, nil
	}
	return nil, ErrConflict
}
