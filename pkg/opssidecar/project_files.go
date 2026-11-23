package opssidecar

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/pkg/sftp"
)

const projectMaxEditBytes = 512 * 1024

var errProjectConflict = errors.New("远端文件已被外部修改，请重新加载后重试")

type pathLockTable struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func newPathLockTable() *pathLockTable {
	return &pathLockTable{locks: map[string]*sync.Mutex{}}
}

func (t *pathLockTable) lock(connectionID, targetPath string) func() {
	key := connectionID + "\x00" + cleanRemotePath(targetPath)
	t.mu.Lock()
	m := t.locks[key]
	if m == nil {
		m = &sync.Mutex{}
		t.locks[key] = m
	}
	t.mu.Unlock()
	m.Lock()
	return m.Unlock
}

type projectFileText struct {
	Path    string      `json:"path"`
	Content string      `json:"content"`
	Version string      `json:"version"`
	Bytes   int         `json:"bytes"`
	mode    os.FileMode `json:"-"`
}

func resolveProjectRelPath(root, rel string) (string, error) {
	rootClean := cleanRemotePath(root)
	if rootClean == "" {
		return "", errors.New("项目根目录无效")
	}
	rel = strings.TrimSpace(rel)
	if rel == "" || strings.ContainsAny(rel, "\\\x00") || path.IsAbs(rel) {
		return "", errors.New("请使用项目内的相对路径")
	}
	clean := path.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, "../") ||
		clean == ".git" || strings.HasPrefix(clean, ".git/") {
		return "", errors.New("禁止访问项目外或 .git 目录")
	}
	if clean == "." {
		return rootClean, nil
	}
	return path.Join(rootClean, clean), nil
}

func hashTextBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func readProjectTextFile(client *sftp.Client, target string) (projectFileText, error) {
	info, err := client.Lstat(target)
	if err != nil {
		return projectFileText{}, err
	}
	if !info.Mode().IsRegular() {
		return projectFileText{}, errors.New("仅支持普通文本文件")
	}
	if info.Size() > projectMaxEditBytes {
		return projectFileText{}, fmt.Errorf("文件大小超过编辑上限（%d KiB）", projectMaxEditBytes/1024)
	}
	f, err := client.Open(target)
	if err != nil {
		return projectFileText{}, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, projectMaxEditBytes+1))
	if err != nil {
		return projectFileText{}, err
	}
	if len(data) > projectMaxEditBytes || bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		return projectFileText{}, errors.New("文件不是有效的 UTF-8 文本或超出大小上限")
	}
	return projectFileText{
		Path:    target,
		Content: string(data),
		Version: hashTextBytes(data),
		Bytes:   len(data),
		mode:    info.Mode().Perm(),
	}, nil
}

func randomTempSuffix() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "tmp"
	}
	return hex.EncodeToString(b)
}

