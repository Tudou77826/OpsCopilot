package acp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"opscopilot/pkg/assistantcontrol"
)

// SetStandardReviewer installs a one-action reviewer for non-iCode agents. It
// cannot itself grant terminal ownership or bypass the host's MCP safeguards.
func (c *Conversation) SetStandardReviewer(reviewer assistantcontrol.Reviewer) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.standardReviewer = reviewer
}

// startOpenCode builds an isolated session the same way the reference ACP
// client launches OpenCode: the resolved real executable, `acp` subcommand,
// the project (or isolated) directory as cwd, and the user's FULL environment
// — no env scrubbing, no HOME/XDG redirection, no synthesized configuration.
// OpenCode keeps its own model, auth and permission behavior; OpsCopilot
// contributes tools via session/new mcpServers and the first-prompt usage
// guide, which keeps the detection path and the chat path identical.
func (c *Conversation) startOpenCode(ctx, turnContext context.Context, config ManagedConfig) error {
	if _, err := normalizeApprovalMode(config.ApprovalMode); err != nil {
		return err
	}
	var workspace []string
	if config.WorkspaceJSON != "" && json.Unmarshal([]byte(config.WorkspaceJSON), &workspace) != nil {
		return errors.New("项目工作目录配置无效")
	}
	for _, path := range workspace {
		if !filepath.IsAbs(path) {
			return errors.New("项目工作目录必须是绝对路径")
		}
		info, statErr := os.Stat(path)
		if statErr != nil || !info.IsDir() {
			return errors.New("项目工作目录不可用")
		}
	}
	dir, err := os.MkdirTemp(config.ParentDirectory, "assistant-opencode-")
	if err != nil {
		return errors.New("无法准备助手工作目录")
	}
	c.directory, c.config = dir, config
	primary := dir
	if len(workspace) > 0 {
		primary = workspace[0]
	}
	env := append([]string(nil), os.Environ()...)
	process := ProcessConfig{Executable: config.Executable, Args: []string{"acp"}, Directory: primary, Environment: env}
	return c.startStandardSession(ctx, turnContext, "OpenCode", process, primary, workspace)
}
