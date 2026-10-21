package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/assistantsettings"
	"opscopilot/pkg/companion"
)

func (a *App) assistantPreferences() (*assistantsettings.Store, error) {
	if a.configMgr == nil {
		return nil, fmt.Errorf("伙伴助手配置未就绪")
	}
	return assistantsettings.New(filepath.Join(a.configMgr.Directory(), "assistant.json")), nil
}
func (a *App) GetAssistantSettings() (assistantsettings.Snapshot, error) {
	store, err := a.assistantPreferences()
	if err != nil {
		return assistantsettings.Snapshot{}, err
	}
	settings, err := store.Read()
	if err == nil && a.companion != nil && a.companion.Enabled() {
		snapshot, readErr := a.companion.ReadSnapshot()
		if readErr != nil {
			return settings, readErr
		}
		if snapshot.Pet != nil {
			settings.DefaultPrompt = companion.PersonaFor(snapshot.Pet.SpeciesID)
		}
	}
	return settings, err
}
func (a *App) SaveAssistantSettings(personaPrompt, revision string) (assistantsettings.Snapshot, error) {
	store, err := a.assistantPreferences()
	if err != nil {
		return assistantsettings.Snapshot{}, err
	}
	current, err := a.GetAssistantSettings()
	if err != nil {
		return current, err
	}
	if strings.TrimSpace(personaPrompt) == current.DefaultPrompt {
		personaPrompt = ""
	}
	if _, err = store.Save(personaPrompt, revision); err != nil {
		return assistantsettings.Snapshot{}, err
	}
	return a.GetAssistantSettings()
}

// Only the companion entry uses persona preferences; diagnosis and legacy callers retain their own prompts.
func (a *App) AskCompanionAI(question string) string {
	turn, beginErr := a.assistantCoordinator().Begin(a.ctx, assistantcontrol.Target{}, nil)
	if beginErr != nil {
		return fmt.Sprintf("Error: %v", beginErr)
	}
	defer turn.Finish("completed", "知识问答已结束")
	settings, err := a.GetAssistantSettings()
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	if a.aiService == nil {
		return "Error: AI 服务未就绪"
	}
	answer, err := a.aiService.AskWithPersona(turn.Context(), question, a.resolveKnowledgeBase(), settings.EffectivePersona())
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return answer
}
