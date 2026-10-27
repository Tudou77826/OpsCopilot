package terminalcontrol

import (
	"context"
	"errors"
	"testing"
)

func TestAIReviewRetainsLeaseExactCommandAndSeparateAudit(t *testing.T) {
	j := &memoryJournal{}
	a, s, c, w, target := protectedFixture(t, j)
	run := retainedTake(t, a, target)
	id := requireReview(t, run, context.Background(), "printf hello")
	if err := a.RecordPolicyReview(id, true); err != nil {
		t.Fatal(err)
	}
	op, err := a.ApproveReadOnlyReview(id, "仅输出诊断文本")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan commandReply, 1)
	go func() { r, e := op.Execute(context.Background()); done <- commandReply{r, e} }()
	nextWrite(t, w)
	if !stateOf(t, s, "tab").Owned {
		t.Fatal("self review released ownership")
	}
	commandPrompt(c, 1, 0, "/retained", "hello")
	if reply := awaitReply(t, done); reply.err != nil {
		t.Fatal(reply.err)
	}
	found := false
	for _, event := range j.snapshot() {
		if event.Phase == "dispatch_intent" && event.Reason == "host_policy_review" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing AI decision audit")
	}
	if _, err := op.Execute(context.Background()); !errors.Is(err, ErrStaleApproval) {
		t.Fatal("review replayed", err)
	}
}

func TestAIReviewCannotApproveMutationOrRevokedLease(t *testing.T) {
	for _, command := range []string{"rm /tmp/data", "echo x | tee /tmp/config", "cat /root/.ssh/id_rsa", "systemctl restart api", "printf hello"} {
		t.Run(command, func(t *testing.T) {
			a, _, _, w, target := protectedFixture(t, &memoryJournal{})
			run := retainedTake(t, a, target)
			id := requireReview(t, run, context.Background(), command)
			if command == "printf hello" {
				_ = run.Release()
			}
			if _, err := a.ApproveReadOnlyReview(id, "错误的只读判断"); err == nil {
				t.Fatal("human boundary or revocation bypassed")
			}
			select {
			case <-w.writes:
				t.Fatal("unexpected command dispatch")
			default:
			}
		})
	}
}
