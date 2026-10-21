package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/assistantskills"
)

type AssistantConnectionChoice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (a *App) assistantLibrary() (assistantskills.Library, error) {
	if a.configMgr == nil {
		return assistantskills.Library{}, errors.New("助手配置未就绪")
	}
	return assistantskills.Library{Directory: filepath.Join(a.configMgr.Directory(), "assistant-skills")}, nil
}
func (a *App) AssistantLibrary() (assistantskills.LibraryState, error) {
	library, err := a.assistantLibrary()
	if err != nil {
		return assistantskills.LibraryState{}, err
	}
	return library.Read()
}
func (a *App) mutateAssistantLibrary(change func(assistantskills.Library) error) error {
	a.assistantConversationMu.Lock()
	defer a.assistantConversationMu.Unlock()
	state := a.assistantCoordinator().Snapshot()
	if a.assistantRunning || state.Phase == "running" || state.Phase == "waiting" {
		return assistantcontrol.ErrBusy
	}
	library, err := a.assistantLibrary()
	if err != nil {
		return err
	}
	return change(library)
}
func (a *App) SelectAssistantSkillDirectory() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "选择包含 SKILL.md 的 Skill 目录"})
}
func (a *App) ImportAssistantSkill(directory string, replace bool, expected string) error {
	return a.mutateAssistantLibrary(func(l assistantskills.Library) error { return l.ImportAs(directory, replace, expected) })
}
func (a *App) EnableAssistantSkill(name string, enabled bool) error {
	return a.mutateAssistantLibrary(func(l assistantskills.Library) error { return l.Enable(name, enabled) })
}
func (a *App) RemoveAssistantSkill(name string) error {
	return a.mutateAssistantLibrary(func(l assistantskills.Library) error { return l.Remove(name) })
}
func (a *App) SaveAssistantCommand(command assistantskills.Command) error {
	return a.mutateAssistantLibrary(func(l assistantskills.Library) error { return l.SaveCommand(command) })
}
func (a *App) RemoveAssistantCommand(name string) error {
	return a.mutateAssistantLibrary(func(l assistantskills.Library) error { return l.RemoveCommand(name) })
}
func (a *App) AssistantCommandConnections() []AssistantConnectionChoice {
	choices := []AssistantConnectionChoice{}
	if a.savedSessionMgr != nil {
		connections, _ := savedWorkspaceConnections(a.savedSessionMgr.Snapshot())
		for _, c := range connections {
			choices = append(choices, AssistantConnectionChoice{c.ID, c.Name})
		}
	}
	return choices
}

// Prepare validates deterministic launch inputs before a turn or queued message
// starts. No command, connection or secret is executed/created by this method.
// Edited prose remains user input; authority still comes from runtime tools.
func (a *App) PrepareAssistantCommand(name, template string, values map[string]string) (string, error) {
	library, err := a.assistantLibrary()
	if err != nil {
		return "", err
	}
	state, err := library.Read()
	if err != nil {
		return "", err
	}
	var command *assistantskills.Command
	for _, c := range state.Commands {
		if c.Name == name {
			copy := c
			command = &copy
			break
		}
	}
	if command == nil {
		return "", errors.New("快捷命令已被移除，请重新选择")
	}
	if command.Skill != "" {
		enabled := false
		for _, s := range state.Skills {
			if s.Name == command.Skill {
				enabled = s.Enabled
			}
		}
		if !enabled {
			return "", errors.New("关联的 Skill 已停用或移除")
		}
	}
	edited := *command
	edited.Template = template
	if err = assistantskills.ValidateCommand(edited); err != nil {
		return "", err
	}
	connections := map[string]string{}
	for _, c := range a.AssistantCommandConnections() {
		connections[c.ID] = c.Name
	}
	replacements := []string{}
	for _, f := range command.Fields {
		value := strings.TrimSpace(values[f.Key])
		if (f.Required && value == "") || len(value) > 2048 {
			return "", fmt.Errorf("请填写 %s（最多 2 KB）", f.Label)
		}
		if strings.Contains(value, "{{") || strings.Contains(value, "}}") {
			return "", errors.New("填写内容不能包含模板标记")
		}
		if f.Kind == "connection" && value != "" {
			label, ok := connections[value]
			if !ok {
				return "", errors.New("所选 SSH 连接已不存在，请重新选择")
			}
			value = fmt.Sprintf("%s（已保存 SSH 连接 ID：%s）", label, value)
		}
		replacements = append(replacements, "{{"+f.Key+"}}", value)
	}
	rendered := strings.NewReplacer(replacements...).Replace(template)
	if command.Skill != "" {
		rendered = "请先加载 Skill「" + command.Skill + "」，按其中的指引完成以下要求：\n\n" + rendered
	}
	if len(rendered) > 16384 {
		return "", errors.New("填写后的指令不能超过 16 KB")
	}
	return rendered, nil
}
