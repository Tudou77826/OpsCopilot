package acp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// startCustomAgent uses only ACP v1 and the standard MCP stdio transport.
// The agent's own model and authentication remain under its control.
func (c *Conversation) startCustomAgent(ctx, turnContext context.Context, config ManagedConfig) error {
	if !filepath.IsAbs(config.Executable) {
		return errors.New("请填写 Agent 程序的绝对路径")
	}
	var args []string
	if config.ArgumentsJSON != "" && json.Unmarshal([]byte(config.ArgumentsJSON), &args) != nil {
		return errors.New("Agent 启动参数无效")
	}
	args = normalizeCustomArguments(args)
	workspace, primary, dir, err := prepareStandardWorkspace(config, "assistant-acp-")
	if err != nil {
		return err
	}
	c.directory, c.config = dir, config
	process := ProcessConfig{Executable: config.Executable, Args: args, Directory: primary, Environment: os.Environ()}
	return c.startStandardSession(ctx, turnContext, "Agent", process, primary, workspace)
}

func normalizeCustomArguments(args []string) []string {
	result := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.TrimSpace(arg) != "" {
			result = append(result, arg)
		}
	}
	return result
}
