package terminalcontrol

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func frame(c *Controller, rc int, dir string) []byte {
	return []byte(fmt.Sprintf("\x1eOCH1|%s|%d|1000|123|%s\x1f", c.protocol.nonce, rc, hex.EncodeToString([]byte(dir))))
}

func readyController(t *testing.T, w io.Writer) *Controller {
	t.Helper()
	c, err := New(w)
	if err != nil {
		t.Fatal(err)
	}
	c.Feed(frame(c, 0, "/fixture"))
	return c
}

func TestFrameFragmentationAndValidation(t *testing.T) {
	c := readyController(t, io.Discard)
	data := frame(c, 7, "/tmp/a | b\n中文")
	before := c.Snapshot().Sequence
	for _, b := range data {
		if out := c.Feed([]byte{b}); len(out) != 0 {
			t.Fatalf("control frame leaked: %q", out)
		}
	}
	s := c.Snapshot()
	if s.Sequence != before+1 || s.ExitCode != 7 || s.Directory != "/tmp/a | b\n中文" {
		t.Fatalf("bad snapshot: %+v", s)
	}
	for _, bad := range []string{
		"\x1eOCH1|wrong|0|1000|123|2f\x1f",
		fmt.Sprintf("\x1eOCH1|%s|999|0|1|2f\x1f", c.protocol.nonce),
		fmt.Sprintf("\x1eOCH1|%s|0|0|1|zz\x1f", c.protocol.nonce),
		"ordinary $ prompt > text",
	} {
		before = c.Snapshot().Sequence
		if got := c.Feed([]byte(bad)); string(got) != bad {
			t.Fatalf("lost display data: %q", got)
		}
		if c.Snapshot().Sequence != before {
			t.Fatal("invalid data asserted shell readiness")
		}
	}
	if len(c.Feed(append([]byte{0x1e}, bytes.Repeat([]byte{'x'}, maxFrameBytes+10)...))) == 0 {
		t.Fatal("oversized frame was retained")
	}
	if len(c.protocol.buffer) > maxFrameBytes {
		t.Fatal("unbounded parser buffer")
	}
	ordinary := "\x1e{\"event\":\"ordinary-log-record\"}\n"
	if got := c.Feed([]byte(ordinary)); string(got) != ordinary {
		t.Fatalf("unrelated record output was delayed: %q", got)
	}
}

func TestUnknownTerminalNeverReceivesBootstrap(t *testing.T) {
	var written bytes.Buffer
	c, err := New(&written)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Acquire(); !errors.Is(err, ErrNeedConfirmation) {
		t.Fatal(err)
	}
	if _, err := c.Bootstrap(context.Background(), false); !errors.Is(err, ErrNeedConfirmation) {
		t.Fatal(err)
	}
	if written.Len() != 0 {
		t.Fatal("sent code into unknown foreground program")
	}
}

func TestCancelledBootstrapDoesNotWrite(t *testing.T) {
	var written bytes.Buffer
	c, err := New(&written)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Bootstrap(ctx, true); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if written.Len() != 0 || c.Snapshot().Ready {
		t.Fatal("cancelled bootstrap changed terminal state")
	}
}

func TestPartialHumanInputCannotBeOverriddenByLatePrompt(t *testing.T) {
	var written bytes.Buffer
	c := readyController(t, &written)
	if err := c.HumanWrite([]byte("printf unfinished")); err != nil {
		t.Fatal(err)
	}
	c.Feed(frame(c, 0, "/fixture"))
	if c.Snapshot().Ready {
		t.Fatal("late prompt authorized a partially typed line")
	}
	if _, err := c.Acquire(); !errors.Is(err, ErrNeedConfirmation) {
		t.Fatal(err)
	}
	before := written.Len()
	if _, err := c.Bootstrap(context.Background(), true); !errors.Is(err, ErrNeedConfirmation) {
		t.Fatal(err)
	}
	if written.Len() != before {
		t.Fatal("bootstrap appended code to human's unfinished line")
	}
	if err := c.HumanWrite([]byte{3}); err != nil {
		t.Fatal(err)
	}
	c.Feed(frame(c, 130, "/fixture"))
	if _, err := c.Acquire(); err != nil {
		t.Fatal(err)
	}
}

func TestTerminalStatusRepliesDoNotBecomePartialHumanInput(t *testing.T) {
	var written bytes.Buffer
	c := readyController(t, &written)
	if err := c.HumanWrite([]byte("\x1b[4;6R")); err != nil {
		t.Fatal(err)
	}
	l, err := c.Acquire()
	if err != nil {
		t.Fatal("CPR dirtied an idle shell", err)
	}
	for _, reply := range []string{"\x1b[4;6R", "\x1b[?1;2c", "\x1b[>0;1;0c", "\x1b[0n"} {
		if err := c.HumanWrite([]byte(reply)); err != nil {
			t.Fatal(err)
		}
	}
	for _, input := range []string{"\x1b[A", "\x1b[4;6Rtouch bad\n", "\x1b[4;6R\n", "\x1b[4;", "text"} {
		if err := c.HumanWrite([]byte(input)); !errors.Is(err, ErrOwned) {
			t.Fatalf("exempted human/mixed input: %q", input)
		}
	}
	if _, err := c.Execute(context.Background(), Lease{}, "not-owned"); !errors.Is(err, ErrRevoked) {
		t.Fatal(err)
	}
	c.Revoke()
	if _, err := c.Execute(context.Background(), l, "stale"); !errors.Is(err, ErrRevoked) {
		t.Fatal(err)
	}
}

