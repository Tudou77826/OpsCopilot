// Package assistantlocal manages saved local projects and their directory groups.
// These are resource hints, not an OS sandbox or blanket consent.
package assistantlocal

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const MaxDirectories = 16

// Normalize accepts only existing absolute directories, resolves symlinks and
// deduplicates by the host's path rules. It never scans directory contents.
func Normalize(paths []string) ([]string, error) {
	if len(paths) == 0 || len(paths) > MaxDirectories {
		return nil, errors.New("每次请添加 1 至 16 个本地目录")
	}
	out := []string{}
	for _, raw := range paths {
		if raw == "" || len(raw) > 2048 || strings.ContainsAny(raw, "\x00\r\n") || !filepath.IsAbs(raw) {
			return nil, errors.New("请使用现有本地目录的完整绝对路径")
		}
		resolved, err := filepath.EvalSymlinks(filepath.Clean(raw))
		if err != nil {
			return nil, errors.New("本地目录不存在或无法访问，请检查完整路径")
		}
		if len(resolved) > 2048 {
			return nil, errors.New("目录完整路径过长")
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.IsDir() {
			return nil, errors.New("请选择目录，不能添加普通文件")
		}
		if !contains(out, resolved) {
			out = append(out, resolved)
		}
	}
	return out, nil
}

func same(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
func contains(paths []string, path string) bool {
	for _, p := range paths {
		if same(p, path) {
			return true
		}
	}
	return false
}
