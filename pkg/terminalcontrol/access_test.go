package terminalcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type memoryJournal struct {
	mu     sync.Mutex
	events []ExecutionAudit
	hook   func(ExecutionAudit) error
}

func (j *memoryJournal) Append(_ context.Context, event ExecutionAudit) error {
	if j.hook != nil {
		if err := j.hook(event); err != nil {
			return err
		}
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.events = append(j.events, event)
	return nil
}
func (j *memoryJournal) snapshot() []ExecutionAudit {
	j.mu.Lock()
	defer j.mu.Unlock()
	return append([]ExecutionAudit(nil), j.events...)
}

func protectedFixture(t *testing.T, journal ExecutionJournal) (*Access, *Service, *Controller, *notifyingWriter, Target) {
	t.Helper()
	s := NewService(nil)
	w := &notifyingWriter{make(chan string, 20)}
	c, _, err := s.Attach("tab", w)
	if err != nil {
		t.Fatal(err)
	}
	c.Feed(frame(c, 0, "/retained"))
	target := Target{SessionID: "tab", ConnectionID: stateOf(t, s, "tab").ConnectionID}
	a, err := NewAccess(context.Background(), s, AccessScope{Actor: "test-agent", Expires: time.Now().Add(time.Minute), Targets: []AccessTarget{{Target: target, UID: c.Snapshot().UID, ReadFiles: []string{"/var/log/service.log"}, ListDirectories: []string{"/var/log"}}}}, journal)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Cancel(); s.Detach("tab", nil) })
	return a, s, c, w, target
}
func protectedTake(t *testing.T, a *Access, target Target) ControlledTerminal {
	t.Helper()
	run, err := a.Operations().Take(context.Background(), TakeRequest{Target: target, ConfirmedOrdinaryShell: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = run.Release() })
	return run
}

func TestAccessDoesNotBootstrapUnknownForegroundAfterYield(t *testing.T) {
	a, s, c, w, target := protectedFixture(t, &memoryJournal{})
	if err := c.HumanWrite([]byte("vim\n")); err != nil {
		t.Fatal(err)
	}
	nextWrite(t, w)
	if _, err := a.Operations().Take(context.Background(), TakeRequest{Target: target, ConfirmedOrdinaryShell: true}); !errors.Is(err, ErrNeedConfirmation) {
		t.Fatal(err)
	}
	select {
	case command := <-w.writes:
		t.Fatalf("injected into unknown foreground: %q", command)
	default:
	}
	if stateOf(t, s, target.SessionID).Owned {
		t.Fatal("failed reacquire retained ownership")
	}
}
func requireReview(t *testing.T, run ControlledTerminal, ctx context.Context, command string) string {
	t.Helper()
	result, err := run.Execute(ctx, CommandRequest{Command: command})
	var review *ApprovalRequired
	if !errors.As(err, &review) || result.Submission != NotSubmitted || result.Completion != NotStarted {
		t.Fatal(result, err)
	}
	return review.ID
}

func TestAccessScopeAndLiteralReadOnly(t *testing.T) {
	j := &memoryJournal{}
	a, s, c, w, target := protectedFixture(t, j)
	wrong := target
	wrong.ConnectionID = "old"
	if _, err := a.Operations().Take(context.Background(), TakeRequest{Target: wrong, ConfirmedOrdinaryShell: true}); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
	run := protectedTake(t, a, target)
	done := executeReply(run, context.Background(), CommandRequest{Command: "tail -n 20 /var/log/service.log"})
	nextWrite(t, w)
	commandPrompt(c, 1, 7, "/retained", "sensitive output not for audit")
	reply := awaitReply(t, done)
	if reply.err != nil || reply.result.ExitCode == nil || *reply.result.ExitCode != 7 || !stateOf(t, s, "tab").Owned {
		t.Fatal(reply)
	}
	events := j.snapshot()
	if len(events) != 2 || events[0].Phase != "dispatch_intent" || events[1].Submission != Submitted || events[1].Completion != Completed || events[0].OperationID != events[1].OperationID {
		t.Fatal(events)
	}
	data, _ := json.Marshal(events)
	if strings.Contains(string(data), "service.log") || strings.Contains(string(data), "sensitive output") || strings.Contains(string(data), "/retained") {
		t.Fatal(string(data))
	}
	a.Cancel()
	if _, err := run.Inspect(ReadOptions{}); err == nil {
		t.Fatal("expired scope retained output access")
	}
}

