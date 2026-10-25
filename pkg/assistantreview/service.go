// Package assistantreview selects a bounded fast-model classification first,
// falling back only when that model is missing or its request fails.
package assistantreview

import (
	"context"
	"strings"
	"time"

	"opscopilot/pkg/assistantcontrol"
)

const FastTimeout = 12 * time.Second

type Classifier func(context.Context, assistantcontrol.ReviewInput) (string, error)

type Service struct {
	Fast       Classifier
	Fallback   assistantcontrol.Reviewer
	OnFallback func(string)
}

func (s Service) Review(ctx context.Context, input assistantcontrol.ReviewInput) assistantcontrol.ReviewVerdict {
	failure := assistantcontrol.ReviewVerdict{Reason: "自审未完成，需人工确认"}
	if ctx.Err() != nil || len(input.Action) > 16*1024 || len(input.UserRequest) > 16*1024 {
		return failure
	}
	if s.Fast == nil {
		return s.fallback(ctx, input, "未配置快速模型")
	}
	fastCtx, cancel := context.WithTimeout(ctx, FastTimeout)
	choice, err := s.Fast(fastCtx, input)
	requestErr := fastCtx.Err()
	cancel()
	if ctx.Err() != nil {
		return failure
	}
	if err != nil || requestErr != nil {
		return s.fallback(ctx, input, "快速模型访问失败")
	}
	// A valid human-required decision or an invalid choice never triggers another
	// model call. In particular, do not ask a second model for a different verdict.
	return verdict(choice)
}

func (s Service) fallback(ctx context.Context, input assistantcontrol.ReviewInput, reason string) assistantcontrol.ReviewVerdict {
	result := assistantcontrol.ReviewVerdict{Reason: "自审未完成，需人工确认"}
	if ctx.Err() != nil {
		return result
	}
	notice := reason + "，本次自审改用隔离的 iCode 会话，等待可能较长。建议在「设置 → 模型服务」配置可用的快速模型，自审通常会更快。"
	if s.OnFallback != nil {
		s.OnFallback(notice)
	}
	if ctx.Err() == nil && s.Fallback != nil {
		result = s.Fallback(ctx, input)
	}
	if ctx.Err() != nil {
		return assistantcontrol.ReviewVerdict{Reason: "本轮已停止，自审结果不再有效"}
	}
	result.Notice = notice
	return result
}

func verdict(choice string) assistantcontrol.ReviewVerdict {
	switch strings.TrimSpace(choice) {
	case "A":
		return assistantcontrol.ReviewVerdict{ReadOnly: true, InScope: true, Reason: "快速模型判定：属于本轮要求范围内的非敏感只读检查"}
	case "B":
		return assistantcontrol.ReviewVerdict{InScope: true, Reason: "快速模型判定：涉及写入、状态或权限变化，需人工确认"}
	case "C":
		return assistantcontrol.ReviewVerdict{Sensitive: true, Reason: "快速模型判定：涉及敏感信息或对外披露，需人工确认"}
	case "D":
		return assistantcontrol.ReviewVerdict{Reason: "快速模型判定：操作范围或目标与本轮要求不匹配，需人工确认"}
	case "E":
		return assistantcontrol.ReviewVerdict{Reason: "快速模型判定：信息不足，无法确认只读和授权范围，需人工确认"}
	default:
		return assistantcontrol.ReviewVerdict{Reason: "快速模型未返回有效选项，需人工确认"}
	}
}
