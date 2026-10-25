package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"opscopilot/pkg/assistantcontrol"
)

// SwitchLocalModel uses ACP's session-scoped model switch. It never replaces
// the conversation, replays a prompt, or writes the user's iCode configuration.
func (c *Conversation) SwitchLocalModel(ctx context.Context, model LocalModel) error {
	if !c.mu.TryLock() {
		return assistantcontrol.ErrBusy
	}
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("助手会话已结束，请新建对话后切换模型")
	}
	if c.client == nil {
		c.resetContextUsage()
		return nil
	} // The first prompt starts with the selection.
	profile, err := switchModelProfile(model)
	if err != nil {
		return err
	}
	// A separate profile keeps the active model intact if iCode rejects a switch.
	c.modelSwitch++
	id := fmt.Sprintf("opscopilot-switch-%d", c.modelSwitch)
	profile["id"] = id
	if err = c.client.callExtension(ctx, "profiles/models/write", map[string]any{"profile": profile}, nil); err != nil {
		return errors.New("模型配置未能载入当前 iCode 会话，尚未切换；请检查运行组件后重试")
	}
	if err = c.client.Call(ctx, "session/set_model", map[string]string{"sessionId": c.sessionID, "modelId": id}, nil); err != nil {
		// A transport error can leave the remote outcome unknown. Do not pretend
		// the old model is still usable or silently start a replacement session.
		select {
		case <-c.client.Done():
			c.close()
			return errors.New("模型切换未能确认，会话已停止；请新建对话后重试")
		default:
			return errors.New("iCode 未接受模型切换，仍使用原模型；请检查所选模型和 iCode 版本")
		}
	}
	c.config.LocalModel = model
	c.resetContextUsage()
	return nil
}

func switchModelProfile(model LocalModel) (map[string]any, error) {
	profile := map[string]any{}
	variables := map[string]string{}
	if json.Unmarshal([]byte(model.Profile), &profile) != nil || json.Unmarshal([]byte(model.Environment), &variables) != nil || stringValue(profile["model_id"]) == "" {
		return nil, errors.New("iCode 模型配置无法载入")
	}
	// A running process cannot acquire new environment variables. Materialize
	// only the selected model's imported references in its isolated profile via
	// the private stdio channel. Neither profile nor credentials reach the UI.
	for _, field := range []string{"api_key", "http_headers", "chat_options"} {
		value := stringValue(profile[field])
		for key, secret := range variables {
			if !strings.HasPrefix(key, "OPSCOPILOT_IMPORTED_MODEL_") {
				return nil, errors.New("iCode 模型认证引用无效")
			}
			value = strings.ReplaceAll(value, "{{"+key+"}}", secret)
		}
		profile[field] = value
	}
	return profile, nil
}
