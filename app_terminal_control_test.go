package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"opscopilot/pkg/config"
	"opscopilot/pkg/session"
	"opscopilot/pkg/terminalcontrol"
)

type handoffTestWriter struct {
	mu     sync.Mutex
	writes []string
}

func TestTerminalObserverFactoryBindsConnectionWithoutInputOrIdentity(t *testing.T) {
	a := newLifecycleApp()
	a.configMgr = config.NewManagerWithDir(t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.ctx = ctx
	life, err := a.terminalLifecycles.Begin("observer")
	if err != nil {
		t.Fatal(err)
	}
	writer := &handoffTestWriter{}
	connection, err := a.installTerminal(life, "observer", ConnectConfig{Host: "fixture-only", User: "lab"}, &lifecycleConn{}, writer)
	if err != nil {
		t.Fatal(err)
	}
	defer a.CloseSession("observer")
	target := terminalcontrol.Target{SessionID: "observer", ConnectionID: connection.connectionID}
	observer, err := a.newTerminalObserver(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	view, err := observer.Inspect(terminalcontrol.ReadOptions{Limit: 100})
	if err != nil || view.Owned || view.LastPrompt != nil {
		t.Fatal(view, err)
	}
	writer.mu.Lock()
	writes := len(writer.writes)
	writer.mu.Unlock()
	if writes != 0 {
		t.Fatal("screen observer sent terminal input")
	}
	target.ConnectionID = "stale"
	if _, err = a.newTerminalObserver(ctx, target); !errors.Is(err, terminalcontrol.ErrUnavailable) {
		t.Fatal(err)
	}
	cancel()
	if _, err = observer.Inspect(terminalcontrol.ReadOptions{}); !errors.Is(err, terminalcontrol.ErrDenied) {
		t.Fatal(err)
	}
}

func TestTerminalAccessFactoryBindsLiveConnectionAndAppLifetime(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := newLifecycleApp()
	a.ctx, a.configMgr = ctx, config.NewManagerWithDir(t.TempDir())
	life, err := a.terminalLifecycles.Begin("protected")
	if err != nil {
		t.Fatal(err)
	}
	connection, err := a.installTerminal(life, "protected", ConnectConfig{Host: "fixture-only", User: "lab"}, &lifecycleConn{}, &handoffTestWriter{})
	if err != nil {
		t.Fatal(err)
	}
	defer a.CloseSession("protected")
	target := terminalcontrol.Target{SessionID: "protected", ConnectionID: connection.connectionID}
	scope := terminalcontrol.AccessScope{Actor: "test-agent", Expires: time.Now().Add(time.Minute), Targets: []terminalcontrol.AccessTarget{{Target: target, UID: 0}}}
	access, err := a.newTerminalAccess(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	defer access.Cancel()
	// The normal portable launch uses a relative config directory. Resolving an
	// audit path must not require changing the user's working directory.
	a.configMgr = config.NewManagerWithDir(".")
	portable, err := a.newTerminalAccess(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	defer portable.Cancel()
	scope.Targets[0].Target.ConnectionID = "stale"
	if _, err := a.newTerminalAccess(ctx, scope); !errors.Is(err, terminalcontrol.ErrUnavailable) {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-access.Done():
	case <-time.After(time.Second):
		t.Fatal("app shutdown did not invalidate issued scope")
	}
}

func (w *handoffTestWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writes = append(w.writes, string(p))
	return len(p), nil
}
func (w *handoffTestWriter) Close() error { return nil }

func TestTerminalHandoffGatesSharedInputEndpoints(t *testing.T) {
	a := &App{ctx: context.Background(), sessionMgr: session.NewManager()}
	raw := &handoffTestWriter{}
	_, gated, err := a.terminalControlService().Attach("controlled", raw)
	if err != nil {
		t.Fatal(err)
	}
	a.sessionMgr.AddWithID("controlled", nil, gated)
	a.sessionMgr.AddWithID("manual", nil, &handoffTestWriter{})
	run, err := a.StartTerminalHandoffTest("controlled", "inspect", true, a.TerminalHandoffStates()[0].ConnectionID)
	if err != nil {
		t.Fatal(err)
	}
	defer a.CloseSession("controlled")
	if err := a.SendCommand("controlled", "must-not-run\n"); !errors.Is(err, terminalcontrol.ErrOwned) {
		t.Fatal(err)
	}
	accepted := a.sessionMgr.BroadcastAccepted([]string{"controlled", "manual"}, "broadcast\n")
	if len(accepted) != 1 || accepted[0] != "manual" {
		t.Fatalf("broadcast bypassed input ownership: %v", accepted)
	}
	if err := a.StopTerminalHandoffTest("controlled", run.RunID); err != nil {
		t.Fatal(err)
	}
	for _, state := range a.TerminalHandoffStates() {
		if state.SessionID == "controlled" && state.Owned {
			t.Fatal("stop retained ownership")
		}
	}
	a.CloseSession("controlled")
	if len(a.TerminalHandoffStates()) != 0 {
		t.Fatal("close retained control endpoint")
	}
}
