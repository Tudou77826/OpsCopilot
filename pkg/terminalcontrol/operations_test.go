package terminalcontrol

import (
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"
)

func takeReady(t *testing.T, service *Service, c *Controller, id string) ControlledTerminal {
	t.Helper()
	c.Feed(frame(c, 0, "/retained"))
	var operations Operations = service
	run, err := operations.Take(context.Background(), TakeRequest{Target: Target{SessionID: id, ConnectionID: stateOf(t, service, id).ConnectionID}, ConfirmedOrdinaryShell: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = run.Release() })
	return run
}

type commandReply struct {
	result CommandResult
	err    error
}

func executeReply(run ControlledTerminal, ctx context.Context, request CommandRequest) <-chan commandReply {
	done := make(chan commandReply, 1)
	go func() { result, err := run.Execute(ctx, request); done <- commandReply{result, err} }()
	return done
}

func awaitReply(t *testing.T, done <-chan commandReply) commandReply {
	t.Helper()
	select {
	case reply := <-done:
		return reply
	case <-time.After(time.Second):
		t.Fatal("command did not return")
		return commandReply{}
	}
}

func commandPrompt(c *Controller, request int, exit int, dir, output string) {
	c.Feed([]byte("\x1eOCB1|" + c.protocol.nonce + "|" + strconv.Itoa(request) + "\x1f" + output))
	c.Feed(frame(c, exit, dir))
}

func TestOperationsMultipleCommandsKeepOneRunAndReturnEvidence(t *testing.T) {
	s := NewService(nil)
	w := &notifyingWriter{make(chan string, 10)}
	c, endpoint, _ := s.Attach("tab", w)
	defer s.Detach("tab", nil)
	c.Feed([]byte("earlier terminal contents"))
	run := takeReady(t, s, c, "tab")
	if _, err := endpoint.Write([]byte("competing input")); !errors.Is(err, ErrOwned) {
		t.Fatal(err)
	}
	for index, exit := range []int{7, 0} {
		done := executeReply(run, context.Background(), CommandRequest{Command: "command pwd"})
		if wire := nextWrite(t, w); !strings.Contains(wire, "command pwd") {
			t.Fatal(wire)
		}
		commandPrompt(c, index+1, exit, "/retained", "observed result\r\n")
		reply := awaitReply(t, done)
		result := reply.result
		if reply.err != nil || result.Submission != Submitted || result.Completion != Completed || result.ExitCode == nil || *result.ExitCode != exit {
			t.Fatal(result, reply.err)
		}
		if result.OperationID == "" || result.RunID != run.ID() || result.Target != run.Target() {
			t.Fatal(result)
		}
		if result.Output.Text != "observed result\r\n" || result.Context.LastPrompt.PID != 123 || result.Context.LastPrompt.Directory != "/retained" {
			t.Fatal(result)
		}
		if !stateOf(t, s, "tab").Owned {
			t.Fatal("command completion released the entire multi-step run")
		}
		// Returned pointers/snapshots must not be able to rewrite service state.
		*result.ExitCode = 99
		state := stateOf(t, s, "tab")
		if *state.ExitCode != exit {
			t.Fatal("result aliases authoritative state")
		}
		*state.ExitCode = 101
		if *stateOf(t, s, "tab").ExitCode != exit {
			t.Fatal("snapshot aliases authoritative state")
		}
	}
	if err := run.Release(); err != nil {
		t.Fatal(err)
	}
	if stateOf(t, s, "tab").Owned {
		t.Fatal("release retained ownership")
	}
	if _, err := run.Inspect(ReadOptions{}); !errors.Is(err, ErrRevoked) {
		t.Fatal(err)
	}
	if result, err := run.Execute(context.Background(), CommandRequest{Command: "must-not-run"}); !errors.Is(err, ErrRevoked) || result.Submission != NotSubmitted {
		t.Fatal(result, err)
	}
	if _, err := endpoint.Write([]byte("human\n")); err != nil {
		t.Fatal(err)
	}
	if nextWrite(t, w) != "human\n" {
		t.Fatal("handoff did not return the original input stream")
	}
}

