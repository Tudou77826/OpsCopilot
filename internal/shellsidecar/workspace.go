package shellsidecar

import (
	"crypto/subtle"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// NewWorkspaceFT exposes only an OS-enforced directory root. Configs are siblings,
// never children. os.Root enforces link/junction boundaries at open time too.
func NewWorkspaceFT(svc *TerminalService, directory string) (*FTService, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("文件区根目录不能是链接")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	service := NewFTService(svc, ".")
	service.root = root
	return service, nil
}
func (s *FTService) localOpen(path string, flags int) (*os.File, error) {
	if s.root != nil {
		return s.root.OpenFile(path, flags, 0600)
	}
	return os.OpenFile(path, flags, 0600)
}
func (s *FTService) localStat(path string) (os.FileInfo, error) {
	if s.root != nil {
		return s.root.Stat(path)
	}
	return os.Stat(path)
}
func (s *FTService) localMkdirAll(path string) error {
	if s.root != nil {
		return s.root.MkdirAll(path, 0700)
	}
	return os.MkdirAll(path, 0755)
}
func (s *FTService) localRemoveAll(path string) error {
	if s.root != nil {
		return s.root.RemoveAll(path)
	}
	return os.RemoveAll(path)
}
func (s *FTService) localRename(oldPath, newPath string) error {
	if oldPath == "." || newPath == "." || oldPath == filepath.Clean(s.dataDir) || newPath == filepath.Clean(s.dataDir) {
		return fmt.Errorf("不能移动文件区根目录")
	}
	if s.root != nil {
		return s.root.Rename(oldPath, newPath)
	}
	return os.Rename(oldPath, newPath)
}
func (s *FTService) Close() {
	s.mu.Lock()
	for _, task := range s.tasks {
		task.cancel()
	}
	s.mu.Unlock()
	if s.root != nil {
		_ = s.root.Close()
	}
}

// WorkspaceHTTP is host-only, token protected, streamed and bounded. Uploads are
// exclusively created so a browser import cannot silently overwrite a file.
func (s *FTService) WorkspaceHTTP(token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.root == nil || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "forbidden", 403)
			return
		}
		path, err := s.resolveLocal(r.URL.Query().Get("path"))
		if err != nil || path == "." {
			http.Error(w, "invalid path", 400)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		switch r.Method {
		case http.MethodPut:
			const maxBytes = 2 << 30
			if r.ContentLength < 0 || r.ContentLength > maxBytes {
				http.Error(w, "invalid file length", 413)
				return
			}
			f, err := s.localOpen(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
			if err != nil {
				http.Error(w, "cannot create file", 409)
				return
			}
			n, copyErr := io.Copy(f, http.MaxBytesReader(w, r.Body, maxBytes))
			closeErr := f.Close()
			if copyErr != nil || closeErr != nil || n != r.ContentLength {
				_ = s.root.Remove(path)
				http.Error(w, "import incomplete", 400)
				return
			}
			w.WriteHeader(http.StatusCreated)
		case http.MethodGet:
			f, err := s.localOpen(path, os.O_RDONLY)
			if err != nil {
				http.Error(w, "file unavailable", 404)
				return
			}
			defer f.Close()
			info, err := f.Stat()
			if err != nil || !info.Mode().IsRegular() {
				http.Error(w, "not a regular file", 400)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			http.ServeContent(w, r, info.Name(), info.ModTime(), f)
		default:
			http.Error(w, "method not allowed", 405)
		}
	})
}

func workspacePath(p string) (string, error) {
	if strings.ContainsAny(p, "\\:\x00") {
		return "", fmt.Errorf("文件区路径无效")
	}
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		p = "."
	}
	if !filepath.IsLocal(p) {
		return "", fmt.Errorf("路径超出文件区")
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return "", fmt.Errorf("路径超出文件区")
		}
	}
	return filepath.Clean(p), nil
}
