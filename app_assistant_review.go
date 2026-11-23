package main

import (
	"context"
	"errors"
	"strings"

	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/llm"
)

// assistantRiskFloor is only used inside Ops-owned execution paths, after
// iCode's own approval. Explicit host-side risk checks take precedence and
// request a human decision. This function never calls a model.
func assistantRiskFloor(ctx context.Context, _ assistantcontrol.ReviewInput) assistantcontrol.ReviewVerdict {
	if ctx.Err() != nil {
		return assistantcontrol.ReviewVerdict{Reason: "本轮已停止"}
	}
	return assistantcontrol.ReviewVerdict{LowRisk: true, InScope: true, Reason: "未触发明确的高风险兜底规则"}
}

// assistantAgentReviewer is used only for agent runtimes that do not provide
// iCode's contextual command review. Unavailable fast models fail closed.
func (a *App) assistantAgentReviewer() assistantcontrol.Reviewer {
	if a.configMgr == nil || a.configMgr.Config == nil || a.aiService == nil {
		return assistantcontrol.AgentActionReviewer(nil)
	}
	cfg := a.configMgr.Config.LLM
	if strings.TrimSpace(cfg.FastModel) == "" || strings.TrimSpace(cfg.BaseURL) == "" {
		return assistantcontrol.AgentActionReviewer(nil)
	}
	provider := a.aiService.GetFastProvider()
	if provider == nil {
		return assistantcontrol.AgentActionReviewer(nil)
	}
	return assistantcontrol.AgentActionReviewer(func(ctx context.Context, system, input string) (string, error) {
		if ctx.Err() != nil {
			return "", errors.New("审查已取消")
		}
		messages := []llm.ChatMessage{{Role: "system", Content: system}, {Role: "user", Content: input}}
		if fast, ok := provider.(interface {
			ChatCompletionNoThinking(context.Context, []llm.ChatMessage) (string, error)
		}); ok {
			return fast.ChatCompletionNoThinking(ctx, messages)
		}
		return provider.ChatCompletion(ctx, messages)
	})
}

// Prior user messages are context for judging a continuation, not new grants.
// Exclude assistant/tool output and bound the information sent to the reviewer.
func assistantReviewContext(snapshot assistantcontrol.ConversationSnapshot, currentTurn string) []string {
	var result []string
	for i := len(snapshot.Messages) - 1; i >= 0 && len(result) < 6; i-- {
		message := snapshot.Messages[i]
		if message.Role != "user" || message.TurnID == currentTurn || strings.TrimSpace(message.Content) == "" {
			continue
		}
		result = append(result, assistantcontrol.DisplayExcerpt(message.Content, 1024))
	}
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result
}
