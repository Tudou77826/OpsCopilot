package acp

import (
	"context"
	"encoding/json"
	"errors"

	"opscopilot/pkg/assistantcontrol"
)

func normalizeApprovalMode(mode string) (string, error) {
	switch mode {
	case "", "auto":
		return "auto", nil
	case "manual", "bypass":
		return mode, nil
	default:
		return "", errors.New("不支持的权限模式")
	}
}

// SwitchApprovalMode changes only the current session, never the user's
// global configuration or Ops' independent guards for remote operations.
func (c *Conversation) SwitchApprovalMode(ctx context.Context, mode string) error {
	mode, err := normalizeApprovalMode(mode)
	if err != nil {
		return err
	}
	if !c.mu.TryLock() {
		return assistantcontrol.ErrBusy
	}
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("助手会话已结束，请新建对话后切换权限模式")
	}
	if c.client != nil && c.config.IsStandardACP() {
		// Only a genuinely attempted switch can leave the session in an unknown
		// mode; a silent mismatch must never be reused.
		if err = c.switchStandardMode(ctx, mode); err != nil {
			c.close()
			return err
		}
		c.config.ApprovalMode = mode
		return nil
	}
	if c.client != nil {
		if err := c.client.Call(ctx, "session/set_mode", map[string]string{"sessionId": c.sessionID, "modeId": mode}, nil); err != nil {
			// A failed transport call may have reached iCode. Close the session so
			// a potentially changed mode is never silently reused.
			c.close()
			return errors.New("权限模式切换未能确认，会话已停止；请重新选择后继续")
		}
	}
	c.config.ApprovalMode = mode
	return nil
}

// switchStandardMode asks the agent to change its advertised mode option via
// session/set_config_option, falling back to legacy session/set_mode only on
// method-not-found. Agents that publish no matching mode choice are skipped:
// the local record stays the only authoritative state, mirroring the previous
// record-only behavior.
func (c *Conversation) switchStandardMode(ctx context.Context, mode string) error {
	optionID := ""
	for _, option := range c.features.snapshot().Options {
		if option.Category != "mode" {
			continue
		}
		for _, choice := range option.Choices {
			if choice.Value == mode {
				optionID = option.ID
				break
			}
		}
		if optionID != "" {
			break
		}
	}
	if optionID == "" {
		return nil
	}
	var result json.RawMessage
	if err := c.client.Call(ctx, "session/set_config_option", map[string]string{"sessionId": c.sessionID, "configId": optionID, "value": mode}, &result); err != nil {
		var rpc *RPCError
		if errors.As(err, &rpc) && rpc.Code == -32601 {
			if legacy := c.client.Call(ctx, "session/set_mode", map[string]string{"sessionId": c.sessionID, "modeId": mode}, nil); legacy != nil {
				return errors.New("权限模式切换未能确认，会话已停止；请重新选择后继续")
			}
			return nil
		}
		return errors.New("权限模式切换未能确认，会话已停止；请重新选择后继续")
	}
	c.features.update(result)
	for _, option := range c.features.snapshot().Options {
		if option.ID == optionID && option.Current == mode {
			return nil
		}
	}
	return errors.New("Agent 未确认权限模式切换，会话已停止；请重新选择后继续")
}