func TestAccessApprovalReleasesAndExecutesExactCommandOnce(t *testing.T) {
	j := &memoryJournal{}
	a, s, c, w, target := protectedFixture(t, j)
	run := protectedTake(t, a, target)
	id := requireReview(t, run, context.Background(), "printf 'test-password-DO-NOT-LOG'")
	if stateOf(t, s, "tab").Owned {
		t.Fatal("approval retained ownership")
	}
	details, err := a.Review(id)
	if err != nil || details.Command != "printf 'test-password-DO-NOT-LOG'" || details.Target != target {
		t.Fatal(details, err)
	}
	approved, err := a.Approve(id, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Approve(id, true); !errors.Is(err, ErrStaleApproval) {
		t.Fatal(err)
	}
	done := make(chan commandReply, 1)
	go func() { r, e := approved.Execute(context.Background()); done <- commandReply{r, e} }()
	if wire := nextWrite(t, w); !strings.Contains(wire, details.Command) {
		t.Fatal(wire)
	}
	commandPrompt(c, 1, 0, "/retained", "test-password-DO-NOT-LOG")
	if reply := awaitReply(t, done); reply.err != nil || reply.result.Completion != Completed || reply.result.RunID == run.ID() {
		t.Fatal(reply)
	}
	if _, err := approved.Execute(context.Background()); !errors.Is(err, ErrStaleApproval) {
		t.Fatal(err)
	}
	if stateOf(t, s, "tab").Owned {
		t.Fatal("approved one-shot retained ownership")
	}
	events := j.snapshot()
	if len(events) != 4 || events[0].Phase != "approval_required" || events[1].Phase != "approval_granted" || events[2].Reason != "explicit_approval" || events[2].ApprovalID != id || events[3].ApprovalID != id {
		t.Fatal(events)
	}
	data, _ := json.Marshal(events)
	if strings.Contains(string(data), "test-password") {
		t.Fatal(string(data))
	}
}

func TestAccessStaleDeniedStoppedAndCanceledApprovalsCannotDispatch(t *testing.T) {
	for _, mode := range []string{"denied", "stop", "caller_cancel", "access_cancel", "changed_prompt", "unknown_prompt", "identity", "replacement", "intervening_run", "expiry"} {
		t.Run(mode, func(t *testing.T) {
			a, s, c, w, target := protectedFixture(t, &memoryJournal{})
			run := protectedTake(t, a, target)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			id := requireReview(t, run, ctx, "restart-business-service")
			approved, err := a.Approve(id, mode != "denied")
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
				_ = s.Stop(target.SessionID, run.ID())
			case "caller_cancel":
				cancel()
			case "access_cancel":
				a.Cancel()
			case "changed_prompt":
				c.Feed(frame(c, 0, "/different"))
			case "unknown_prompt":
				if err := c.HumanWrite([]byte("")); err != nil {
					t.Fatal(err)
				}
				nextWrite(t, w)
			case "identity":
				c.mu.Lock()
				c.state.UID++
				c.mu.Unlock()
			case "replacement":
				_, _, _ = s.AttachConnection("tab", "replacement", &notifyingWriter{make(chan string, 10)})
			case "intervening_run":
				fresh := takeReady(t, s, c, "tab")
				_ = fresh.Release()
			case "expiry":
				a.mu.Lock()
				a.reviews[id].details.Expires = time.Now().Add(-time.Second)
				a.mu.Unlock()
			}
			if result, err := approved.Execute(context.Background()); err == nil || result.Submission == Submitted {
				t.Fatal(result, err)
			}
			select {
			case wire := <-w.writes:
				t.Fatalf("stale approval dispatched/probed: %q", wire)
			default:
			}
		})
	}
}

