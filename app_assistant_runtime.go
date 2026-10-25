package main

import (
	"context"
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
	if a.configMgr != nil && a.configMgr.Config != nil {
		if configured := strings.TrimSpace(a.configMgr.Config.AssistantRuntime.Executable); configured != "" {
			application, _ := os.Executable()
			return resolveAssistantExecutable(configured, application, exec.LookPath)
		}
	}
	return findChrysExecutable()
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
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	return acp.InspectRuntime(ctx, executable)
}

func (a *App) SelectAssistantRuntimeExecutable() (string, error) {
	pattern := "*"
	if runtime.GOOS == "windows" {
		pattern = "*.exe"
	}
	return wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{Title: "选择 iCode 程序", Filters: []wailsruntime.FileFilter{{DisplayName: "iCode 程序", Pattern: pattern}}})
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
