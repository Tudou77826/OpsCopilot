package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"opscopilot/pkg/agenttransport/acp"
)

func (a *App) assistantExecutable() (string, error) {
	kind := a.assistantRuntimeKind()
	if kind == "custom" {
		if a.configMgr == nil || a.configMgr.Config == nil {
			return "", errors.New("助手设置不可用")
		}
		path := strings.TrimSpace(a.configMgr.Config.AssistantRuntime.Executable)
		if !filepath.IsAbs(path) {
			return "", errors.New("请填写 Agent 程序的绝对路径")
		}
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			return "", errors.New("找不到指定的 Agent 程序")
		}
		return path, nil
	}
	if kind == "opencode" {
		configured := ""
		if a.configMgr != nil && a.configMgr.Config != nil {
			configured = strings.TrimSpace(a.configMgr.Config.AssistantRuntime.Executable)
		}
		return resolveOpenCodeExecutable(configured)
	}
	if kind == "codeagent" {
		configured := ""
		if a.configMgr != nil && a.configMgr.Config != nil {
			configured = strings.TrimSpace(a.configMgr.Config.AssistantRuntime.Executable)
		}
		return resolveCodeAgentExecutable(configured)
	}
	if a.configMgr != nil && a.configMgr.Config != nil {
		if configured := strings.TrimSpace(a.configMgr.Config.AssistantRuntime.Executable); configured != "" {
			application, _ := os.Executable()
			return resolveAssistantExecutable(configured, application, exec.LookPath)
		}
	}
	return findChrysExecutable()
}

func (a *App) assistantRuntimeKind() string {
	if a.configMgr != nil && a.configMgr.Config != nil && a.configMgr.Config.AssistantRuntime.Kind != "" {
		return a.configMgr.Config.AssistantRuntime.Kind
	}
	// Standard ACP (CodeAgent) is the primary base; iCode remains available as
	// an explicit legacy selection. Previously saved explicit kinds never flip.
	return "codeagent"
}

func (a *App) AssistantAgentKind() string { return a.assistantRuntimeKind() }

// standardRuntimeKind reports whether the persisted kind speaks only standard
// ACP, so capability-driven behavior applies uniformly.
func standardRuntimeKind(kind string) bool {
	return kind == "opencode" || kind == "custom" || kind == "codeagent"
}

func resolveOpenCodeExecutable(configured string) (string, error) {
	if configured != "" {
		if !filepath.IsAbs(configured) {
			return "", errors.New("OpenCode 程序路径必须是绝对路径")
		}
		info, err := os.Stat(configured)
		if err != nil || info.IsDir() {
			return "", errors.New("找不到指定的 OpenCode 程序")
		}
		return configured, nil
	}
	if runtime.GOOS == "windows" {
		candidates := []string{filepath.Join(os.Getenv("APPDATA"), "npm", "node_modules", "opencode-ai", "bin", "opencode.exe"), filepath.Join(os.Getenv("USERPROFILE"), ".opencode", "bin", "opencode.exe")}
		for _, path := range candidates {
			if filepath.IsAbs(path) {
				if info, err := os.Stat(path); err == nil && !info.IsDir() {
					return path, nil
				}
			}
		}
	}
	if path, err := exec.LookPath("opencode"); err == nil {
		if runtime.GOOS != "windows" || strings.EqualFold(filepath.Ext(path), ".exe") {
			return filepath.Abs(path)
		}
	}
	return "", errors.New("尚未找到 OpenCode 程序，请安装或选择程序路径")
}

// resolveCodeAgentExecutable accepts an explicit .exe/.cmd/.bat path, resolves
// an extensionless path against same-directory launchers, or falls back to
// PATH. npm-style .cmd shims are returned as-is; the launcher wraps them.
func resolveCodeAgentExecutable(configured string) (string, error) {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		if !filepath.IsAbs(configured) {
			return "", errors.New("CodeAgent 程序路径必须是绝对路径")
		}
		resolved := configured
		if runtime.GOOS == "windows" {
			extended, err := resolveWindowsLauncher(configured)
			if err != nil {
				return "", err
			}
			resolved = extended
		}
		if info, err := os.Stat(resolved); err != nil || info.IsDir() {
			return "", errors.New("找不到指定的 CodeAgent 程序")
		}
		return resolved, nil
	}
	if path, err := exec.LookPath("codeagent"); err == nil {
		absolute, absErr := filepath.Abs(path)
		if absErr != nil {
			return "", absErr
		}
		if runtime.GOOS == "windows" {
			return resolveWindowsLauncher(absolute)
		}
		return absolute, nil
	}
	return "", errors.New("尚未找到 CodeAgent 程序，请安装 codeagent 并加入 PATH，或选择程序路径")
}