func TestCommandWaitsForActualCompletionFrame(t *testing.T) {
	w := &blockingWriter{entered: make(chan struct{}), release: make(chan struct{})}
	close(w.release)
	c := readyController(t, w)
	l, _ := c.Acquire()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := c.Execute(ctx, l, "false"); done <- err }()
	select {
	case <-w.entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	c.Feed([]byte("ordinary $ prompt > text\n"))
	c.Feed(frame(c, 0, "/late-prompt"))
	select {
	case err := <-done:
		t.Fatalf("finished on display text: %v", err)
	default:
	}
	c.Feed([]byte(fmt.Sprintf("\x1eOCB1|%s|1\x1f", c.protocol.nonce)))
	c.Feed(frame(c, 1, "/fixture"))
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if c.Snapshot().ExitCode != 1 {
		t.Fatal("lost failed exit status")
	}
}

func TestDisconnectWakesPendingExecution(t *testing.T) {
	w := &blockingWriter{entered: make(chan struct{}), release: make(chan struct{})}
	close(w.release)
	c := readyController(t, w)
	l, _ := c.Acquire()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := c.Execute(ctx, l, "sleep 20"); done <- err }()
	select {
	case <-w.entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	c.Closed()
	select {
	case err := <-done:
		if !errors.Is(err, io.EOF) {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if c.Snapshot().Owned {
		t.Fatal("disconnect retained ownership")
	}
}

func TestTimeoutRevokesWithoutClaimingRemoteStopped(t *testing.T) {
	c := readyController(t, io.Discard)
	l, _ := c.Acquire()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := c.Execute(ctx, l, "sleep 20"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	s := c.Snapshot()
	if s.Owned || s.Ready {
		t.Fatalf("timeout claimed idle/completed: %+v", s)
	}
	if _, err := c.Execute(context.Background(), l, "next"); !errors.Is(err, ErrRevoked) {
		t.Fatal(err)
	}
}

func TestOwnershipAndStoppedLease(t *testing.T) {
	var written bytes.Buffer
	c := readyController(t, &written)
	l, err := c.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	if err := c.HumanWrite([]byte("human\n")); !errors.Is(err, ErrOwned) {
		t.Fatal(err)
	}
	c.Revoke()
	if _, err := c.Execute(context.Background(), l, "touch must-not-run"); !errors.Is(err, ErrRevoked) {
		t.Fatal(err)
	}
	if written.Len() != 0 {
		t.Fatal("stale command reached terminal")
	}
	if err := c.HumanWrite([]byte("human\n")); err != nil {
		t.Fatal(err)
	}
	if c.Snapshot().Ready {
		t.Fatal("human input left stale readiness")
	}
}

func TestLeaseCannotOperateAnotherTerminal(t *testing.T) {
	var aWritten, bWritten bytes.Buffer
	a := readyController(t, &aWritten)
	b := readyController(t, &bWritten)
	aLease, _ := a.Acquire()
	_, _ = b.Acquire()
	if _, err := b.Execute(context.Background(), aLease, "must-not-cross-tabs"); !errors.Is(err, ErrRevoked) {
		t.Fatal(err)
	}
	if aWritten.Len() != 0 || bWritten.Len() != 0 {
		t.Fatal("a different terminal accepted this lease")
	}
}

func TestInvalidAndAlreadyCancelledCommandsDoNotWrite(t *testing.T) {
	var written bytes.Buffer
	c := readyController(t, &written)
	l, _ := c.Acquire()
	for _, command := range []string{"", "one\ntwo", "one\rtwo", "bad\x00", strings.Repeat("x", 2049)} {
		if _, err := c.Execute(context.Background(), l, command); err == nil {
			t.Fatal("accepted invalid command")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Execute(ctx, l, "must-not-run"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if written.Len() != 0 {
		t.Fatal("invalid/cancelled input written")
	}
}

type blockingWriter struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	mu      sync.Mutex
	writes  []string
}

func (w *blockingWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.entered) })
	<-w.release
	w.mu.Lock()
	w.writes = append(w.writes, string(p))
	w.mu.Unlock()
	return len(p), nil
}

func TestRevocationUnderTransportBackpressure(t *testing.T) {
	w := &blockingWriter{entered: make(chan struct{}), release: make(chan struct{})}
	c := readyController(t, w)
	l, _ := c.Acquire()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	first := make(chan error, 1)
	go func() { _, err := c.Execute(ctx, l, "first"); first <- err }()
	select {
	case <-w.entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	second := make(chan error, 1)
	go func() { _, err := c.Execute(ctx, l, "second"); second <- err }()
	// Revocation must not wait on the writer. The first command has already
	// committed; the queued second command must fail its post-lock lease check.
	c.Revoke()
	if c.Snapshot().Owned {
		t.Fatal("stop retained ownership")
	}
	close(w.release)
	for _, done := range []chan error{first, second} {
		select {
		case err := <-done:
			if !errors.Is(err, ErrRevoked) {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.writes) != 1 || !strings.HasSuffix(w.writes[0], "first\n") {
		t.Fatalf("queued command escaped revocation: %q", w.writes)
	}
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) / 2, nil }

func TestPartialWriteRevokesAndLeavesStateUnknown(t *testing.T) {
	c := readyController(t, shortWriter{})
	l, _ := c.Acquire()
	if _, err := c.Execute(context.Background(), l, "printf test"); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal(err)
	}
	s := c.Snapshot()
	if s.Owned || s.Ready {
		t.Fatalf("partial command considered complete: %+v", s)
	}
}
