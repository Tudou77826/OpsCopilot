package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"opscopilot/pkg/remote"
	"opscopilot/pkg/session"
)

// In-memory endpoints only; these tests never dial a real server.
type lifecycleConn struct {
	closed       atomic.Int32
	started      atomic.Int32
	shellEntered chan struct{}
	shellRelease chan struct{}
}

func (c *lifecycleConn) StartShell(int, int) (io.WriteCloser, io.Reader, error) {
	c.started.Add(1)
	if c.shellEntered != nil {
		close(c.shellEntered)
		<-c.shellRelease
	}
	return &handoffTestWriter{}, strings.NewReader(""), nil
}
func (*lifecycleConn) Resize(int, int) error                       { return nil }
func (*lifecycleConn) Run(context.Context, string) (string, error) { return "", nil }
func (*lifecycleConn) Healthy() bool                               { return true }
func (*lifecycleConn) Protocol() string                            { return remote.ProtocolSSH }
func (c *lifecycleConn) Close() error                              { c.closed.Add(1); return nil }

func newLifecycleApp() *App {
	return &App{ctx: context.Background(), sessionMgr: session.NewManager()}
}

func awaitLifecycle(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("lifecycle operation did not return")
	}
}

func TestTerminalLifecycleCloseDuringDialAndShell(t *testing.T) {
	for _, phase := range []string{"dial", "shell"} {
		t.Run(phase, func(t *testing.T) {
			a := newLifecycleApp()
			entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			conn := &lifecycleConn{}
			if phase == "shell" {
				conn.shellEntered, conn.shellRelease = entered, release
			}
			dial := func(*remote.ConnectConfig) (remote.Connection, error) {
				if phase == "dial" {
					close(entered)
					<-release
				}
				return conn, nil
			}
			var result ConnectResult
			go func() { result = a.connectTerminal(ConnectConfig{}, "tab", dial, nil); close(done) }()
			<-entered
			duplicate := a.connectTerminal(ConnectConfig{}, "tab", func(*remote.ConnectConfig) (remote.Connection, error) {
				t.Error("duplicate reconnect dialed another connection")
				return nil, errors.New("must not dial")
			}, nil)
			if duplicate.Success {
				t.Fatal("duplicate reconnect accepted")
			}
			a.CloseSession("tab")
			if phase == "shell" && conn.closed.Load() != 1 {
				t.Fatal("closing tab did not close transport while shell start was blocked")
			}
			close(release)
			awaitLifecycle(t, done)
			if result.Success || conn.closed.Load() != 1 {
				t.Fatalf("late connection resurrected: %+v close=%d", result, conn.closed.Load())
			}
			if phase == "dial" && conn.started.Load() != 0 {
				t.Fatal("canceled dial still started shell")
			}
			if _, ok := a.sessionMgr.Get("tab"); ok || len(a.TerminalHandoffStates()) != 0 || len(a.sessionStates) != 0 {
				t.Fatal("closed tab retained state")
			}
		})
	}
}

func TestTerminalLifecycleStaleReconnectDoesNotDial(t *testing.T) {
	a := newLifecycleApp()
	state := a.storeSessionState("tab", ConnectConfig{})
	a.updateSessionState("tab", "disconnected", "eof")
	a.CloseSession("tab")
	result := a.connectTerminal(ConnectConfig{}, "tab", func(*remote.ConnectConfig) (remote.Connection, error) {
		t.Fatal("closed session dialed from stale reconnect config")
		return nil, nil
	}, state)
	if result.Success {
		t.Fatal("stale reconnect accepted")
	}
}

func installLifecycleTerminal(t *testing.T, a *App) *terminalConnection {
	t.Helper()
	life, err := a.terminalLifecycles.Begin("tab")
	if err != nil {
		t.Fatal(err)
	}
	target, err := a.installTerminal(life, "tab", ConnectConfig{}, &lifecycleConn{}, &handoffTestWriter{})
	if err != nil {
		t.Fatal(err)
	}
	return target
}

type lateTerminalReader struct {
	entered   chan struct{}
	release   chan struct{}
	reads     int
	aggregate bool
}

func (r *lateTerminalReader) Read(p []byte) (int, error) {
	r.reads++
	if r.aggregate && r.reads == 1 {
		for i := range p {
			p[i] = 'x'
		}
		return len(p), nil
	}
	close(r.entered)
	<-r.release
	return copy(p, "OLD OUTPUT"), io.EOF
}

