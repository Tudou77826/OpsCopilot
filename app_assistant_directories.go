package main

import (
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/assistantlocal"
	"path/filepath"
)

func (a *App) assistantLocalProjectStore() *assistantlocal.Projects {
	a.assistantProjectsOnce.Do(func() {
		a.assistantProjects = &assistantlocal.Projects{}
		if a.configMgr != nil {
			a.assistantProjects.File = filepath.Join(a.configMgr.Directory(), "assistant-projects.json")
		}
	})
	return a.assistantProjects
}
func (a *App) AssistantLocalProjects() (assistantlocal.ProjectState, error) {
	return a.assistantLocalProjectStore().Snapshot()
}
func (a *App) AssistantLocalDirectories() ([]string, error) {
	s, err := a.AssistantLocalProjects()
	if err != nil {
		return nil, err
	}
	p, _ := s.ActiveProject()
	return append([]string{}, p.Directories...), nil
}
func (a *App) changeAssistantProject(change func(*assistantlocal.Projects) (assistantlocal.ProjectState, error)) (assistantlocal.ProjectState, error) {
	a.assistantConversationMu.Lock()
	defer a.assistantConversationMu.Unlock()
	if a.assistantRunning {
		return assistantlocal.ProjectState{}, assistantcontrol.ErrBusy
	}
	return change(a.assistantLocalProjectStore())
}
func (a *App) CreateAssistantLocalProject(name string) (assistantlocal.ProjectState, error) {
	return a.changeAssistantProject(func(s *assistantlocal.Projects) (assistantlocal.ProjectState, error) { return s.Create(name) })
}
func (a *App) SelectAssistantLocalProject(id string) (assistantlocal.ProjectState, error) {
	return a.changeAssistantProject(func(s *assistantlocal.Projects) (assistantlocal.ProjectState, error) { return s.Select(id) })
}
func (a *App) RenameAssistantLocalProject(id, name string) (assistantlocal.ProjectState, error) {
	return a.changeAssistantProject(func(s *assistantlocal.Projects) (assistantlocal.ProjectState, error) { return s.Rename(id, name) })
}
func (a *App) DeleteAssistantLocalProject(id string) (assistantlocal.ProjectState, error) {
	return a.changeAssistantProject(func(s *assistantlocal.Projects) (assistantlocal.ProjectState, error) { return s.Delete(id) })
}
func (a *App) AddAssistantProjectDirectories(id string, paths []string) (assistantlocal.ProjectState, error) {
	return a.changeAssistantProject(func(s *assistantlocal.Projects) (assistantlocal.ProjectState, error) { return s.Add(id, paths) })
}
func (a *App) RemoveAssistantProjectDirectory(id, path string) (assistantlocal.ProjectState, error) {
	return a.changeAssistantProject(func(s *assistantlocal.Projects) (assistantlocal.ProjectState, error) { return s.Remove(id, path) })
}
func (a *App) SelectAssistantLocalDirectory() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "为项目添加本地目录"})
}
