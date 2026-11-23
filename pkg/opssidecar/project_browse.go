package opssidecar

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/pkg/sftp"
)

const (
	defaultProjectListLimit    = 2000
	defaultProjectPreviewBytes = 512 * 1024
	defaultProjectImageBytes   = 4 * 1024 * 1024
	defaultProjectSearchFiles  = 4000
)

type projectFSEntry struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Directory bool   `json:"directory"`
	Symlink   bool   `json:"symlink"`
	Size      int64  `json:"size"`
	Mtime     int64  `json:"mtime"`
}

var projectSearchIgnoredDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"target":       true,
	"dist":         true,
	"build":        true,
	".next":        true,
	".venv":        true,
	"__pycache__":  true,
}

func cleanRemotePath(raw string) string {
	trimmed := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if trimmed == "" || trimmed == "~" {
		return ""
	}
	cleaned := path.Clean(trimmed)
	if !strings.HasPrefix(cleaned, "/") {
		cleaned = "/" + cleaned
	}
	return cleaned
}

func remoteParentPath(p string) string {
	cleaned := cleanRemotePath(p)
	if cleaned == "" || cleaned == "/" {
		return "/"
	}
	parent := path.Dir(cleaned)
	if parent == "." || parent == "" {
		return "/"
	}
	return parent
}

func classifyProjectFSError(id string, err error) Response {
	var hkErr *hostKeyCheckError
	if errors.As(err, &hkErr) {
		code := CodeHostKeyUntrusted
		if hkErr.challenge.Status == "host_key_mismatch" {
			code = CodeHostKeyMismatch
		}
		return errorResponse(id, code, hkErr.Error(), "")
	}
	if errors.Is(err, errServerNotFound) {
		return errorResponse(id, CodeNotFound, err.Error(), "")
	}
	if errors.Is(err, os.ErrNotExist) {
		return errorResponse(id, CodeNotFound, "远端路径不存在", "")
	}
	if errors.Is(err, os.ErrPermission) {
		return errorResponse(id, CodePermissionDenied, "无权访问该远端路径", "")
	}
	var statusErr *sftp.StatusError
	if errors.As(err, &statusErr) {
		switch statusErr.Code {
		case 2: // SSH_FX_NO_SUCH_FILE
			return errorResponse(id, CodeNotFound, "远端路径不存在", "")
		case 3: // SSH_FX_PERMISSION_DENIED
			return errorResponse(id, CodePermissionDenied, "无权访问该远端路径", "")
		}
	}
	return errorResponse(id, CodeConnectFailed, err.Error(), "")
}

