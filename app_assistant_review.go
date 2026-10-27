package main

import (
	"context"

	"opscopilot/pkg/assistantcontrol"
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
