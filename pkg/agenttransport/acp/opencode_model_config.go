package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// openCodeUserModelConfig reads only model/provider settings from OpenCode's
// resolved user configuration. Agent definitions, tools, MCP servers, hooks,
// plugins and permissions are never copied into the managed session.
func openCodeUserModelConfig(ctx context.Context, executable, workdir string) (map[string]any, error) {
	command := exec.CommandContext(ctx, executable, "debug", "config", "--pure")
	command.Dir = workdir
	command.Env = replaceEnvironment(os.Environ(), map[string]string{"OPENCODE_DISABLE_PROJECT_CONFIG": "1", "OPENCODE_PURE": "1", "OPENCODE_DISABLE_AUTOUPDATE": "true"})
	hideWindow(command)
	var out boundedBuffer
	command.Stdout = &out
	command.Stderr = io.Discard
	if err := command.Run(); err != nil || out.overflow {
		return nil, errors.New("无法读取 OpenCode 模型配置，请检查运行环境")
	}
	return selectOpenCodeModelConfig(out.Bytes())
}

type boundedBuffer struct {
	bytes.Buffer
	overflow bool
}

func (b *boundedBuffer) Write(data []byte) (int, error) {
	if b.Len()+len(data) > 2<<20 {
		b.overflow = true
		return len(data), nil
	}
	return b.Buffer.Write(data)
}

func selectOpenCodeModelConfig(raw []byte) (map[string]any, error) {
	var config map[string]json.RawMessage
	if len(raw) > 2<<20 || json.Unmarshal(raw, &config) != nil {
		return nil, errors.New("OpenCode 模型配置无法解析")
	}
	result := map[string]any{}
	for _, key := range []string{"model", "small_model", "provider", "enabled_providers", "disabled_providers"} {
		if value := config[key]; len(value) > 0 && string(value) != "null" {
			var parsed any
			if json.Unmarshal(value, &parsed) != nil {
				return nil, errors.New("OpenCode 模型配置无法解析")
			}
			result[key] = parsed
		}
	}
	return result, nil
}

// copyOpenCodeAuth copies the user's provider authentication into the private
// process profile. It returns no authentication data to the UI or logs.
func copyOpenCodeAuth(target string) error {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return errors.New("无法定位 OpenCode 认证目录")
		}
		base = filepath.Join(home, ".local", "share")
	}
	file, err := os.Open(filepath.Join(base, "opencode", "auth.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errors.New("无法读取 OpenCode 模型认证配置")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (2<<20)+1))
	if err != nil || len(data) > 2<<20 || !json.Valid(data) {
		return errors.New("无法读取 OpenCode 模型认证配置")
	}
	destination := filepath.Join(target, "xdg_data_home", "opencode")
	if err = os.MkdirAll(destination, 0700); err != nil {
		return errors.New("无法准备模型认证目录")
	}
	if err = os.WriteFile(filepath.Join(destination, "auth.json"), data, 0600); err != nil {
		return errors.New("无法准备模型认证信息")
	}
	return nil
}

func replaceEnvironment(env []string, replacements map[string]string) []string {
	result := make([]string, 0, len(env)+len(replacements))
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if _, exists := replacements[strings.ToUpper(key)]; !exists {
			result = append(result, entry)
		}
	}
	for key, value := range replacements {
		result = append(result, key+"="+value)
	}
	return result
}
