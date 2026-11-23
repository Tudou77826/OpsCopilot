package acp

import (
	"context"
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

// SwitchApprovalMode changes only the current iCode session, never the user's
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
	if c.client != nil && (c.config.AgentKind == "opencode" || c.config.AgentKind == "custom") {
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
