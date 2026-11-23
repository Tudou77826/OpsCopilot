package acp

import (
	"context"
	"encoding/json"
	"errors"

	"opscopilot/pkg/assistantcontrol"
)

// SwitchStandardModel uses the model option advertised by the active ACP
// session. It never invents model identifiers or rewrites agent configuration.
func (c *Conversation) SwitchStandardModel(ctx context.Context, id string) error {
	if !c.mu.TryLock() {
		return assistantcontrol.ErrBusy
	}
	defer c.mu.Unlock()
	if c.closed || c.client == nil || c.sessionID == "" {
		return errors.New("助手会话尚未就绪，无法切换模型")
	}
	optionID := ""
	for _, option := range c.features.snapshot().Options {
		if option.Category != "model" {
			continue
		}
		for _, choice := range option.Choices {
			if choice.Value == id {
				optionID = option.ID
				break
			}
		}
		if optionID != "" {
			break
		}
	}
	if optionID == "" {
		return errors.New("当前 Agent 未提供所选模型")
	}
	var result json.RawMessage
	if err := c.client.Call(ctx, "session/set_config_option", map[string]string{"sessionId": c.sessionID, "configId": optionID, "value": id}, &result); err != nil {
		// A failed call could have reached the agent. Avoid presenting stale model
		// state or continuing a conversation with unknown configuration.
		c.close()
		return errors.New("模型切换未能确认，会话已停止；请重新选择")
	}
	c.features.update(result)
	confirmed := false
	for _, option := range c.features.snapshot().Options {
		if option.ID == optionID && option.Current == id {
			confirmed = true
			break
		}
	}
	if !confirmed {
		c.close()
		return errors.New("Agent 未确认模型切换，会话已停止；请重新选择")
	}
	c.resetContextUsage()
	return nil
}
