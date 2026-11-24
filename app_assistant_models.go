package main

import (
	"context"
	"errors"
	"path/filepath"
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
	if standardRuntimeKind(a.assistantRuntimeKind()) {
		conversation, err := a.prepareOpenCodeForModelsLocked()
		if err != nil {
			// models must stay a non-null array: the frontend treats it as a list.
			return AssistantModels{Models: []acp.LocalModelOption{}, Detail: err.Error()}, nil
		}
		return standardAssistantModels(conversation.SessionFeatures()), nil
	}
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

func standardAssistantModels(features acp.SessionFeatures) AssistantModels {
	result := AssistantModels{Models: []acp.LocalModelOption{}}
	for _, option := range features.Options {
		if option.Category != "model" {
			continue
		}
		for _, choice := range option.Choices {
			model := acp.LocalModelOption{ID: choice.Value, Name: choice.Name, ModelID: choice.Value, Provider: "Agent"}
			result.Models = append(result.Models, model)
			if choice.Value == option.Current {
				current := model
				result.Current = &current
			}
		}
	}
	if len(result.Models) == 0 {
		result.Detail = "当前 Agent 未提供模型列表"
	}
	return result
}

func (a *App) prepareOpenCodeForModelsLocked() (*acp.Conversation, error) {
	if a.assistantRunning {
		return nil, assistantcontrol.ErrBusy
	}
	if a.configMgr == nil {
		return nil, errors.New("助手设置不可用")
	}
	executable, err := a.assistantExecutable()
	if err != nil {
		return nil, err
	}
	workspace, err := a.assistantProjectWorkspaceJSON()
	if err != nil {
		return nil, err
	}
	config, err := a.assistantModelConfigLocked()
	if err != nil {
		return nil, err
	}
	config.Executable, config.ApprovalMode, config.WorkspaceJSON = executable, a.assistantApprovalMode, workspace
	config.SkillsDirectory, config.AuditDirectory = filepath.Join(a.configMgr.Directory(), "assistant-skills"), filepath.Join(a.configMgr.Directory(), "audit", "assistant")
	if a.assistantConversation == nil {
		if a.assistantRecovered {
			a.assistantConversation = acp.NewRecoveredConversation(assistantcontrol.RecoveryContext(a.assistantHistory.Snapshot()), "")
		} else {
			a.assistantConversation = &acp.Conversation{}
		}
	}
	if kind := a.assistantConversation.AgentKind(); kind != "" && kind != config.AgentKind {
		a.assistantConversation.Close()
		a.assistantRecovered = true
		a.assistantRecoveryNote = "Agent 底座已切换，将参考近期可见对话继续。"
		a.assistantConversation = acp.NewRecoveredConversation(assistantcontrol.RecoveryContext(a.assistantHistory.Snapshot()), "")
	}
	if a.assistantConversation.CustomRuntimeChanged(config) {
		a.assistantConversation.Close()
		a.assistantRecovered = true
		a.assistantRecoveryNote = "Agent 配置已变更，将参考近期可见对话继续。"
		a.assistantConversation = acp.NewRecoveredConversation(assistantcontrol.RecoveryContext(a.assistantHistory.Snapshot()), "")
	}
	if previous, started := a.assistantConversation.WorkspaceConfig(); started && previous != workspace {
		a.assistantConversation.Close()
		a.assistantRecovered = true
		a.assistantRecoveryNote = "项目工作区已变更，将参考近期可见对话继续；请核实旧项目中的结论。"
		a.assistantConversation = acp.NewRecoveredConversation(assistantcontrol.RecoveryContext(a.assistantHistory.Snapshot()), "")
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	startup, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	if err := a.assistantConversation.Prepare(ctx, startup, config); err != nil {
		a.assistantConversation.Close()
		a.assistantConversation = nil
		return nil, err
	}
	return a.assistantConversation, nil
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
	if standardRuntimeKind(a.assistantRuntimeKind()) {
		conversation, err := a.prepareOpenCodeForModelsLocked()
		if err != nil {
			return AssistantModels{}, err
		}
		ctx := a.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		if err = conversation.SwitchStandardModel(ctx, id); err != nil {
			return AssistantModels{}, err
		}
		return standardAssistantModels(conversation.SessionFeatures()), nil
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
		ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		if err = a.assistantConversation.SwitchLocalModel(ctx, model); err != nil {
			return AssistantModels{}, err
		}
	}
	a.assistantSelectedModel = model
	_ = a.saveAssistantHistoryLocked(false, nil)
	return AssistantModels{Models: options, Current: modelOption(model), Detail: a.assistantSaveWarning}, nil
}
