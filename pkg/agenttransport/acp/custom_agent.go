package acp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"opscopilot/pkg/agenttransport/mcp"
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
	var workspace []string
	if config.WorkspaceJSON != "" && json.Unmarshal([]byte(config.WorkspaceJSON), &workspace) != nil {
		return errors.New("项目工作目录配置无效")
	}
	for _, path := range workspace {
		info, err := os.Stat(path)
		if !filepath.IsAbs(path) || err != nil || !info.IsDir() {
			return errors.New("项目工作目录不可用")
		}
	}
	dir, err := os.MkdirTemp(config.ParentDirectory, "assistant-acp-")
	if err != nil {
		return errors.New("无法准备助手工作目录")
	}
	c.directory, c.config = dir, config
	ctx, c.cancel = context.WithCancel(ctx)
	c.router.observeUsage = c.observeContextUsage
	c.router.observeFeatures = c.features.update
	bridge, err := mcp.StartStdioBridge(ctx, &c.router)
	if err != nil {
		return errors.New("无法启动 Ops 工具桥接")
	}
	c.stdioBridge = bridge
	proxy, err := os.Executable()
	if err != nil {
		return errors.New("无法定位 Ops 工具程序")
	}
	primary := dir
	if len(workspace) > 0 {
		primary = workspace[0]
	}
	c.startupStage("process")
	client, err := Start(ctx, ProcessConfig{Executable: config.Executable, Args: args, Directory: primary, Environment: os.Environ()}, &c.router)
	if err != nil {
		return err
	}
	c.client = client
	startup, cancel := context.WithTimeout(turnContext, 45*time.Second)
	defer cancel()
	c.startupStage("handshake")
	capabilities, err := client.Initialize(startup)
	c.runtimeInfo = capabilities
	if err != nil {
		return &managedFailure{message: "Agent ACP 握手失败，请检查启动参数和运行环境", cause: err}
	}
	c.startupStage("session")
	servers := []any{map[string]any{"name": "opscopilot", "command": proxy, "args": []string{"--assistant-mcp-stdio"}, "env": []any{
		map[string]string{"name": mcp.StdioAddressEnv, "value": bridge.Address},
		map[string]string{"name": mcp.StdioTokenEnv, "value": bridge.Token},
	}}}
	var result json.RawMessage
	if err = client.Call(startup, "session/new", map[string]any{"cwd": primary, "mcpServers": servers}, &result); err != nil {
		return &managedFailure{message: "Agent 会话创建失败，请检查认证和 MCP 支持", cause: err}
	}
	var session struct {
		SessionID string `json:"sessionId"`
	}
	if json.Unmarshal(result, &session) != nil || session.SessionID == "" {
		return errors.New("Agent 未返回有效会话")
	}
	c.features.update(result)
	c.sessionID = session.SessionID
	c.router.setSession(session.SessionID)
	return nil
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