// writeProjectTextFile 带版本检查与同目录安全替换的文件提交。
//   - force=false && expectedVersion!=nil：提交前两次核对目标文件版本，不一致返回 errProjectConflict。
//   - force=false && expectedVersion==nil：视为新建文件，使用 SFTP Rename 防止覆盖并发创建的文件。
//   - force=true：跳过版本校验直接以 PosixRename 覆盖（用户确认强制保存）。
func writeProjectTextFile(
	client *sftp.Client,
	target string,
	content string,
	expectedVersion *string,
	force bool,
) (projectFileText, error) {
	data := []byte(content)
	if len(data) > projectMaxEditBytes || bytes.IndexByte(data, 0) >= 0 {
		return projectFileText{}, errors.New("内容超出文本写入上限或包含二进制 NUL 字节")
	}
	mode := os.FileMode(0o644)
	overwrite := false

	if expectedVersion != nil && !force {
		current, err := readProjectTextFile(client, target)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return projectFileText{}, errProjectConflict
			}
			return projectFileText{}, err
		}
		if current.Version != *expectedVersion {
			return projectFileText{}, errProjectConflict
		}
		if current.mode != 0 {
			mode = current.mode
		}
		overwrite = true
	} else if force {
		if info, err := client.Lstat(target); err == nil && info.Mode().IsRegular() {
			if info.Mode().Perm() != 0 {
				mode = info.Mode().Perm()
			}
			overwrite = true
		}
	}

	dir := path.Dir(target)
	if err := client.MkdirAll(dir); err != nil {
		return projectFileText{}, fmt.Errorf("无法创建父目录 %s: %w", dir, err)
	}

	tempPath := path.Join(dir, ".aaw-tmp-"+randomTempSuffix())
	f, err := client.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return projectFileText{}, fmt.Errorf("无法创建同目录临时文件: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = client.Remove(tempPath)
		}
	}()

	_, writeErr := f.Write(data)
	chmodErr := f.Chmod(mode)
	closeErr := f.Close()
	if err := errors.Join(writeErr, chmodErr, closeErr); err != nil {
		return projectFileText{}, fmt.Errorf("写入临时文件失败: %w", err)
	}

	if overwrite {
		if expectedVersion != nil && !force {
			// 提交前再次核对版本，尽量收窄并发修改窗口。
			current, err := readProjectTextFile(client, target)
			if err != nil || current.Version != *expectedVersion {
				return projectFileText{}, errProjectConflict
			}
		}
		if err := client.PosixRename(tempPath, target); err != nil {
			return projectFileText{}, fmt.Errorf("远端 SFTP 不支持同目录安全替换（posix-rename）或提交失败: %w", err)
		}
	} else {
		// 新建文件使用标准 Rename：若目标已存在则拒绝，避免静默覆盖。
		if err := client.Rename(tempPath, target); err != nil {
			return projectFileText{}, fmt.Errorf("新建文件失败（可能已存在同名文件）: %w", err)
		}
	}
	committed = true

	return projectFileText{
		Path:    target,
		Version: hashTextBytes(data),
		Bytes:   len(data),
		mode:    mode,
	}, nil
}

func (s *Service) handleProjectFSRead(req Request) (Response, bool) {
	var params struct {
		ConnectionID string `json:"connectionId"`
		Path         string `json:"path"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || strings.TrimSpace(params.ConnectionID) == "" || strings.TrimSpace(params.Path) == "" {
		return invalidParams(req.ID, "connectionId 与 path 必填"), false
	}
	var out projectFileText
	err := s.withProjectSFTP(params.ConnectionID, func(client *sftp.Client) error {
		var readErr error
		out, readErr = readProjectTextFile(client, cleanRemotePath(params.Path))
		return readErr
	})
	if err != nil {
		if strings.Contains(err.Error(), "仅支持") || strings.Contains(err.Error(), "上限") || strings.Contains(err.Error(), "不是有效") {
			return errorResponse(req.ID, CodeUnsupported, err.Error(), ""), false
		}
		return classifyProjectFSError(req.ID, err), false
	}
	return okResponse(req.ID, out), false
}

func (s *Service) handleProjectFSWrite(req Request) (Response, bool) {
	var params struct {
		ConnectionID    string  `json:"connectionId"`
		Path            string  `json:"path"`
		Content         string  `json:"content"`
		ExpectedVersion *string `json:"expectedVersion,omitempty"`
		ExpectedContent *string `json:"expectedContent,omitempty"`
		Force           bool    `json:"force,omitempty"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || strings.TrimSpace(params.ConnectionID) == "" || strings.TrimSpace(params.Path) == "" {
		return invalidParams(req.ID, "connectionId 与 path 必填"), false
	}

	target := cleanRemotePath(params.Path)
	expectedVersion := params.ExpectedVersion
	if expectedVersion == nil && params.ExpectedContent != nil {
		h := hashTextBytes([]byte(*params.ExpectedContent))
		expectedVersion = &h
	}

	unlock := s.projectPathLocks.lock(params.ConnectionID, target)
	defer unlock()

	var out projectFileText
	err := s.withProjectSFTP(params.ConnectionID, func(client *sftp.Client) error {
		var writeErr error
		out, writeErr = writeProjectTextFile(client, target, params.Content, expectedVersion, params.Force)
		return writeErr
	})
	if err != nil {
		if errors.Is(err, errProjectConflict) {
			return errorResponse(req.ID, CodeConflict, err.Error(), "文件在打开后被外部修改，请重新加载对比后再保存"), false
		}
		return classifyProjectFSError(req.ID, err), false
	}
	return okResponse(req.ID, map[string]any{
		"path":    out.Path,
		"version": out.Version,
		"bytes":   out.Bytes,
	}), false
}
