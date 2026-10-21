package terminalcontrol

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type notifyingWriter struct{ writes chan string }

func (w *notifyingWriter) Write(p []byte) (int, error) { w.writes <- string(p); return len(p), nil }
func (w *notifyingWriter) Close() error                { return nil }

func nextWrite(t *testing.T, w *notifyingWriter) string {
	t.Helper()
	select {
	case text := <-w.writes:
		return text
	case <-time.After(time.Second):
		t.Fatal("expected terminal write")
		return ""
	}
}

func stateOf(t *testing.T, s *Service, id string) State {
	t.Helper()
	for _, state := range s.States() {
		if state.SessionID == id {
			return state
		}
	}
	t.Fatal("missing endpoint", id)
	return State{}
}

func TestReconnectRejectsOldIdleConfirmationAndStop(t *testing.T) {
	s := NewService(nil)
	old, _, err := s.Attach("tab", &notifyingWriter{writes: make(chan string, 8)})
	if err != nil {
		t.Fatal(err)
	}
	first := stateOf(t, s, "tab")
	run, err := s.StartForConnection(context.Background(), "tab", first.ConnectionID, "slow", true)
	if err != nil {
		t.Fatal(err)
	}
	fresh, _, err := s.Attach("tab", &notifyingWriter{writes: make(chan string, 8)})
	if err != nil {
		t.Fatal(err)
	}
	second := stateOf(t, s, "tab")
	if first.ConnectionID == second.ConnectionID {
		t.Fatal("connection identity reused")
	}
	if _, err := s.StartForConnection(context.Background(), "tab", first.ConnectionID, "inspect", true); !errors.Is(err, ErrNeedConfirmation) {
		t.Fatal(err)
	}
	if _, err := s.StartForConnection(context.Background(), "tab", "", "inspect", true); !errors.Is(err, ErrNeedConfirmation) {
		t.Fatal(err)
	}
	s.Detach("tab", old)
	_ = s.Stop("tab", run.RunID)
	if state := stateOf(t, s, "tab"); state.ConnectionID != second.ConnectionID || state.Phase != "idle" {
		t.Fatal(state)
	}
	if !old.Snapshot().Closed || fresh.Snapshot().Closed {
		t.Fatal("replacement lifecycle incorrect")
	}
	s.Detach("tab", fresh)
}

func waitPhase(t *testing.T, s *Service, id, phase string) State {
	t.Helper()
	deadline := time.After(time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		state := stateOf(t, s, id)
		if state.Phase == phase {
			return state
		}
		select {
		case <-deadline:
			t.Fatalf("phase=%s, want=%s", state.Phase, phase)
			return state
		case <-tick.C:
		}
	}
}