func TestOperationsUnknownContextAndStaleTargetsNeverProbe(t *testing.T) {
	s := NewService(nil)
	w := &notifyingWriter{make(chan string, 10)}
	c, _, _ := s.Attach("tab", w)
	defer s.Detach("tab", nil)
	target := Target{SessionID: "tab", ConnectionID: stateOf(t, s, "tab").ConnectionID}
	c.Feed([]byte("zsql> arbitrary prompt text"))
	view, err := s.Inspect(target, ReadOptions{})
	if err != nil || view.Ready || view.LastPrompt != nil || view.Output.Text != "zsql> arbitrary prompt text" {
		t.Fatal(view, err)
	}
	if _, err := s.Take(context.Background(), TakeRequest{Target: target}); !errors.Is(err, ErrNeedConfirmation) {
		t.Fatal(err)
	}
	_, _, _ = s.Attach("tab", &notifyingWriter{make(chan string, 10)})
	if _, err := s.Inspect(target, ReadOptions{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := s.Take(context.Background(), TakeRequest{Target: target, ConfirmedOrdinaryShell: true}); !errors.Is(err, ErrNeedConfirmation) {
		t.Fatal(err)
	}
	select {
	case text := <-w.writes:
		t.Fatalf("unknown/stale context received code: %q", text)
	default:
	}
}

func TestOperationsInspectReportsReservationDuringPreparation(t *testing.T) {
	s := NewService(nil)
	w := &notifyingWriter{make(chan string, 10)}
	c, endpoint, _ := s.Attach("tab", w)
	defer s.Detach("tab", nil)
	target := Target{SessionID: "tab", ConnectionID: stateOf(t, s, "tab").ConnectionID}
	run, _, err := s.beginRun(context.Background(), TakeRequest{Target: target, ConfirmedOrdinaryShell: true})
	if err != nil {
		t.Fatal(err)
	}
	defer run.Release()
	view, err := s.Inspect(target, ReadOptions{})
	if err != nil || !view.Owned || view.Ready || c.Snapshot().Owned {
		t.Fatalf("preparation reservation is not reflected correctly: %+v, %v", view, err)
	}
	if _, err := endpoint.Write([]byte("human")); !errors.Is(err, ErrOwned) {
		t.Fatal(err)
	}
	if err := run.Release(); err != nil {
		t.Fatal(err)
	}
	view, err = s.Inspect(target, ReadOptions{})
	if err != nil || view.Owned {
		t.Fatal(view, err)
	}
	select {
	case text := <-w.writes:
		t.Fatalf("inspection/reservation wrote to an unknown shell: %q", text)
	default:
	}
}

func TestOperationsRejectRawKeysAndInvalidRequestsBeforeWriting(t *testing.T) {
	s := NewService(nil)
	w := &notifyingWriter{make(chan string, 10)}
	c, _, _ := s.Attach("tab", w)
	defer s.Detach("tab", nil)
	run := takeReady(t, s, c, "tab")
	for _, request := range []CommandRequest{
		{Command: ""}, {Command: "  "}, {Command: "first\nsecond"}, {Command: "\x03"}, {Command: "\x1b[A"}, {Command: "a\x00b"}, {Command: strings.Repeat("x", 2049)}, {Command: string([]byte{255})},
		{Command: "id", OutputLimit: OutputCapacity + 1}, {Command: "id", Timeout: -1}, {Command: "id", Timeout: MaxRunDuration + 1},
	} {
		result, err := run.Execute(context.Background(), request)
		if err == nil || result.Submission != NotSubmitted || result.Completion != NotStarted || result.ExitCode != nil {
			t.Fatal(result, err)
		}
	}
	select {
	case text := <-w.writes:
		t.Fatalf("invalid request wrote: %q", text)
	default:
	}
	if !stateOf(t, s, "tab").Owned {
		t.Fatal("invalid arguments unexpectedly ended a usable run")
	}
}

func TestOperationsCancellationAtDispatchCannotSendCommand(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := NewService(func(event Audit) {
		if event.Phase == "dispatch_requested" {
			cancel()
		}
	})
	w := &notifyingWriter{make(chan string, 10)}
	c, _, _ := s.Attach("tab", w)
	defer s.Detach("tab", nil)
	run := takeReady(t, s, c, "tab")
	result, err := run.Execute(ctx, CommandRequest{Command: "must-not-run"})
	if !errors.Is(err, context.Canceled) || result.Submission != NotSubmitted || result.ExitCode != nil {
		t.Fatal(result, err)
	}
	if stateOf(t, s, "tab").Owned {
		t.Fatal("cancellation retained ownership")
	}
	select {
	case text := <-w.writes:
		t.Fatalf("command escaped canceled caller: %q", text)
	default:
	}
}

func TestOperationsStopDuringBlockedWriteReportsUnknownResult(t *testing.T) {
	w := &blockingCloser{&blockingWriter{entered: make(chan struct{}), release: make(chan struct{})}}
	s := NewService(nil)
	c, _, _ := s.Attach("tab", w)
	defer s.Detach("tab", nil)
	run := takeReady(t, s, c, "tab")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := executeReply(run, ctx, CommandRequest{Command: "slow-command"})
	<-w.entered
	c.Feed([]byte("BEFORE_STOP\r\n"))
	cancel()
	if state := waitPhase(t, s, "tab", "stopped"); state.Owned || state.Ready {
		t.Fatal(state)
	}
	commandPrompt(c, 1, 0, "/private-after-takeover", "AFTER_TAKEOVER_SECRET\r\n")
	close(w.release)
	reply := awaitReply(t, done)
	if !errors.Is(reply.err, context.Canceled) || reply.result.Completion != ResultUnknown || reply.result.Submission != Submitted || reply.result.ExitCode != nil {
		t.Fatal(reply)
	}
	if reply.result.Output.Text != "BEFORE_STOP\r\n" || reply.result.Context == nil || reply.result.Context.LastPrompt.Directory != "/retained" {
		t.Fatalf("old operation collected post-takeover context/output: %+v", reply.result)
	}
}

func TestOperationsConcurrentExecuteAndFixedTestsShareExclusion(t *testing.T) {
	s := NewService(nil)
	w := &notifyingWriter{make(chan string, 10)}
	c, _, _ := s.Attach("tab", w)
	defer s.Detach("tab", nil)
	_, _, _ = s.Attach("other", &notifyingWriter{make(chan string, 10)})
	defer s.Detach("other", nil)
	run := takeReady(t, s, c, "tab")
	done := executeReply(run, context.Background(), CommandRequest{Command: "first"})
	nextWrite(t, w)
	if result, err := run.Execute(context.Background(), CommandRequest{Command: "second"}); !errors.Is(err, ErrBusy) || result.Submission != NotSubmitted {
		t.Fatal(result, err)
	}
	if _, err := s.Start(context.Background(), "other", "inspect", true); err == nil {
		t.Fatal("fixed scenario bypassed process-wide exclusion")
	}
	commandPrompt(c, 1, 0, "/retained", "")
	if reply := awaitReply(t, done); reply.err != nil {
		t.Fatal(reply.err)
	}
}

type failedOperationWriter struct{}

func (failedOperationWriter) Write([]byte) (int, error) { return 1, io.ErrClosedPipe }
func (failedOperationWriter) Close() error              { return nil }

func TestOperationsPartialSubmissionNeverClaimsExitOrRetries(t *testing.T) {
	s := NewService(nil)
	c, _, _ := s.Attach("tab", failedOperationWriter{})
	defer s.Detach("tab", nil)
	run := takeReady(t, s, c, "tab")
	result, err := run.Execute(context.Background(), CommandRequest{Command: "id"})
	if !errors.Is(err, io.ErrClosedPipe) || result.Submission != SubmissionUnknown || result.Completion != ResultUnknown || result.ExitCode != nil {
		t.Fatal(result, err)
	}
	if stateOf(t, s, "tab").Owned {
		t.Fatal("failed submission retained ownership")
	}
}

func TestOperationsOldRunCannotReadOrReleaseReplacement(t *testing.T) {
	s := NewService(nil)
	c, _, _ := s.Attach("tab", &notifyingWriter{make(chan string, 10)})
	old := takeReady(t, s, c, "tab")
	c2, _, _ := s.Attach("tab", &notifyingWriter{make(chan string, 10)})
	defer s.Detach("tab", nil)
	fresh := takeReady(t, s, c2, "tab")
	if _, err := old.Inspect(ReadOptions{}); !errors.Is(err, ErrRevoked) {
		t.Fatal(err)
	}
	if err := old.Release(); !errors.Is(err, ErrRevoked) {
		t.Fatal(err)
	}
	if result, err := old.Execute(context.Background(), CommandRequest{Command: "stale"}); !errors.Is(err, ErrRevoked) || result.Submission != NotSubmitted {
		t.Fatal(result, err)
	}
	if stateOf(t, s, "tab").RunID != fresh.ID() || !stateOf(t, s, "tab").Owned {
		t.Fatal("old run changed replacement ownership")
	}
}

func TestOperationsRunDeadlineReleasesWithoutFurtherCalls(t *testing.T) {
	s := NewService(nil)
	c, _, _ := s.Attach("tab", &notifyingWriter{make(chan string, 10)})
	defer s.Detach("tab", nil)
	c.Feed(frame(c, 0, "/"))
	target := Target{SessionID: "tab", ConnectionID: stateOf(t, s, "tab").ConnectionID}
	run, err := s.Take(context.Background(), TakeRequest{Target: target, ConfirmedOrdinaryShell: true, Timeout: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if state := waitPhase(t, s, "tab", "stopped"); state.Owned {
		t.Fatal(state)
	}
	if _, err := run.Inspect(ReadOptions{}); !errors.Is(err, ErrRevoked) {
		t.Fatal(err)
	}
}
