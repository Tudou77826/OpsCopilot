package acp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"opscopilot/pkg/agenttransport/mcp"
)

// startStandardSession performs the shared standard-ACP startup: Ops tool
// bridge, process launch (with one automatic retry for cold-start flakiness),
// handshake, capability-gated additionalDirectories, native session resume,
// then a fresh session/new. Kind-specific validation and process construction
// stay with the caller.
func (c *Conversation) startStandardSession(ctx, turnContext context.Context, agentName string, process ProcessConfig, primary string, workspace []string) error {
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
	c.startupStage("process")
	client, capabilities, err := c.launchStandardAgent(ctx, turnContext, agentName, process)
	if err != nil {
		return err
	}
	c.client = client
	c.runtimeInfo = capabilities
	c.startupStage("capabilities")
	c.startupStage("session")
	startup, cancelStartup := context.WithTimeout(turnContext, 90*time.Second)
	defer cancelStartup()
	servers := []any{map[string]any{"name": "opscopilot", "command": proxy, "args": []string{"--assistant-mcp-stdio"}, "env": []any{
		map[string]string{"name": mcp.StdioAddressEnv, "value": bridge.Address},
		map[string]string{"name": mcp.StdioTokenEnv, "value": bridge.Token},
	}}}
	params := map[string]any{"cwd": primary, "mcpServers": servers}
	// Extra project roots ride session/new only when the agent declares the
	// capability; otherwise the per-message directory note remains the fallback.
	if len(workspace) > 1 && additionalDirectoriesDeclared(capabilities) {
		params["additionalDirectories"] = workspace[1:]
	}
	if c.loadResumedSession(startup, capabilities, primary, servers) {
		return nil
	}
	if !c.bindAgentSession(startup, agentName, client, "session/new", params) {
		return &managedFailure{message: agentName + " 会话创建失败，请检查认证和 MCP 支持", cause: errors.New("session/new returned no sessionId")}
	}
	return nil
}

// launchStandardAgent starts the agent process and completes the ACP handshake.
// Cold-start flakiness (first run, busy disk, antivirus scan) can push one
// handshake past the timeout even though the same launch succeeds seconds
// later, so a failed handshake is retried once with a fresh process; the
// underlying cause and child diagnostics surface in the final error.
func (c *Conversation) launchStandardAgent(ctx, turnContext context.Context, agentName string, process ProcessConfig) (*Client, InitializeResult, error) {
	var lastErr error
	for attempt := 1; attempt <= 2; attempt++ {
		if err := ctx.Err(); err != nil || turnContext.Err() != nil {
			return nil, InitializeResult{}, &managedFailure{message: agentName + " ACP 握手失败，请检查启动参数和运行环境", cause: err}
		}
		client, err := Start(ctx, process, &c.router)
		if err != nil {
			return nil, InitializeResult{}, err
		}
		c.startupStage("handshake")
		// 90s 与参考客户端默认请求超时一致：冷启动握手可能很慢。
		startup, cancel := context.WithTimeout(turnContext, 90*time.Second)
		capabilities, initErr := client.Initialize(startup)
		cancel()
		if initErr == nil {
			c.client = client
			return client, capabilities, nil
		}
		lastErr = initErr
		closed := client
		client.Close()
		wait, cancelWait := context.WithTimeout(context.Background(), 3*time.Second)
		_ = closed.WaitProcess(wait)
		cancelWait()
		// Read diagnostics after the child has been reaped so the exit code is
		// recorded and the summary is deterministic.
		detail := handshakeFailureDetail(lastErr, closed)
		if attempt == 2 {
			return nil, InitializeResult{}, &managedFailure{message: agentName + " ACP 握手失败（已自动重试一次）：" + detail, cause: lastErr}
		}
	}
	return nil, InitializeResult{}, &managedFailure{message: agentName + " ACP 握手失败，请检查启动参数和运行环境", cause: lastErr}
}

// handshakeFailureDetail combines the RPC cause with the child-process
// diagnostics (exit code / recognized startup problem) for the user message.
func handshakeFailureDetail(err error, client *Client) string {
	detail := err.Error()
	if diag := strings.TrimSpace(client.StartupDiagnostic()); diag != "" {
		detail += "；" + diag
	}
	if len(detail) > 300 {
		detail = detail[:300]
	}
	return detail
}