func TestServiceFixedScenariosAndProcessExclusion(t *testing.T) {
	s := NewService(nil)
	w := &notifyingWriter{make(chan string, 10)}
	c, endpoint, err := s.Attach("one", w)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _ = s.Attach("two", &notifyingWriter{make(chan string, 10)})
	defer s.Detach("one", nil)
	defer s.Detach("two", nil)
	if _, err = s.Start(context.Background(), "one", "arbitrary-code", true); err == nil {
		t.Fatal("accepted arbitrary action")
	}
	if _, err = s.Start(context.Background(), "one", "inspect", false); !errors.Is(err, ErrNeedConfirmation) {
		t.Fatal(err)
	}
	c.Feed(frame(c, 0, "/retained"))
	state, err := s.Start(context.Background(), "one", "inspect", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := endpoint.Write([]byte("human\n")); !errors.Is(err, ErrOwned) {
		t.Fatal(err)
	}
	if _, err := s.Start(context.Background(), "two", "inspect", true); err == nil {
		t.Fatal("parallel runs accepted")
	}
	command := nextWrite(t, w)
	if !strings.Contains(command, "command id; command pwd") {
		t.Fatalf("unexpected scenario: %q", command)
	}
	// Ignore old or wrong request markers; they cannot complete this operation.
	c.Feed(frame(c, 0, "/old"))
	c.Feed([]byte("\x1eOCB1|" + c.protocol.nonce + "|999\x1f"))
	c.Feed(frame(c, 0, "/old"))
	if stateOf(t, s, "one").Phase != "running" {
		t.Fatal("completed on stale output")
	}
	c.Feed([]byte("\x1eOCB1|" + c.protocol.nonce + "|1\x1f"))
	c.Feed(frame(c, 0, "/retained"))
	completed := waitPhase(t, s, "one", "completed")
	if completed.Owned || completed.ExitCode == nil || *completed.ExitCode != 0 {
		t.Fatalf("bad completion: %+v", completed)
	}
	if err := s.Stop("one", state.RunID); err != nil {
		t.Fatal(err)
	}
	if _, err := endpoint.Write([]byte("human\n")); err != nil {
		t.Fatal(err)
	}
}

func TestServiceStopReleasesButDoesNotClaimRemoteExit(t *testing.T) {
	s := NewService(nil)
	w := &notifyingWriter{make(chan string, 10)}
	c, endpoint, _ := s.Attach("one", w)
	defer s.Detach("one", nil)
	c.Feed(frame(c, 0, "/"))
	state, err := s.Start(context.Background(), "one", "slow", true)
	if err != nil {
		t.Fatal(err)
	}
	nextWrite(t, w)
	if err := s.Stop("one", state.RunID); err != nil {
		t.Fatal(err)
	}
	stopped := stateOf(t, s, "one")
	if stopped.Owned || stopped.Ready || stopped.Phase != "stopped" {
		t.Fatalf("false remote completion: %+v", stopped)
	}
	if _, err := endpoint.Write([]byte{3}); err != nil {
		t.Fatal(err)
	}
	if nextWrite(t, w) != string([]byte{3}) {
		t.Fatal("unexpected post-stop write")
	}
	c.Feed([]byte("\x1eOCB1|" + c.protocol.nonce + "|1\x1f"))
	c.Feed(frame(c, 130, "/"))
	if !stateOf(t, s, "one").Ready {
		t.Fatal("did not observe remote return")
	}
}

func TestReconnectAndStaleStopsCannotRevokeNewEndpoint(t *testing.T) {
	s := NewService(nil)
	w := &notifyingWriter{make(chan string, 10)}
	c, _, _ := s.Attach("one", w)
	c.Feed(frame(c, 0, "/"))
	old, err := s.Start(context.Background(), "one", "slow", true)
	if err != nil {
		t.Fatal(err)
	}
	nextWrite(t, w)
	w2 := &notifyingWriter{make(chan string, 10)}
	c2, _, _ := s.Attach("one", w2)
	defer s.Detach("one", nil)
	c2.Feed(frame(c2, 0, "/new"))
	newRun, err := s.Start(context.Background(), "one", "slow", true)
	if err != nil {
		t.Fatal(err)
	}
	nextWrite(t, w2)
	s.Detach("one", c)
	if err := s.Stop("one", old.RunID); err == nil {
		t.Fatal("stale stop accepted")
	}
	if !stateOf(t, s, "one").Owned {
		t.Fatal("old endpoint revoked new run")
	}
	if err := s.Stop("one", newRun.RunID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Execute(context.Background(), Lease{}, "ignored"); !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
}

func TestPreparationReservesInputAndStopFencesBootstrap(t *testing.T) {
	s := NewService(nil)
	w := &notifyingWriter{make(chan string, 10)}
	_, endpoint, _ := s.Attach("one", w)
	defer s.Detach("one", nil)
	state, err := s.Start(context.Background(), "one", "inspect", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := endpoint.Write([]byte("human\n")); !errors.Is(err, ErrOwned) {
		t.Fatal(err)
	}
	nextWrite(t, w) // bootstrap only, never the scenario after stop
	if err := s.Stop("one", state.RunID); err != nil {
		t.Fatal(err)
	}
	select {
	case text := <-w.writes:
		t.Fatalf("command escaped stop: %q", text)
	case <-time.After(20 * time.Millisecond):
	}
	if stateOf(t, s, "one").Owned {
		t.Fatal("preparation remained owned")
	}
}

type blockingCloser struct{ *blockingWriter }

func (*blockingCloser) Close() error { return nil }

func TestRunCancellationRevokesEvenWhenTransportWriteIsBlocked(t *testing.T) {
	w := &blockingCloser{&blockingWriter{entered: make(chan struct{}), release: make(chan struct{})}}
	s := NewService(nil)
	c, _, err := s.Attach("one", w)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Detach("one", nil)
	c.Feed(frame(c, 0, "/"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := s.Start(ctx, "one", "slow", true); err != nil {
		t.Fatal(err)
	}
	select {
	case <-w.entered:
	case <-time.After(time.Second):
		t.Fatal("write not committed")
	}
	cancel()
	state := waitPhase(t, s, "one", "stopped")
	if state.Owned || state.Ready {
		t.Fatal("cancellation claimed running transport was complete")
	}
	close(w.release)
}

func TestAuditDoesNotHoldControlLock(t *testing.T) {
	entered := make(chan Audit, 10)
	release := make(chan struct{})
	s := NewService(func(event Audit) {
		entered <- event
		if event.Phase == "accepted" {
			<-release
		}
	})
	w := &notifyingWriter{make(chan string, 10)}
	c, _, _ := s.Attach("one", w)
	defer s.Detach("one", nil)
	c.Feed(frame(c, 0, "/"))
	started := make(chan error, 1)
	go func() { _, err := s.Start(context.Background(), "one", "inspect", true); started <- err }()
	event := <-entered
	stopped := make(chan error, 1)
	go func() { stopped <- s.Stop("one", event.RunID) }()
	var stopErr error
	select {
	case stopErr = <-stopped:
	case <-time.After(time.Second):
		// Unblock the callback even on failure so the test leaves no locked service.
		close(release)
		<-started
		<-stopped
		t.Fatal("a slow audit callback blocked stopping the run")
	}
	close(release)
	if stopErr != nil {
		t.Fatal(stopErr)
	}
	if err := <-started; err != nil {
		t.Fatal(err)
	}
	if stateOf(t, s, "one").Owned {
		t.Fatal("stop did not release ownership")
	}
	select {
	case command := <-w.writes:
		t.Fatalf("command dispatched after stop: %q", command)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestCancellationReleasesOwnershipDuringAcceptedAudit(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	s := NewService(func(event Audit) {
		if event.Phase == "accepted" {
			close(entered)
			<-release
		}
	})
	defer close(release)
	w := &notifyingWriter{make(chan string, 10)}
	c, _, _ := s.Attach("one", w)
	defer s.Detach("one", nil)
	c.Feed(frame(c, 0, "/"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := s.Start(ctx, "one", "inspect", true); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("audit was not called")
	}
	cancel()
	if state := waitPhase(t, s, "one", "stopped"); state.Owned {
		t.Fatal("cancellation waited for logging")
	}
}

func TestAuditMayReadStateAndStopBeforeDispatch(t *testing.T) {
	done := make(chan error, 1)
	var s *Service
	s = NewService(func(event Audit) {
		_ = s.States()
		if event.Phase == "accepted" {
			done <- s.Stop(event.SessionID, event.RunID)
		}
	})
	w := &notifyingWriter{make(chan string, 10)}
	c, _, _ := s.Attach("one", w)
	defer s.Detach("one", nil)
	c.Feed(frame(c, 0, "/"))
	if _, err := s.Start(context.Background(), "one", "inspect", true); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("reentrant audit callback deadlocked")
	}
	if stateOf(t, s, "one").Owned {
		t.Fatal("audit cancellation was lost")
	}
	select {
	case text := <-w.writes:
		t.Fatalf("command escaped cancellation: %q", text)
	case <-time.After(20 * time.Millisecond):
	}
}
