package main

import (
	"context"
	"sync"

	"opscopilot/pkg/agenttransport/acp"
	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/assistantreview"
)

func (a *App) assistantReviewer(turn *assistantcontrol.Turn, managed acp.ManagedConfig, question string) assistantcontrol.Reviewer {
	var fast assistantreview.Classifier
	if a.configMgr != nil && a.configMgr.Config != nil {
		config := a.configMgr.Config.LLM
		// Only the explicitly configured fast model is used for direct review.
		// The conversation itself continues to use the native iCode model.
		fast = assistantreview.NewFastClassifier(config.BaseURL, config.APIKey, config.FastModel)
	}
	var noticeOnce sync.Once
	review := assistantreview.Service{Fast: fast, Fallback: func(ctx context.Context, input assistantcontrol.ReviewInput) assistantcontrol.ReviewVerdict {
		return acp.ReviewAction(ctx, managed, input)
	}, OnFallback: func(notice string) { noticeOnce.Do(func() { turn.Record("review_fallback", notice) }) }}
	return func(ctx context.Context, input assistantcontrol.ReviewInput) assistantcontrol.ReviewVerdict {
		input.UserRequest = question
		return review.Review(ctx, input)
	}
}