func TestAccessRechecksAfterWriteAheadAudit(t *testing.T) {
	for _, mode := range []string{"stop", "prompt", "uid", "audit_fail"} {
		t.Run(mode, func(t *testing.T) {
			j := &memoryJournal{}
			a, s, c, w, target := protectedFixture(t, j)
			run := protectedTake(t, a, target)
			j.hook = func(event ExecutionAudit) error {
				if event.Phase != "dispatch_intent" {
					return nil
				}
				switch mode {
				case "stop":
					_ = s.Stop(target.SessionID, run.ID())
				case "prompt":
					c.Feed(frame(c, 0, "/changed"))
				case "uid":
					c.mu.Lock()
					c.state.UID++
					c.mu.Unlock()
				case "audit_fail":
					return errors.New("disk full")
				}
				return nil
			}
			result, err := run.Execute(context.Background(), CommandRequest{Command: "pwd"})
			if err == nil || result.Submission != NotSubmitted || stateOf(t, s, "tab").Owned {
				t.Fatal(result, err)
			}
			select {
			case wire := <-w.writes:
				t.Fatal(wire)
			default:
			}
		})
	}
}

func TestAccessResultAuditFailureDoesNotEraseExecutionEvidence(t *testing.T) {
	j := &memoryJournal{hook: func(e ExecutionAudit) error {
		if e.Phase == "result" {
			return errors.New("disk full")
		}
		return nil
	}}
	a, s, c, w, target := protectedFixture(t, j)
	run := protectedTake(t, a, target)
	done := executeReply(run, context.Background(), CommandRequest{Command: "pwd"})
	nextWrite(t, w)
	commandPrompt(c, 1, 0, "/retained", "")
	reply := awaitReply(t, done)
	if !errors.Is(reply.err, ErrExecutionAudit) || reply.result.Completion != Completed || reply.result.Submission != Submitted || reply.result.ExitCode == nil || stateOf(t, s, "tab").Owned {
		t.Fatal(reply)
	}
}

func TestAccessPolicyNeverTrustsPrefixOrSensitivePaths(t *testing.T) {
	target := AccessTarget{ReadFiles: []string{"/var/log/a.log", "/var/log/.env", "/etc/shadow"}, ListDirectories: []string{"/var/log"}}
	for _, command := range []string{"pwd", "command id -u", "tail -n 20 /var/log/a.log", "cat -- '/var/log/a.log'", "ls -la /var/log"} {
		if !simpleReadOnly(command, target) {
			t.Errorf("expected simple read: %s", command)
		}
	}
	for _, command := range []string{"ls; reboot", "pwd $(id)", "pwd > /tmp/x", "cat /etc/shadow", "cat /var/log/.env", "cat /other/log", "cat /var/log/../log/a.log", "find / -exec rm {} ;", "awk 'BEGIN {system(\"reboot\")}'", "date -s now", "env", "ip link set down", "tail -n 20 /var/log/*", "sh -c pwd", "printf ok", "ls -la", "cat -", "uname `id`", "head -c -1 /var/log/a.log"} {
		if simpleReadOnly(command, target) {
			t.Errorf("unexpected auto approval: %s", command)
		}
	}
}

