package patchstore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PendingStore 保存尚未成功上传到远端的补丁。
// 它与 Git 工作目录分离，避免将本地 outbox 混入远端仓库。
type PendingStore struct {
	baseDir string
}

func NewPendingStore(baseDir string) *PendingStore {
	return &PendingStore{baseDir: baseDir}
}

func (s *PendingStore) Save(patch Patch) error {
	filePath := s.patchFilePath(patch)
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("create pending dir: %w", err)
	}
	if err := os.WriteFile(filePath, []byte(formatPatchFile(patch)), 0644); err != nil {
		return fmt.Errorf("write pending patch: %w", err)
	}
	return nil
}

func (s *PendingStore) Delete(patch Patch) error {
	filePath := s.patchFilePath(patch)
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove pending patch: %w", err)
	}
	_ = removeEmptyParents(filepath.Dir(filePath), s.baseDir)
	return nil
}

func (s *PendingStore) List() ([]Patch, error) {
	if s == nil {
		return nil, nil
	}
	if _, err := os.Stat(s.baseDir); os.IsNotExist(err) {
		return nil, nil
	}

	var patches []Patch
	err := filepath.Walk(s.baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		patch, err := parsePatchFile(data)
		if err != nil {
			return nil
		}
		patches = append(patches, patch)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return patches, nil
}

func (s *PendingStore) patchFilePath(patch Patch) string {
	serviceDir := sanitizeDirName(patch.Service)
	moduleDir := sanitizeDirName(patch.Module)
	dateStr := patch.Timestamp.Format("2006-01-02")
	fileName := fmt.Sprintf("%s_%s.md", dateStr, patch.ID)
	return filepath.Join(s.baseDir, "patches", serviceDir, moduleDir, fileName)
}

func removeEmptyParents(dir, stopDir string) error {
	stopDir = filepath.Clean(stopDir)
	dir = filepath.Clean(dir)

	for len(dir) >= len(stopDir) {
		if dir == stopDir {
			return nil
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			return nil
		}
		if err := os.Remove(dir); err != nil {
			return err
		}
		dir = filepath.Dir(dir)
	}

	return nil
}
