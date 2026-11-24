package projectspike

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pkg/sftp"
	"opscopilot/pkg/core/ops"
	"opscopilot/pkg/filetransfer"
	"opscopilot/pkg/remote"
)

const maxTextBytes = 64 << 10

var textField = map[string]string{"type": "string"}

var tools = []tool{
	{"read_file", "读取远端项目的文本文件，返回内容和版本。使用项目相对路径。", object(map[string]any{"path": textField}, "path")},
	{"edit_file", "按读取版本局部修改远端文件。old_text 必须唯一匹配。", object(map[string]any{
		"path": textField, "version": textField, "old_text": textField, "new_text": textField,
	}, "path", "version", "old_text", "new_text")},
	{"write_file", "保存远端完整文件内容。新建时省略 version，覆盖已有文件时传入读取版本。", object(map[string]any{
		"path": textField, "version": textField, "content": textField,
	}, "path", "content")},
	{"run_command", "在远端项目内运行 Linux Shell 命令，用于查找文件、搜索内容和执行测试。返回标准输出、标准错误和退出码。", object(map[string]any{
		"command": textField, "cwd": textField,
	}, "command")},
}

type toolInput struct {
	Path    string  `json:"path"`
	Version *string `json:"version"`
	OldText string  `json:"old_text"`
	NewText *string `json:"new_text"`
	Content *string `json:"content"`
	Command string  `json:"command"`
	Cwd     string  `json:"cwd"`
}

type projectFile struct {
	Content string `json:"content"`
	Version string `json:"version"`
	mode    os.FileMode
}

func projectPath(root, name string) (string, error) {
	clean := path.Clean(name)
	if name == "" || path.IsAbs(name) || strings.ContainsAny(name, "\\\x00") ||
		clean == ".." || strings.HasPrefix(clean, "../") ||
		clean == ".git" || strings.HasPrefix(clean, ".git/") {
		return "", errors.New("请使用项目内的相对路径")
	}
	return path.Join(root, clean), nil
}

func canonicalDir(files *sftp.Client, name string) error {
	real, err := files.RealPath(name)
	if err != nil {
		return err
	}
	if real != name {
		return errors.New("路径经过符号链接")
	}
	return nil
}

func readText(files *sftp.Client, name string) (projectFile, error) {
	info, err := files.Lstat(name)
	if err != nil {
		return projectFile{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxTextBytes {
		return projectFile{}, errors.New("仅支持普通文本文件，大小上限 64 KiB")
	}
	f, err := files.Open(name)
	if err != nil {
		return projectFile{}, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxTextBytes+1))
	if err != nil {
		return projectFile{}, err
	}
	if len(b) > maxTextBytes || !utf8.Valid(b) || bytes.Contains(b, []byte{0}) {
		return projectFile{}, errors.New("文件超出文本读取范围")
	}
	hash := sha256.Sum256(b)
	return projectFile{Content: string(b), Version: hex.EncodeToString(hash[:]), mode: info.Mode().Perm()}, nil
}

func replaceText(text, old, replacement string) (string, error) {
	if old == "" || strings.Count(text, old) != 1 {
		return "", errors.New("old_text 必须在当前文件中恰好匹配一次")
	}
	return strings.Replace(text, old, replacement, 1), nil
}

func saveText(files *sftp.Client, name, content string, expected *string) (projectFile, error) {
	if len(content) > maxTextBytes || strings.ContainsRune(content, 0) {
		return projectFile{}, errors.New("内容超出文本写入范围")
	}
	mode := os.FileMode(0644)
	if expected != nil {
		current, err := readText(files, name)
		if err != nil || current.Version != *expected {
			return projectFile{}, errors.New("文件已变化，请重新读取")
		}
		mode = current.mode
	}
	temp := path.Join(path.Dir(name), ".aaw-"+rand.Text())
	f, err := files.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return projectFile{}, err
	}
	defer files.Remove(temp)
	_, writeErr := f.Write([]byte(content))
	chmodErr := f.Chmod(mode)
	if err := errors.Join(writeErr, chmodErr, f.Close()); err != nil {
		return projectFile{}, err
	}
	if expected == nil {
		// 标准 SFTP rename 要求目标不存在，防止新建覆盖并发创建的文件。
		if err := files.Rename(temp, name); err != nil {
			return projectFile{}, err
		}
	} else {
		current, err := readText(files, name)
		if err != nil || current.Version != *expected {
			return projectFile{}, errors.New("文件已变化，请重新读取")
		}
		if err := files.PosixRename(temp, name); err != nil {
			return projectFile{}, err
		}
	}
	hash := sha256.Sum256([]byte(content))
	return projectFile{Version: hex.EncodeToString(hash[:])}, nil
}

func (s *service) call(ctx context.Context, toolName string, args json.RawMessage) (any, error) {
	var input toolInput
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, errors.New("工具参数不是有效的 JSON 对象")
	}
	var target string
	var err error
	if toolName == "run_command" {
		if strings.TrimSpace(input.Command) == "" || len(input.Command) > 4096 || strings.ContainsRune(input.Command, 0) {
			return nil, errors.New("命令为空或超出长度限制")
		}
		cwd := input.Cwd
		if cwd == "" {
			cwd = "."
		}
		target, err = projectPath(s.root, cwd)
	} else {
		target, err = projectPath(s.root, input.Path)
		if target == s.root {
			return nil, errors.New("文件路径不能是项目根目录")
		}
	}
	if err != nil {
		return nil, err
	}
	switch toolName {
	case "read_file", "run_command":
	case "edit_file":
		if input.Version == nil || input.NewText == nil {
			return nil, errors.New("编辑需要版本和替换文本")
		}
	case "write_file":
		if input.Content == nil {
			return nil, errors.New("保存需要完整内容")
		}
	default:
		return nil, errors.New("未知项目工具")
	}
	if err := s.approve(toolName, input); err != nil {
		return nil, err
	}
	files, err := filetransfer.DialSFTP(ctx, s.conn.SSHClient(), 8*time.Second)
	if err != nil {
		return nil, err
	}
	defer files.Close()
	if toolName == "run_command" {
		if err := canonicalDir(files, target); err != nil {
			return nil, err
		}
		command := "cd " + ops.QuoteShell(target) + " && (\n" + input.Command + "\n)"
		if s.usesRootLogin {
			// 调试项目使用现有低权限账号运行命令。
			command = "runuser -u nobody -- env -i PATH=/usr/bin:/bin HOME=/tmp sh -c " + ops.QuoteShell(command)
		}
		return s.conn.RunCommand(ctx, remote.CommandRequest{Command: command, CaptureBytes: 8192})
	}
	if err := canonicalDir(files, path.Dir(target)); err != nil {
		return nil, err
	}
	switch toolName {
	case "read_file":
		return readText(files, target)
	case "edit_file":
		current, err := readText(files, target)
		if err != nil || current.Version != *input.Version {
			return nil, errors.New("文件已变化，请重新读取")
		}
		content, err := replaceText(current.Content, input.OldText, *input.NewText)
		if err != nil {
			return nil, err
		}
		return saveText(files, target, content, input.Version)
	default:
		return saveText(files, target, *input.Content, input.Version)
	}
}
