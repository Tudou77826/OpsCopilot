package acp

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// openCodeProjectCommands imports prompt-only project commands without loading
// project agent/permission/plugin configuration. Commands with pre-execution
// In reviewed modes, shell/file interpolation and subagents are excluded:
// OpenCode may perform them outside the ordinary ACP permission callback.
// Full access preserves native command behavior. A bad command never blocks chat.
func openCodeProjectCommands(project string, fullAccess bool) (map[string]any, []AgentCommand, error) {
	commands := map[string]any{}
	var unavailable []AgentCommand
	dir := filepath.Join(project, ".opencode", "commands")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return commands, nil, nil
	}
	if err != nil {
		return nil, nil, errors.New("无法读取项目的 Agent 命令")
	}
	if len(entries) > 256 {
		return nil, nil, errors.New("项目 Agent 命令过多")
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if !validAgentCommandName(name) {
			continue
		}
		file, err := os.Open(filepath.Join(dir, entry.Name()))
		if err != nil {
			unavailable = append(unavailable, AgentCommand{Name: name, Unavailable: "无法读取命令文件"})
			continue
		}
		data, err := io.ReadAll(io.LimitReader(file, 64*1024+1))
		_ = file.Close()
		if err != nil || len(data) > 64*1024 || !utf8.Valid(data) {
			unavailable = append(unavailable, AgentCommand{Name: name, Unavailable: "命令文件无效或过大"})
			continue
		}
		text := strings.ReplaceAll(string(data), "\r\n", "\n")
		var meta struct {
			Description string `yaml:"description"`
			Agent       string `yaml:"agent"`
			Model       string `yaml:"model"`
			Subtask     bool   `yaml:"subtask"`
		}
		if strings.HasPrefix(text, "---\n") {
			end := strings.Index(text[4:], "\n---\n")
			if end < 0 || yaml.Unmarshal([]byte(text[4:4+end]), &meta) != nil {
				unavailable = append(unavailable, AgentCommand{Name: name, Unavailable: "命令格式无效"})
				continue
			}
			text = text[4+end+5:]
		}
		if !fullAccess && (strings.Contains(text, "!`") || strings.Contains(text, "@") || meta.Agent != "" || meta.Model != "" || meta.Subtask) {
			unavailable = append(unavailable, AgentCommand{Name: name, Description: boundedFeatureText(meta.Description, 512), Unavailable: "包含自动执行、文件引用或子 Agent，暂不支持"})
			continue
		}
		if strings.TrimSpace(text) == "" {
			unavailable = append(unavailable, AgentCommand{Name: name, Unavailable: "命令内容为空"})
			continue
		}
		command := map[string]any{"template": text, "description": boundedFeatureText(meta.Description, 512)}
		if fullAccess {
			if meta.Agent != "" {
				command["agent"] = meta.Agent
			}
			if meta.Model != "" {
				command["model"] = meta.Model
			}
			if meta.Subtask {
				command["subtask"] = true
			}
		}
		commands[name] = command
	}
	return commands, unavailable, nil
}
