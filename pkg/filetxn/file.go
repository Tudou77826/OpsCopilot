// Package filetxn serializes cooperating local processes and atomically replaces
// JSON files. The lock is OS-owned: a crashed process cannot leave a stale lock.
package filetxn

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"
)

var ErrConflict = errors.New("配置已被其他窗口修改，请重新加载后再保存")

func Lock(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		if err = tryLock(f); err == nil {
			return func() { unlock(f); f.Close() }, nil
		}
		if time.Now().After(deadline) {
			f.Close()
			return nil, fmt.Errorf("配置文件正在使用: %w", err)
		}
		time.Sleep(10 * time.Millisecond)
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
