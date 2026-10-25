package main

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/terminalcontrol"
)

func TestAssistantLocalDirectoriesRestoreBusyAndNewConversation(t *testing.T) {
	a := newLifecycleApp()
	paths := []string{t.TempDir(), t.TempDir()}
	project, err := a.CreateAssistantLocalProject("A项目")
	if err != nil {
		t.Fatal(err)
	}
	id := project.ActiveID
	if _, err := a.AddAssistantProjectDirectories(id, paths); err != nil {
		t.Fatal(err)
	}
	if dirs, err := a.AssistantLocalDirectories(); err != nil || len(dirs) != 2 {
		t.Fatal("multi directory add failed")
	}
	context := a.assistantPageContext(terminalcontrol.Target{})
	if !strings.Contains(context, "localDirectories") {
		t.Fatal("directories missing from next message")
	}
	if _, err := a.RestoreAssistantConversation(); err != nil {
		t.Fatal("restore discarded paths", err)
	}
	a.assistantRunning = true
	if _, err := a.AddAssistantProjectDirectories(id, []string{t.TempDir()}); !errors.Is(err, assistantcontrol.ErrBusy) {
		t.Fatal("human scope mutated while running", err)
	}
	if _, err := a.RemoveAssistantProjectDirectory(id, paths[0]); !errors.Is(err, assistantcontrol.ErrBusy) {
		t.Fatal(err)
	}
	a.assistantRunning = false
	if err := a.NewAssistantConversation(); err != nil {
		t.Fatal(err)
	}
	if dirs, err := a.AssistantLocalDirectories(); err != nil || len(dirs) != 2 {
		t.Fatal("new conversation discarded saved project")
	}
	b, err := a.CreateAssistantLocalProject("B项目")
	if err != nil {
		t.Fatal(err)
	}
	context = a.assistantPageContext(terminalcontrol.Target{})
	var selected struct {
		LocalProject struct {
			Name string `json:"name"`
		} `json:"localProject"`
		LocalDirectories []string `json:"localDirectories"`
	}
	if err = json.Unmarshal([]byte(context), &selected); err != nil || selected.LocalProject.Name != "B项目" || len(selected.LocalDirectories) != 0 {
		t.Fatal("previous project context leaked", context)
	}
	if _, err = a.SelectAssistantLocalProject(id); err != nil {
		t.Fatal(err)
	}
	state, _ := a.AssistantLocalProjects()
	if state.ActiveID == b.ActiveID || len(state.Projects[0].Directories) != 2 {
		t.Fatal("project isolation failed", state)
	}
}
