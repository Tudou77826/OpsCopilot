package assistantcontrol

import (
	"context"
	"errors"
	"testing"
	"time"
)

func riskPrompt() Prompt {
	return Prompt{Kind: "approval", Title: "确认操作后果及范围", Choices: []Choice{{ID: "reject", Label: "拒绝"}, {ID: "allow", Label: "允许本次"}}, Expires: time.Now().Add(time.Minute)}
}

func TestTaskAuthorizationSeparatesConsequenceAndScope(t *testing.T) {
	for _, tc := range []struct {
		name                                string
		low, scope, sensitive, force, allow bool
	}{
		{"requested save", true, true, false, false, true},
		{"out of scope write", true, false, false, false, false},
		{"high consequence", false, true, false, false, false},
		{"sensitive read", true, true, true, false, false},
		{"host floor overrides model", true, true, false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &Coordinator{}
			turn, _ := c.Begin(context.Background(), Target{}, nil)
			defer turn.Finish("completed", "")
			calls := 0
			review := func(context.Context, ReviewInput) ReviewVerdict {
				calls++
				return ReviewVerdict{LowRisk: tc.low, InScope: tc.scope, Sensitive: tc.sensitive, Reason: "fixture"}
			}
			input := ReviewInput{Tool: "edit", Location: "project A", Action: "write report"}
			force := ""
			if tc.force {
				force = "destructive overwrite"
			}
			done := make(chan error, 1)
			go func() { done <- turn.Authorize(turn.Context(), review, input, riskPrompt(), force) }()
			if tc.allow {
				select {
				case err := <-done:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(time.Second):
					t.Fatal("low-risk task unnecessarily asked human")
				}
				if c.Snapshot().Prompt != nil {
					t.Fatal("unexpected approval")
				}
				return
			}
			deadline := time.Now().Add(time.Second)
			for c.Snapshot().Prompt == nil && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			p := c.Snapshot().Prompt
			if p == nil {
				t.Fatal("risk bypassed human")
			}
			if err := c.Respond(turn.ID(), p.ID, Answer{Selected: []string{"reject"}}); err != nil {
				t.Fatal(err)
			}
			if err := <-done; !errors.Is(err, ErrActionDenied) {
				t.Fatal(err)
			}
			if _, err := turn.ReviewAction(turn.Context(), review, input, ""); !errors.Is(err, ErrActionDenied) {
				t.Fatal("refusal was reset", err)
			}
			if tc.force && calls != 0 {
				t.Fatal("high risk spent a model call")
			}
		})
	}
}

func TestTaskReviewCannotAllowAfterStop(t *testing.T) {
	c := &Coordinator{}
	turn, _ := c.Begin(context.Background(), Target{}, nil)
	started, finish := make(chan struct{}), make(chan struct{})
	review := func(context.Context, ReviewInput) ReviewVerdict {
		close(started)
		<-finish
		return ReviewVerdict{LowRisk: true, InScope: true, Reason: "late"}
	}
	done := make(chan error, 1)
	go func() {
		done <- turn.Authorize(context.Background(), review, ReviewInput{Action: "save"}, riskPrompt(), "")
	}()
	<-started
	_ = c.Stop(turn.ID())
	close(finish)
	if err := <-done; !errors.Is(err, ErrStale) {
		t.Fatal("late verdict accepted", err)
	}
}