// resolveWindowsLauncher resolves extensionless Windows paths against the
// same-directory .exe/.cmd/.bat launchers, mirroring reference ACP clients.
func resolveWindowsLauncher(path string) (string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".exe", ".cmd", ".bat":
		return path, nil
	case "":
		for _, ext := range []string{".exe", ".cmd", ".bat"} {
			candidate := path + ext
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
		}
		return "", errors.New("该路径没有可用的 Windows 启动入口（.exe/.cmd/.bat）")
	default:
		return "", errors.New("Windows 下仅支持 .exe/.cmd/.bat 程序路径")
	}
}

func (a *App) assistantModelConfig(_ string) (acp.ManagedConfig, error) {
	a.assistantConversationMu.Lock()
	defer a.assistantConversationMu.Unlock()
	return a.assistantModelConfigLocked()
}

func (a *App) assistantModelConfigLocked() (acp.ManagedConfig, error) {
	if err := a.ensureAssistantHistoryLocked(); err != nil {
		return acp.ManagedConfig{}, err
	}
	if a.assistantRuntimeKind() == "opencode" {
		// 对齐参考 ACP 客户端：OpenCode 用它自己的模型与认证配置，启动环境
		// 完整继承用户环境；OpsCopilot 不注入合成 provider 配置。
		return acp.ManagedConfig{AgentKind: "opencode"}, nil
	}
	if a.assistantRuntimeKind() == "custom" {
		if a.configMgr == nil || a.configMgr.Config == nil {
			return acp.ManagedConfig{}, errors.New("助手设置不可用")
		}
		args, err := json.Marshal(a.configMgr.Config.AssistantRuntime.Arguments)
		if err != nil {
			return acp.ManagedConfig{}, errors.New("Agent 启动参数无效")
		}
		return acp.ManagedConfig{AgentKind: "custom", ArgumentsJSON: string(args)}, nil
	}
	if a.assistantRuntimeKind() == "codeagent" {
		if a.configMgr == nil || a.configMgr.Config == nil {
			return acp.ManagedConfig{}, errors.New("助手设置不可用")
		}
		args, err := json.Marshal(a.configMgr.Config.AssistantRuntime.Arguments)
		if err != nil {
			return acp.ManagedConfig{}, errors.New("Agent 启动参数无效")
		}
		env, err := json.Marshal(a.configMgr.Config.AssistantRuntime.AgentEnv)
		if err != nil {
			return acp.ManagedConfig{}, errors.New("Agent 环境变量无效")
		}
		return acp.ManagedConfig{AgentKind: "codeagent", ArgumentsJSON: string(args), AgentEnvJSON: string(env)}, nil
	}
	if a.assistantRuntimeKind() != "icode" {
		return acp.ManagedConfig{}, errors.New("不支持的 Agent 底座")
	}
	// Legacy saved source values are ignored: the assistant always uses iCode.
	if a.assistantSelectedModel.Profile != "" {
		return acp.ManagedConfig{LocalModel: a.assistantSelectedModel}, nil
	}
	if a.assistantSelectedModel.ID != "" {
		model, err := acp.LoadLocalModelByID(acp.LocalModelDirectory(), a.assistantSelectedModel.ID)
		return acp.ManagedConfig{LocalModel: model}, err
	}
	model, err := acp.LoadLocalModel(acp.LocalModelDirectory())
	return acp.ManagedConfig{LocalModel: model}, err
}

type AssistantModelStatus struct {
	Ready  bool   `json:"ready"`
	Name   string `json:"name,omitempty"`
	Detail string `json:"detail"`
}

// InspectAssistantModel returns metadata only. It does not send a model request
// and never returns profiles, endpoints, environment values or credentials.
func (a *App) InspectAssistantModel(source string) AssistantModelStatus {
	config, err := a.assistantModelConfig(source)
	if err != nil {
		return AssistantModelStatus{Detail: err.Error()}
	}
	if config.LocalModel.Profile != "" {
		return AssistantModelStatus{Ready: true, Name: config.LocalModel.Name, Detail: "已读取 iCode 模型及其认证配置；服务连通性将在首次对话时验证。"}
	}
	if config.Model == "" {
		return AssistantModelStatus{Ready: true, Detail: "模型与认证由 Agent 本机配置管理；服务连通性将在首次对话时验证。"}
	}
	return AssistantModelStatus{Ready: true, Name: config.Model, Detail: "使用 OpsCopilot「模型服务」配置；服务连通性将在首次对话时验证。"}
}

