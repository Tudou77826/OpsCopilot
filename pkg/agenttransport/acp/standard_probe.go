package acp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// StandardProbe reports only observed protocol capabilities. In particular,
// it does not claim that an agent is integrated with OpsCopilot or that a
// model, approval policy or an MCP tool call is operational.
type StandardProbe struct {
	Name            string         `json:"name,omitempty"`
	Version         string         `json:"version,omitempty"`
	ProtocolVersion int            `json:"protocolVersion,omitempty"`
	HTTPMCP         bool           `json:"httpMcp"`
	LoadSession     bool           `json:"loadSession"`
	AdditionalRoots bool           `json:"additionalRoots"`
	ModelOptions    int            `json:"modelOptions"`
	ModeOptions     int            `json:"modeOptions"`
	Checks          []RuntimeCheck `json:"checks"`
}

// ProbeStandardAgent starts a temporary ACP process, handshakes and opens an
// isolated session. It never prompts the model, executes a tool or touches the
// active assistant session. Model connectivity and safety remain unverified.
func ProbeStandardAgent(ctx context.Context, executable string, args []string) (StandardProbe, error) {
	report := StandardProbe{Checks: []RuntimeCheck{
		{ID: "process", Label: "启动进程", Status: "skipped"},
		{ID: "handshake", Label: "ACP 握手", Status: "skipped"},
		{ID: "mcp", Label: "MCP stdio 工具接入", Status: "skipped", Detail: "实际工具调用尚未验证"},
		{ID: "session", Label: "创建会话", Status: "skipped"},
	}}
	if !filepath.IsAbs(executable) {
		return report, errors.New("Agent 程序路径必须为绝对路径")
	}
	dir, err := os.MkdirTemp("", "opscopilot-acp-check-")
	if err != nil {
		return report, errors.New("无法准备检测目录")
	}
	defer os.RemoveAll(dir)
	// Match the real custom-Agent environment so locally configured model
	// authentication and runtime dependencies can be found during startup.
	client, err := Start(ctx, ProcessConfig{Executable: executable, Args: args, Directory: dir, Environment: os.Environ()}, nil)
	if err != nil {
		report.Checks[0].Status = "failed"
		return report, err
	}
	defer func() {
		client.Close()
		wait, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = client.WaitProcess(wait)
	}()
	report.Checks[0].Status = "passed"
	var init struct {
		ProtocolVersion int `json:"protocolVersion"`
		AgentInfo       struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"agentInfo"`
		AgentCapabilities struct {
			LoadSession bool `json:"loadSession"`
			MCP         struct {
				HTTP bool `json:"http"`
			} `json:"mcpCapabilities"`
			Session struct {
				AdditionalDirectories json.RawMessage `json:"additionalDirectories"`
			} `json:"sessionCapabilities"`
		} `json:"agentCapabilities"`
	}
	if err = client.Call(ctx, "initialize", map[string]any{"protocolVersion": 1, "clientInfo": map[string]string{"name": "OpsCopilot", "version": "dev"}, "clientCapabilities": map[string]any{}}, &init); err != nil || init.ProtocolVersion != 1 {
		report.Checks[1].Status = "failed"
		return report, errors.New("ACP 握手失败或协议版本不支持")
	}
	report.Checks[1].Status = "passed"
	report.Name, report.Version, report.ProtocolVersion = init.AgentInfo.Name, init.AgentInfo.Version, init.ProtocolVersion
	report.HTTPMCP, report.LoadSession = init.AgentCapabilities.MCP.HTTP, init.AgentCapabilities.LoadSession
	roots := init.AgentCapabilities.Session.AdditionalDirectories
	report.AdditionalRoots = len(roots) != 0 && string(roots) != "null"
	var session struct {
		SessionID     string `json:"sessionId"`
		ConfigOptions []struct {
			Category string            `json:"category"`
			Options  []json.RawMessage `json:"options"`
		} `json:"configOptions"`
	}
	if err = client.Call(ctx, "session/new", map[string]any{"cwd": dir, "mcpServers": []any{}}, &session); err != nil || session.SessionID == "" {
		report.Checks[3].Status = "failed"
		return report, errors.New("Agent 未能创建测试会话")
	}
	report.Checks[3].Status = "passed"
	for _, option := range session.ConfigOptions {
		switch option.Category {
		case "model":
			report.ModelOptions += len(option.Options)
		case "mode":
			report.ModeOptions += len(option.Options)
		}
	}
	return report, nil
}

func probeEnvironment(dir string) []string {
	// Preserve platform startup variables and remove common credential variables.
	// XDG paths isolate test sessions, but agents can have other configuration
	// locations; no prompt or tool execution is allowed during this probe.
	var env []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		upper := strings.ToUpper(key)
		if strings.HasPrefix(upper, "XDG_") || strings.HasPrefix(upper, "OPENCODE_") || strings.Contains(upper, "API_KEY") || strings.Contains(upper, "TOKEN") || strings.Contains(upper, "SECRET") || strings.Contains(upper, "PASSWORD") || upper == "HOME" {
			continue
		}
		env = append(env, entry)
	}
	for _, key := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME", "XDG_STATE_HOME"} {
		env = append(env, key+"="+filepath.Join(dir, strings.ToLower(key)))
	}
	return append(env, "HOME="+dir, "OPENCODE_DISABLE_AUTOUPDATE=true", "OPENCODE_DISABLE_MODELS_FETCH=true")
}
