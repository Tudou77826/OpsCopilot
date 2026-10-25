package main

import (
	"context"
	"time"

	"opscopilot/pkg/agenttransport/acp"
	"opscopilot/pkg/assistantcontrol"
)

type AssistantModels struct {
	Models  []acp.LocalModelOption `json:"models"`
	Current *acp.LocalModelOption  `json:"current,omitempty"`
	Detail  string                 `json:"detail,omitempty"`
}

func modelOption(model acp.LocalModel) *acp.LocalModelOption {
	return &acp.LocalModelOption{ID: model.ID, Name: model.Name, ModelID: model.ModelID, Provider: model.Provider}
}

// AssistantModelOptions reads metadata without starting iCode or contacting a model.
func (a *App) AssistantModelOptions() (AssistantModels, error) {
	a.assistantConversationMu.Lock()
	defer a.assistantConversationMu.Unlock()
	return a.assistantModelsLocked()
}

func (a *App) assistantModelsLocked() (AssistantModels, error) {
	options, err := acp.ListLocalModels(acp.LocalModelDirectory())
	if err != nil {
		return AssistantModels{}, err
	}
	result := AssistantModels{Models: options}
	config, err := a.assistantModelConfigLocked()
	if err != nil {
		result.Detail = err.Error()
		return result, nil
	}
	result.Current = modelOption(config.LocalModel)
	return result, nil
}

func (a *App) SelectAssistantModel(id string) (AssistantModels, error) {
	a.assistantConversationMu.Lock()
	defer a.assistantConversationMu.Unlock()
	if err := a.ensureAssistantHistoryLocked(); err != nil {
		return AssistantModels{}, err
	}
	state := a.assistantCoordinator().Snapshot()
	if a.assistantRunning || state.Phase == "running" || state.Phase == "waiting" {
		return AssistantModels{}, assistantcontrol.ErrBusy
	}
	model, err := acp.LoadLocalModelByID(acp.LocalModelDirectory(), id)
	if err != nil {
		return AssistantModels{}, err
	}
	// Validate the catalog before mutating state, including duplicate IDs.
	options, err := acp.ListLocalModels(acp.LocalModelDirectory())
	if err != nil {
		return AssistantModels{}, err
	}
	if a.assistantSelectedModel == model {
		return AssistantModels{Models: options, Current: modelOption(model)}, nil
	}
	if a.assistantConversation != nil {
		ctx := a.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		if err = a.assistantConversation.SwitchLocalModel(ctx, model); err != nil {
			return AssistantModels{}, err
		}
	}
	a.assistantSelectedModel = model
	_ = a.saveAssistantHistoryLocked(false, nil)
	return AssistantModels{Models: options, Current: modelOption(model), Detail: a.assistantSaveWarning}, nil
}