func TestTerminalLifecycleOldReaderCannotAffectReplacement(t *testing.T) {
	for _, aggregate := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary-read", true: "aggregated-read"}[aggregate], func(t *testing.T) {
			a := newLifecycleApp()
			old := installLifecycleTerminal(t, a)
			old.gate.markReady()
			reader := &lateTerminalReader{entered: make(chan struct{}), release: make(chan struct{}), aggregate: aggregate}
			done := make(chan struct{})
			var events atomic.Int32
			go func() { a.readTerminalSession(old, reader, func(string, interface{}) { events.Add(1) }); close(done) }()
			<-reader.entered
			a.CloseSession("tab")
			fresh := installLifecycleTerminal(t, a)
			defer a.CloseSession("tab")
			close(reader.release)
			awaitLifecycle(t, done)
			if events.Load() != 0 {
				t.Fatal("old output/disconnect reached UI")
			}
			current, ok := a.sessionMgr.Get("tab")
			if !ok || current != fresh.registered || fresh.handoff.Snapshot().Closed {
				t.Fatal("old reader removed replacement")
			}
			if a.sessionStates["tab"].Status != "active" || a.commandExtractors["tab"] != fresh.extractor {
				t.Fatal("old cleanup overwrote replacement state")
			}
		})
	}
}

func TestTerminalLifecycleEOFReleasesBeforeReconnect(t *testing.T) {
	a := newLifecycleApp()
	old := installLifecycleTerminal(t, a)
	var events []string
	old.gate.markReady()
	a.readTerminalSession(old, strings.NewReader("last output"), func(name string, _ interface{}) { events = append(events, name) })
	if _, ok := a.sessionMgr.Get("tab"); ok {
		t.Fatal("EOF retained transport")
	}
	if a.sessionStates["tab"].Status != "disconnected" || !old.handoff.Snapshot().Closed {
		t.Fatal("EOF retained active state or control")
	}
	if len(events) != 3 || events[2] != "session-disconnected" {
		t.Fatal(events)
	}
	fresh := installLifecycleTerminal(t, a)
	defer a.CloseSession("tab")
	if fresh.registered == old.registered || !fresh.lifetime.Apply(nil) {
		t.Fatal("reconnect did not create independent lifetime")
	}
	if old.lifetime.Finish(func() { t.Fatal("late disconnect callback ran") }) {
		t.Fatal("old lifetime still active")
	}
}

func TestTerminalLifecycleOldBufferedOutputIsDiscarded(t *testing.T) {
	a := newLifecycleApp()
	old := installLifecycleTerminal(t, a)
	old.gate.deliver("old banner", func(string) { t.Fatal("unready output should buffer") })
	a.readTerminalSession(old, strings.NewReader(""), func(string, interface{}) {})
	installLifecycleTerminal(t, a)
	defer a.CloseSession("tab")
	old.gate.markReadyAndEmit(func(string) { t.Fatal("old buffered output flushed into replacement") })
	old.gate.deliver("late output", func(string) { t.Fatal("closed gate emitted output") })
}

type blockedLifecycleWriter struct{ entered, release chan struct{} }

func (w *blockedLifecycleWriter) Write([]byte) (int, error) {
	close(w.entered)
	<-w.release
	return 0, io.ErrClosedPipe
}
func (*blockedLifecycleWriter) Close() error { return nil }

func TestTerminalLifecycleOldWriteErrorCannotReachReplacement(t *testing.T) {
	a := newLifecycleApp()
	life, _ := a.terminalLifecycles.Begin("tab")
	w := &blockedLifecycleWriter{entered: make(chan struct{}), release: make(chan struct{})}
	if _, err := a.installTerminal(life, "tab", ConnectConfig{}, &lifecycleConn{}, w); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var events atomic.Int32
	go func() { a.writeTerminal("tab", "old input", func(string, interface{}) { events.Add(1) }); close(done) }()
	<-w.entered
	a.CloseSession("tab")
	fresh := installLifecycleTerminal(t, a)
	defer a.CloseSession("tab")
	close(w.release)
	awaitLifecycle(t, done)
	if events.Load() != 0 {
		t.Fatal("old write error reached replacement UI")
	}
	if a.commandExtractors["tab"] != fresh.extractor {
		t.Fatal("old input affected replacement extractor")
	}
}
