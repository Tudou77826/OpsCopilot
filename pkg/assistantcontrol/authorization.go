package assistantcontrol

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
)

var ErrActionDenied = errors.New("用户未批准本次操作")

type callApprovalKey struct{}

// WithCallApproval records an exact host-confirmed MCP invocation. The value is
// scoped to this call's context and is never accepted from tool arguments.
func WithCallApproval(ctx context.Context) context.Context {
	return context.WithValue(ctx, callApprovalKey{}, true)
}

func CallApproved(ctx context.Context) bool {
	approved, _ := ctx.Value(callApprovalKey{}).(bool)
	return approved
}

// ReviewAction is the shared desktop decision boundary. The caller supplies
// host-observed targets/effects, never a model's claim that it has permission.
// Decisions are exact-action, turn-bound and deliberately not cached as grants.
func (t *Turn) ReviewAction(ctx context.Context, reviewer Reviewer, input ReviewInput, humanReason string) (ReviewVerdict, error) {
	if ctx.Err() != nil || t.Context().Err() != nil {
		return ReviewVerdict{}, ErrStale
	}
	t.owner.mu.Lock()
	denied := t.rejectedActions[actionKey(input)]
	t.owner.mu.Unlock()
	if denied {
		return ReviewVerdict{}, ErrActionDenied
	}
	if t.ApprovalMode() == "bypass" || CallApproved(ctx) {
		return ReviewVerdict{LowRisk: true, InScope: true, Reason: "本次调用已按当前权限模式授权"}, nil
	}
	verdict := ReviewVerdict{}
	if humanReason != "" {
		verdict.Reason = humanReason
	} else if reviewer != nil {
		t.SetActivity("reviewing")
		linked, cancel := context.WithCancel(ctx)
		stop := context.AfterFunc(t.Context(), cancel)
		verdict = reviewer(linked, input)
		stop()
		cancel()
		t.SetActivity("")
	}
	if ctx.Err() != nil || t.Context().Err() != nil {
		return ReviewVerdict{}, ErrStale
	}
	if humanReason != "" {
		t.Record("review", "需要人工确认："+DisplayExcerpt(verdict.Reason, 512))
	}
	return verdict, nil
}

func actionKey(input ReviewInput) [32]byte {
	data, _ := json.Marshal([]string{input.Tool, input.Location, input.Action})
	return sha256.Sum256(data)
}

// ConfirmAction consumes a decision once. The caller must still validate the
// physical resource and audit dispatch immediately before executing the action.
func (t *Turn) ConfirmAction(ctx context.Context, input ReviewInput, prompt Prompt, verdict ReviewVerdict) error {
	_, err := t.ConfirmActionChoice(ctx, input, prompt, verdict)
	return err
}

// ConfirmActionChoice consumes a decision once and reports the exact choice ID
// the user selected. Choices listed as extra approve choices (beyond "allow")
// count as approval and are returned by ID; any other outcome, including no
// selection, is recorded as a rejection and maps to ErrActionDenied.
func (t *Turn) ConfirmActionChoice(ctx context.Context, input ReviewInput, prompt Prompt, verdict ReviewVerdict, extraApprove ...string) (string, error) {
	approved := map[string]bool{"allow": true}
	for _, choice := range extraApprove {
		approved[choice] = true
	}
	if ctx.Err() != nil || t.Context().Err() != nil {
		return "", ErrStale
	}
	if verdict.Allowed() {
		return "allow", nil
	}
	if verdict.Reason != "" {
		prompt.Details = append([]Detail{{Label: "确认原因", Value: DisplayExcerpt(verdict.Reason, 512)}}, prompt.Details...)
	}
	if verdict.Notice != "" {
		prompt.Details = append(prompt.Details, Detail{Label: "自审方式", Value: verdict.Notice})
	}
	answer, err := t.Request(ctx, prompt)
	if err != nil {
		return "", err
	}
	if len(answer.Selected) == 1 && approved[answer.Selected[0]] {
		return answer.Selected[0], nil
	}
	t.owner.mu.Lock()
	if t.rejectedActions == nil {
		t.rejectedActions = make(map[[32]byte]bool)
	}
	t.rejectedActions[actionKey(input)] = true
	t.owner.mu.Unlock()
	return "", ErrActionDenied
}

func (t *Turn) Authorize(ctx context.Context, reviewer Reviewer, input ReviewInput, prompt Prompt, humanReason string) error {
	verdict, err := t.ReviewAction(ctx, reviewer, input, humanReason)
	if err != nil {
		return err
	}
	return t.ConfirmAction(ctx, input, prompt, verdict)
}
