package main

import (
	"errors"

	"opscopilot/pkg/assistanttools"
)

// Read the same saved definitions as the terminal command panel and / menu.
// Definitions carry no credentials from connections and never dispatch actions.
func (a *App) assistantReferenceCommands() ([]assistanttools.ReferenceCommand, error) {
	if a.configMgr == nil {
		return nil, errors.New("助手配置未就绪")
	}
	items := []assistanttools.ReferenceCommand{}
	for _, c := range a.configMgr.QuickCommandsSnapshot() {
		items = append(items, assistanttools.ReferenceCommand{Kind: "terminal", ID: c.ID, Name: c.Name, Group: c.Group, Content: c.Content, Enabled: true})
	}
	state, err := a.AssistantLibrary()
	if err != nil {
		return nil, err
	}
	enabled := map[string]bool{}
	for _, s := range state.Skills {
		enabled[s.Name] = s.Enabled
	}
	for _, c := range state.Commands {
		items = append(items, assistanttools.ReferenceCommand{Kind: "assistant", ID: c.Name, Name: c.Name, Description: c.Description, Skill: c.Skill, Content: c.Template, Fields: c.Fields, Enabled: c.Skill == "" || enabled[c.Skill]})
	}
	return items, nil
}