func (s *Service) handleProjectFSList(req Request) (Response, bool) {
	var params struct {
		ConnectionID string `json:"connectionId"`
		Path         string `json:"path"`
		MaxEntries   int    `json:"maxEntries,omitempty"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || strings.TrimSpace(params.ConnectionID) == "" {
		return invalidParams(req.ID, "connectionId 必填"), false
	}
	limit := params.MaxEntries
	if limit <= 0 || limit > 5000 {
		limit = defaultProjectListLimit
	}

	var resolvedPath string
	var entries []projectFSEntry
	var truncated bool

	err := s.withProjectSFTP(params.ConnectionID, func(client *sftp.Client) error {
		target := cleanRemotePath(params.Path)
		if target == "" {
			wd, wdErr := client.Getwd()
			if wdErr != nil || strings.TrimSpace(wd) == "" {
				target = "/"
			} else {
				target = cleanRemotePath(wd)
			}
		} else if strings.HasPrefix(strings.TrimSpace(params.Path), "~/") {
			wd, wdErr := client.Getwd()
			if wdErr == nil && strings.TrimSpace(wd) != "" {
				target = path.Clean(path.Join(wd, strings.TrimPrefix(strings.TrimSpace(params.Path), "~/")))
			}
		}

		info, statErr := client.Stat(target)
		if statErr != nil {
			return statErr
		}
		if !info.IsDir() {
			return fmt.Errorf("远端路径不是目录: %s", target)
		}
		if real, realErr := client.RealPath(target); realErr == nil && strings.HasPrefix(real, "/") {
			target = cleanRemotePath(real)
		}
		resolvedPath = target

		rawEntries, readErr := client.ReadDir(target)
		if readErr != nil {
			return readErr
		}

		items := make([]projectFSEntry, 0, len(rawEntries))
		for _, item := range rawEntries {
			name := item.Name()
			if name == "." || name == ".." {
				continue
			}
			fullPath := path.Join(target, name)
			isSymlink := item.Mode()&os.ModeSymlink != 0
			isDir := item.IsDir()
			if isSymlink {
				if resolved, err := client.Stat(fullPath); err == nil {
					isDir = resolved.IsDir()
				}
			}
			items = append(items, projectFSEntry{
				Path:      fullPath,
				Name:      name,
				Directory: isDir,
				Symlink:   isSymlink,
				Size:      item.Size(),
				Mtime:     item.ModTime().Unix(),
			})
		}

		sort.Slice(items, func(i, j int) bool {
			if items[i].Directory != items[j].Directory {
				return items[i].Directory
			}
			return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
		})
		if len(items) > limit {
			truncated = true
			items = items[:limit]
		}
		entries = items
		return nil
	})
	if err != nil {
		return classifyProjectFSError(req.ID, err), false
	}

	return okResponse(req.ID, map[string]any{
		"path":       resolvedPath,
		"parentPath": remoteParentPath(resolvedPath),
		"entries":    entries,
		"truncated":  truncated,
	}), false
}

func (s *Service) handleProjectFSStat(req Request) (Response, bool) {
	var params struct {
		ConnectionID string `json:"connectionId"`
		Path         string `json:"path"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || strings.TrimSpace(params.ConnectionID) == "" || strings.TrimSpace(params.Path) == "" {
		return invalidParams(req.ID, "connectionId 与 path 必填"), false
	}

	var entry projectFSEntry
	err := s.withProjectSFTP(params.ConnectionID, func(client *sftp.Client) error {
		target := cleanRemotePath(params.Path)
		linfo, lerr := client.Lstat(target)
		if lerr != nil {
			return lerr
		}
		isSymlink := linfo.Mode()&os.ModeSymlink != 0
		info := linfo
		if isSymlink {
			if resolved, rerr := client.Stat(target); rerr == nil {
				info = resolved
			}
		}
		name := path.Base(target)
		if target == "/" {
			name = "/"
		}
		entry = projectFSEntry{
			Path:      target,
			Name:      name,
			Directory: info.IsDir(),
			Symlink:   isSymlink,
			Size:      info.Size(),
			Mtime:     info.ModTime().Unix(),
		}
		return nil
	})
	if err != nil {
		return classifyProjectFSError(req.ID, err), false
	}
	return okResponse(req.ID, entry), false
}

func imageMimeForPath(p string) string {
	switch strings.ToLower(path.Ext(p)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	case ".ico":
		return "image/x-icon"
	default:
		return ""
	}
}

func (s *Service) handleProjectFSPreview(req Request) (Response, bool) {
	var params struct {
		ConnectionID string `json:"connectionId"`
		Path         string `json:"path"`
		MaxBytes     int64  `json:"maxBytes,omitempty"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || strings.TrimSpace(params.ConnectionID) == "" || strings.TrimSpace(params.Path) == "" {
		return invalidParams(req.ID, "connectionId 与 path 必填"), false
	}
	maxTextBytes := params.MaxBytes
	if maxTextBytes <= 0 || maxTextBytes > 2*1024*1024 {
		maxTextBytes = defaultProjectPreviewBytes
	}

	var result map[string]any
	err := s.withProjectSFTP(params.ConnectionID, func(client *sftp.Client) error {
		target := cleanRemotePath(params.Path)
		info, statErr := client.Stat(target)
		if statErr != nil {
			return statErr
		}
		if info.IsDir() {
			return errors.New("目标是目录，无法作为文件预览")
		}

		imgMime := imageMimeForPath(target)
		if imgMime != "" {
			if info.Size() > defaultProjectImageBytes {
				return fmt.Errorf("图片体积过大（%d 字节），超过预览上限", info.Size())
			}
			f, openErr := client.Open(target)
			if openErr != nil {
				return openErr
			}
			defer f.Close()
			data, readErr := io.ReadAll(io.LimitReader(f, defaultProjectImageBytes+1))
			if readErr != nil {
				return readErr
			}
			if int64(len(data)) > defaultProjectImageBytes {
				return fmt.Errorf("图片体积过大，超过预览上限")
			}
			result = map[string]any{
				"kind":      "image",
				"content":   fmt.Sprintf("data:%s;base64,%s", imgMime, base64.StdEncoding.EncodeToString(data)),
				"mime":      imgMime,
				"bytes":     len(data),
				"truncated": false,
			}
			return nil
		}

		f, openErr := client.Open(target)
		if openErr != nil {
			return openErr
		}
		defer f.Close()
		data, readErr := io.ReadAll(io.LimitReader(f, maxTextBytes+1))
		if readErr != nil {
			return readErr
		}
		truncated := int64(len(data)) > maxTextBytes
		if truncated {
			data = data[:maxTextBytes]
		}
		// 检测 NUL 字节或无效 UTF-8 二进制文件。
		if bytes.IndexByte(data, 0) >= 0 || (!utf8.Valid(data) && !truncated) {
			return errors.New("二进制文件不支持文本预览")
		}
		if truncated {
			// 截断时修剪末尾可能被切半的 UTF-8 字符。
			for len(data) > 0 && !utf8.Valid(data) {
				data = data[:len(data)-1]
			}
		}
		detectedMime := mime.TypeByExtension(path.Ext(target))
		if detectedMime == "" {
			detectedMime = "text/plain; charset=utf-8"
		}
		result = map[string]any{
			"kind":      "text",
			"content":   string(data),
			"mime":      detectedMime,
			"bytes":     info.Size(),
			"truncated": truncated,
		}
		return nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "不支持") || strings.Contains(err.Error(), "过大") || strings.Contains(err.Error(), "目录") {
			return errorResponse(req.ID, CodeUnsupported, err.Error(), ""), false
		}
		return classifyProjectFSError(req.ID, err), false
	}
	return okResponse(req.ID, result), false
}

func (s *Service) handleProjectFSListFiles(req Request) (Response, bool) {
	var params struct {
		ConnectionID string `json:"connectionId"`
		Root         string `json:"root"`
		MaxFiles     int    `json:"maxFiles,omitempty"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || strings.TrimSpace(params.ConnectionID) == "" || strings.TrimSpace(params.Root) == "" {
		return invalidParams(req.ID, "connectionId 与 root 必填"), false
	}
	limit := params.MaxFiles
	if limit <= 0 || limit > 10000 {
		limit = defaultProjectSearchFiles
	}

	files := make([]string, 0, 128)
	truncated := false
	err := s.withProjectSFTP(params.ConnectionID, func(client *sftp.Client) error {
		root := cleanRemotePath(params.Root)
		queue := []string{root}
		for len(queue) > 0 && len(files) < limit {
			dir := queue[0]
			queue = queue[1:]
			entries, readErr := client.ReadDir(dir)
			if readErr != nil {
				if dir == root {
					return readErr
				}
				continue
			}
			for _, item := range entries {
				name := item.Name()
				if name == "." || name == ".." {
					continue
				}
				fullPath := path.Join(dir, name)
				if item.IsDir() {
					if !projectSearchIgnoredDirs[name] {
						queue = append(queue, fullPath)
					}
					continue
				}
				rel := strings.TrimPrefix(fullPath, root)
				rel = strings.TrimPrefix(rel, "/")
				if rel != "" {
					files = append(files, rel)
					if len(files) >= limit {
						truncated = true
						break
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		return classifyProjectFSError(req.ID, err), false
	}
	sort.Strings(files)
	return okResponse(req.ID, map[string]any{
		"paths":     files,
		"truncated": truncated,
	}), false
}
