package terminalcontrol

import (
	"context"
	"errors"
	"testing"
	"time"
)

func retainedTake(t *testing.T, a *Access, target Target) ControlledTerminal {
	t.Helper()
	run, err := a.Operations().Take(context.Background(), TakeRequest{Target: target, ConfirmedOrdinaryShell: true, RetainOnApproval: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = run.Release() })
	return run
}

func TestRetainedApprovalKeepsSameLeaseThroughReviewExecutionAndRead(t *testing.T) {
	a, s, c, w, target := protectedFixture(t, &memoryJournal{})
	run := retainedTake(t, a, target)
	id := requireReview(t, run, context.Background(), "printf hello")
	if !stateOf(t, s, "tab").Owned {
		t.Fatal("review released model ownership")
	}
	if err := c.HumanWrite([]byte("competing input")); !errors.Is(err, ErrOwned) {
		t.Fatal(err)
	}
	if result, err := run.Execute(context.Background(), CommandRequest{Command: "pwd"}); !errors.Is(err, ErrBusy) || result.Submission != NotSubmitted {
		t.Fatal(result, err)
	}
	op, err := a.Approve(id, true)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan commandReply, 1)
	go func() { r, e := op.Execute(context.Background()); done <- commandReply{r, e} }()
	nextWrite(t, w)
	if !stateOf(t, s, "tab").Owned {
		t.Fatal("dispatch lost ownership")
	}
	if err := c.HumanWrite([]byte("competing input")); !errors.Is(err, ErrOwned) {
		t.Fatal(err)
	}
	commandPrompt(c, 1, 0, "/retained", "hello")
	if reply := awaitReply(t, done); reply.err != nil || reply.result.RunID != run.ID() || reply.result.Completion != Completed {
		t.Fatal(reply)
	}
	if view, err := run.Inspect(ReadOptions{}); err != nil || !view.Owned {
		t.Fatal(view, err)
	}
	if _, err := op.Execute(context.Background()); !errors.Is(err, ErrStaleApproval) {
		t.Fatal(err)
	}
	// Approval authorizes the original command only; it cannot bless the next one.
	if second := requireReview(t, run, context.Background(), "printf different"); second == id {
		t.Fatal("approval reused")
	}
	_ = run.Release()
	if stateOf(t, s, "tab").Owned {
		t.Fatal("explicit release failed")
	}
}

func TestRetainedApprovalCannotResumeAfterRevocationOrChangedContext(t *testing.T) {
	for _, mode := range []string{"stop", "release", "scope_cancel", "caller_cancel", "identity", "replacement", "expiry", "denied"} {
		t.Run(mode, func(t *testing.T) {
			a, s, c, w, target := protectedFixture(t, &memoryJournal{})
			run := retainedTake(t, a, target)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			id := requireReview(t, run, ctx, "printf hello")
			op, err := a.Approve(id, mode != "denied")
			if mode == "denied" {
				if !errors.Is(err, ErrDenied) {
					t.Fatal(err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "stop":
				_ = s.Stop("tab", run.ID())
			case "release":
				_ = run.Release()
			case "scope_cancel":
				a.Cancel()
			case "caller_cancel":
				cancel()
			case "identity":
				c.Feed(frame(c, 0, "/changed"))
			case "replacement":
				s.Detach("tab", nil)
			case "expiry":
				a.mu.Lock()
				a.reviews[id].details.Expires = time.Now().Add(-time.Second)
				a.mu.Unlock()
			}
			if result, err := op.Execute(context.Background()); err == nil || result.Submission != NotSubmitted {
				t.Fatal(result, err)
			}
			select {
			case wire := <-w.writes:
				t.Fatalf("revoked command dispatched: %q", wire)
			default:
			}
		})
	}
}

func TestRoutineDiagnosticConsentIsHostIssuedAndSensitiveReadsStillAsk(t *testing.T) {
	for _, command := range []string{"ls", "ls -la /var/log", "cat app.log", "tail -n 100 ./app.log", "head -c 4096 /var/log/app.log", "grep -n ERROR /var/log/app.log"} {
		if assessCommand(command, AccessTarget{}, "/var/log").ReadOnly {
			t.Fatalf("unscoped read: %s", command)
		}
		if !assessCommand(command, AccessTarget{RoutineDiagnostics: true}, "/var/log").ReadOnly {
			t.Fatalf("routine read rejected: %s", command)
		}
	}
	for _, command := range []string{"cat /etc/shadow", "cat ../../root/.ssh/id_rsa", "grep password app.log", "cat /proc/1/environ", "tail -f app.log", "cat /dev/zero", "grep -R ERROR /", "cat app.log; reboot", "cat $(touch file)", "find /var/log -exec rm {} ;", "rm -rf /tmp/data", "systemctl restart app", "echo yes > config"} {
		a := assessCommand(command, AccessTarget{RoutineDiagnostics: true}, "/var/log")
		if a.ReadOnly || a.Category == "" || a.Impact == "" || a.Reason == "" {
			t.Fatalf("unsafe/opaque assessment %q: %+v", command, a)
		}
	}
	for _, filename := range []string{"/home/ops/.netrc", "/root/.docker/config.json", "/home/ops/.my.cnf", "/home/ops/.config/gcloud/application_default_credentials.json", "/home/ops/.bash_history", "/home/ops/.npmrc"} {
		if a := assessCommand("cat "+filename, AccessTarget{RoutineDiagnostics: true}, "/tmp"); a.ReadOnly || a.Category != "可能读取敏感信息" {
			t.Fatalf("sensitive diagnostic read passed: %s", filename)
		}
	}
}
