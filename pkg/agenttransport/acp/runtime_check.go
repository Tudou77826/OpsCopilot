package acp

import "context"

type RuntimeCheck struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type RuntimeReport struct {
	Ready               bool           `json:"ready"`
	Executable          string         `json:"executable"`
	Name                string         `json:"name,omitempty"`
	Version             string         `json:"version,omitempty"`
	ProtocolVersion     int            `json:"protocolVersion,omitempty"`
	ExtensionAPIVersion int            `json:"extensionApiVersion,omitempty"`
	Checks              []RuntimeCheck `json:"checks"`
	Advice              string         `json:"advice,omitempty"`
}

// InspectRuntime exercises the real isolated startup, profiles and MCP binding.
// It never sends session/prompt, loads user Skills or contacts a model service.
func InspectRuntime(ctx context.Context, executable string) RuntimeReport {
	report := RuntimeReport{Executable: executable, Checks: []RuntimeCheck{
		{ID: "executable", Label: "程序路径", Status: "passed"},
		{ID: "process", Label: "运行环境启动", Status: "skipped"},
		{ID: "handshake", Label: "ACP 握手", Status: "skipped"},
		{ID: "capabilities", Label: "接口兼容性", Status: "skipped"},
		{ID: "profile", Label: "助手配置与工具", Status: "skipped"},
		{ID: "session", Label: "独立会话初始化", Status: "skipped"},
	}}
	current := 0
	c := &Conversation{onStartupStage: func(stage string) {
		if current > 0 {
			report.Checks[current].Status = "passed"
		}
		for index, check := range report.Checks {
			if check.ID == stage {
				current = index
				break
			}
		}
	}}
	defer c.Close()
	err := c.start(ctx, ctx, ManagedConfig{Executable: executable, BaseURL: "http://127.0.0.1:1/v1", Model: "runtime-check"})
	report.Name = c.runtimeInfo.AgentInfo.Name
	report.Version = c.runtimeInfo.AgentInfo.Version
	report.ProtocolVersion = c.runtimeInfo.ProtocolVersion
	if extensions := c.runtimeInfo.AgentCapabilities.Meta.Chrys; extensions != nil {
		report.ExtensionAPIVersion = extensions.APIVersion
	}
	if err != nil {
		report.Checks[current].Status = "failed"
		report.Checks[current].Detail = err.Error()
		report.Advice = "请修复所选 iCode 安装及其 Python 依赖后重新检测；只安装 Python 不代表 iCode 可以启动。"
		if current == 3 {
			report.Advice = "请更新所选 iCode；当前版本缺少助手所需的 ACP 接口。"
		}
		return report
	}
	report.Checks[current].Status = "passed"
	report.Ready = true
	return report
}