func TestFileJournalPersistsAndFailsClosedAtLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit", "execution.jsonl")
	j, err := NewFileJournal(path, 1024)
	if err != nil {
		t.Fatal(err)
	}
	event := ExecutionAudit{Time: time.Now().UTC(), Actor: "test", Phase: "dispatch_intent", CommandHash: digest("credential-not-written")}
	if err := j.Append(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(data), "credential-not-written") {
		t.Fatal(string(data), err)
	}
	var read ExecutionAudit
	if err := json.Unmarshal(data, &read); err != nil || read.CommandHash != event.CommandHash {
		t.Fatal(read, err)
	}
	event.Actor = strings.Repeat("x", 1024)
	if err := j.Append(context.Background(), event); err == nil {
		t.Fatal("journal silently exceeded limit")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(data) {
		t.Fatal("failed append changed history")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := j.Append(ctx, ExecutionAudit{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestFileJournalRefusesIncompleteHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	if err := os.WriteFile(path, []byte(`{"phase":"result"`), 0600); err != nil {
		t.Fatal(err)
	}
	j, err := NewFileJournal(path, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Append(context.Background(), ExecutionAudit{Phase: "dispatch_intent"}); err == nil {
		t.Fatal("appended onto torn history")
	}
	data, _ := os.ReadFile(path)
	if string(data) != `{"phase":"result"` {
		t.Fatal(string(data))
	}
}

func TestAccessStoppedBlockedWritePersistsRevocationBeforeResult(t *testing.T) {
	j := &memoryJournal{}
	a, s, c, w, target := protectedFixture(t, j)
	blocked := &blockingWriter{entered: make(chan struct{}), release: make(chan struct{})}
	c.writer = blocked
	run := protectedTake(t, a, target)
	done := executeReply(run, context.Background(), CommandRequest{Command: "pwd"})
	<-blocked.entered
	if err := s.Stop(target.SessionID, run.ID()); err != nil {
		t.Fatal(err)
	}
	if stateOf(t, s, "tab").Owned {
		t.Fatal("audit blocked revocation")
	}
	events := j.snapshot()
	if len(events) != 2 || events[0].Phase != "dispatch_intent" || events[1].Phase != "authority_revoked" {
		t.Fatal(events)
	}
	c.Feed([]byte("POST_HANDOFF_SECRET"))
	close(blocked.release)
	reply := awaitReply(t, done)
	if reply.result.Completion != ResultUnknown || reply.result.ExitCode != nil || strings.Contains(reply.result.Output.Text, "POST_HANDOFF_SECRET") {
		t.Fatal(reply)
	}
	events = j.snapshot()
	if len(events) != 3 || events[2].Completion != ResultUnknown || events[2].Submission != Submitted {
		t.Fatal(events)
	}
	select {
	case wire := <-w.writes:
		t.Fatal(wire)
	default:
	}
}

func TestAccessInspectMustAudit(t *testing.T) {
	j := &memoryJournal{}
	a, _, c, _, target := protectedFixture(t, j)
	if view, err := a.Operations().Inspect(target, ReadOptions{}); err != nil || view.LastPrompt == nil {
		t.Fatal(view, err)
	}
	if events := j.snapshot(); len(events) != 1 || events[0].Phase != "context_read" {
		t.Fatal(events)
	}
	c.Feed([]byte("PRIVATE_OUTPUT"))
	j.hook = func(ExecutionAudit) error { return errors.New("audit unavailable") }
	view, err := a.Operations().Inspect(target, ReadOptions{})
	if !errors.Is(err, ErrExecutionAudit) || view.Output.Text != "" {
		t.Fatal(view, err)
	}
}

func TestAccessCopiesScopeAndRevokesIdleRun(t *testing.T) {
	j := &memoryJournal{}
	_, s, c, _, target := protectedFixture(t, j)
	files := []string{"/var/log/service.log"}
	scope := AccessScope{Actor: "host-issued", Expires: time.Now().Add(time.Minute), Targets: []AccessTarget{{Target: target, UID: c.Snapshot().UID, ReadFiles: files}}}
	a, err := NewAccess(context.Background(), s, scope, j)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Cancel()
	files[0] = "/private/credentials"
	scope.Targets[0].Target.ConnectionID = "replacement"
	scope.Targets[0].UID++
	allowed, err := a.target(target)
	if err != nil || allowed.UID != c.Snapshot().UID || len(allowed.ReadFiles) != 1 || allowed.ReadFiles[0] != "/var/log/service.log" {
		t.Fatal(allowed, err)
	}
	run := protectedTake(t, a, target)
	a.Cancel()
	if state := waitPhase(t, s, "tab", "stopped"); state.Owned {
		t.Fatal(state)
	}
	if result, err := run.Execute(context.Background(), CommandRequest{Command: "pwd"}); err == nil || result.Submission != NotSubmitted {
		t.Fatal(result, err)
	}
}