// CheckAssistantRuntime tests the draft path without saving it or touching the
// current conversation. The same managed startup path is used by real chat.
func (a *App) CheckAssistantRuntime(configured string) acp.RuntimeReport {
	application, _ := os.Executable()
	var executable string
	var err error
	if configured = strings.TrimSpace(configured); configured == "" {
		executable, err = findChrysExecutable()
	} else {
		executable, err = resolveAssistantExecutable(configured, application, exec.LookPath)
	}
	if err != nil {
		return acp.RuntimeReport{Executable: configured, Checks: []acp.RuntimeCheck{{ID: "executable", Label: "程序路径", Status: "failed", Detail: err.Error()}}, Advice: "请选择完整安装中的 iCode 程序，或留空使用自动查找。"}
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	return acp.InspectRuntime(ctx, executable)
}

func (a *App) CheckAssistantAgentRuntime(kind, configured string) acp.RuntimeReport {
	if kind == "" || kind == "icode" {
		return a.CheckAssistantRuntime(configured)
	}
	if kind != "opencode" && kind != "codeagent" {
		return acp.RuntimeReport{Advice: "不支持的 Agent 底座"}
	}
	resolve := resolveOpenCodeExecutable
	if kind == "codeagent" {
		resolve = resolveCodeAgentExecutable
	}
	executable, err := resolve(strings.TrimSpace(configured))
	if err != nil {
		return acp.RuntimeReport{Executable: configured, Checks: []acp.RuntimeCheck{{ID: "executable", Label: "程序路径", Status: "failed", Detail: err.Error()}}}
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	if kind == "opencode" {
		return acp.InspectOpenCodeRuntime(ctx, executable)
	}
	args := []string{"acp"}
	if a.configMgr != nil && a.configMgr.Config != nil && len(a.configMgr.Config.AssistantRuntime.Arguments) > 0 {
		args = a.configMgr.Config.AssistantRuntime.Arguments
	}
	return a.probeStandardRuntime(executable, args)
}

// probeStandardRuntime inspects protocol startup only, not model access,
// native permissions or actual Ops tool invocation.
func (a *App) probeStandardRuntime(executable string, args []string) acp.RuntimeReport {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.TrimSpace(arg) != "" {
			filtered = append(filtered, arg)
		}
	}
	result, err := acp.ProbeStandardAgent(ctx, strings.TrimSpace(executable), filtered)
	report := acp.RuntimeReport{Executable: executable, Name: result.Name, Version: result.Version, ProtocolVersion: result.ProtocolVersion, Checks: result.Checks, Capabilities: &acp.StandardCapabilities{LoadSession: result.LoadSession, AdditionalRoots: result.AdditionalRoots, HTTPMCP: result.HTTPMCP, ModelOptions: result.ModelOptions, ModeOptions: result.ModeOptions}}
	if err != nil {
		report.Advice = err.Error()
		return report
	}
	report.Ready = true
	report.Advice = "已建立测试会话；模型连通、工具调用和 Agent 本机权限行为尚未验证。"
	return report
}

// CheckCustomAssistantRuntime inspects protocol startup only, not model access,
// native permissions or actual Ops tool invocation.
func (a *App) CheckCustomAssistantRuntime(path string, args []string) acp.RuntimeReport {
	return a.probeStandardRuntime(path, args)
}

func (a *App) SelectAssistantRuntimeExecutable() (string, error) {
	pattern := "*"
	if runtime.GOOS == "windows" {
		pattern = "*.exe"
	}
	return wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{Title: "选择 Agent 程序", Filters: []wailsruntime.FileFilter{{DisplayName: "Agent 程序", Pattern: pattern}}})
}

func findChrysExecutable() (string, error) {
	configured := os.Getenv("OPSCOPILOT_ICODE_EXECUTABLE")
	if configured == "" {
		configured = os.Getenv("OPSCOPILOT_CHRYS_EXECUTABLE")
	}
	application, _ := os.Executable()
	return resolveAssistantExecutable(configured, application, exec.LookPath)
}

func resolveAssistantExecutable(configured, application string, lookup func(string) (string, error)) (string, error) {
	if configured != "" {
		if !filepath.IsAbs(configured) {
			return "", errors.New("iCode 程序路径必须是绝对路径")
		}
		info, err := os.Stat(configured)
		if err != nil || info.IsDir() {
			return "", errors.New("找不到指定的 iCode 程序，请检查运行组件路径")
		}
		return configured, nil
	}
	if filepath.IsAbs(application) {
		for _, relative := range []string{"icode/icode.exe", "icode/icode", "chrys/icode.exe", "chrys/icode", "chrys/chrys.exe", "chrys/chrys"} {
			candidate := filepath.Join(filepath.Dir(application), filepath.FromSlash(relative))
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
		}
	}
	for _, name := range []string{"icode", "chrys"} {
		if executable, err := lookup(name); err == nil {
			return filepath.Abs(executable)
		}
	}
	return "", errors.New("尚未找到 iCode 运行组件。请安装 iCode 并加入 PATH，或通过 OPSCOPILOT_ICODE_EXECUTABLE 指定程序绝对路径")
}