// bindAgentSession opens a session and binds its identity to the conversation.
// It reports success.
func (c *Conversation) bindAgentSession(ctx context.Context, agentName string, client *Client, method string, params map[string]any) bool {
	var result json.RawMessage
	if err := client.Call(ctx, method, params, &result); err != nil {
		return false
	}
	var session struct {
		SessionID string `json:"sessionId"`
	}
	if json.Unmarshal(result, &session) != nil || session.SessionID == "" {
		return false
	}
	c.features.update(result)
	c.sessionID = session.SessionID
	c.router.setSession(session.SessionID)
	return true
}

// loadResumedSession attempts to continue the previous native session after a
// process loss. Failure falls back to a fresh session and keeps the prose
// recovery context; success makes native history supersede it.
func (c *Conversation) loadResumedSession(ctx context.Context, capabilities InitializeResult, primary string, servers []any) bool {
	if c.resumeSessionID == "" || !capabilities.AgentCapabilities.LoadSession {
		return false
	}
	params := map[string]any{"sessionId": c.resumeSessionID, "cwd": primary, "mcpServers": servers}
	if !c.bindAgentSession(ctx, "", c.client, "session/load", params) {
		return false
	}
	c.recoveredContext = ""
	return true
}

func additionalDirectoriesDeclared(capabilities InitializeResult) bool {
	raw := capabilities.AgentCapabilities.SessionCapabilities.AdditionalDirectories
	return len(raw) != 0 && string(raw) != "null"
}

// prepareStandardWorkspace validates configured project directories and
// creates the isolated conversation working directory.
func prepareStandardWorkspace(config ManagedConfig, prefix string) ([]string, string, string, error) {
	var workspace []string
	if config.WorkspaceJSON != "" && json.Unmarshal([]byte(config.WorkspaceJSON), &workspace) != nil {
		return nil, "", "", errors.New("项目工作目录配置无效")
	}
	for _, path := range workspace {
		info, statErr := os.Stat(path)
		if !filepath.IsAbs(path) || statErr != nil || !info.IsDir() {
			return nil, "", "", errors.New("项目工作目录不可用")
		}
	}
	dir, err := os.MkdirTemp(config.ParentDirectory, prefix)
	if err != nil {
		return nil, "", "", errors.New("无法准备助手工作目录")
	}
	primary := dir
	if len(workspace) > 0 {
		primary = workspace[0]
	}
	return workspace, primary, dir, nil
}

// startCodeAgent launches a standard ACP CLI (default arguments: `acp`) with
// Ops tools via the MCP stdio bridge. The model and its authentication remain
// under the agent's control; AgentEnv carries only explicit user choices, with
// {{OPS_MODEL_API_KEY}} materialized from the model-service key at launch.
func (c *Conversation) startCodeAgent(ctx, turnContext context.Context, config ManagedConfig) error {
	if !filepath.IsAbs(config.Executable) {
		return errors.New("请填写 Agent 程序的绝对路径")
	}
	args := []string{"acp"}
	if config.ArgumentsJSON != "" && string(config.ArgumentsJSON) != "null" {
		if json.Unmarshal([]byte(config.ArgumentsJSON), &args) != nil {
			return errors.New("Agent 启动参数无效")
		}
		args = normalizeCustomArguments(args)
		if len(args) == 0 {
			args = []string{"acp"}
		}
	}
	env := append([]string(nil), os.Environ()...)
	if config.AgentEnvJSON != "" && string(config.AgentEnvJSON) != "null" {
		var variables map[string]string
		if json.Unmarshal([]byte(config.AgentEnvJSON), &variables) != nil {
			return errors.New("Agent 环境变量无效")
		}
		for _, name := range sortedAgentEnvNames(variables) {
			env = append(env, name+"="+materializeAgentEnvValue(variables[name], config.APIKey))
		}
	}
	workspace, primary, dir, err := prepareStandardWorkspace(config, "assistant-codeagent-")
	if err != nil {
		return err
	}
	c.directory, c.config = dir, config
	// Start routes .cmd/.bat launchers through cmd.exe for every caller.
	process := ProcessConfig{Executable: config.Executable, Args: args, Directory: primary, Environment: env}
	return c.startStandardSession(ctx, turnContext, "Agent", process, primary, workspace)
}

// materializeAgentEnvValue expands the single supported placeholder. The
// model-service API key is supplied at launch time and never stored with the
// configuration.
func materializeAgentEnvValue(value, apiKey string) string {
	return strings.ReplaceAll(value, "{{OPS_MODEL_API_KEY}}", apiKey)
}

func sortedAgentEnvNames(variables map[string]string) []string {
	names := make([]string, 0, len(variables))
	for name := range variables {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
