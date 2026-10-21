package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"opscopilot/pkg/connectionstore"
	"opscopilot/pkg/remote"
	"opscopilot/pkg/terminalcontrol"
)

func TestWorkspaceConnectionInventoryNeverSerializesCredentials(t *testing.T) {
	nodes := []*connectionstore.Node{{ID: "saved", Name: "db-node", Config: &remote.ConnectConfig{Host: "55", Port: 22, User: "user", Password: "secret-password", RootPassword: "root-secret", Bastion: &remote.ConnectConfig{Host: "50", Port: 22, Password: "jump-secret"}}}}
	public, configs := savedWorkspaceConnections(nodes)
	data, err := json.Marshal(public)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"secret-password", "root-secret", "jump-secret", "password", "rootPassword"} {
		if strings.Contains(string(data), secret) {
			t.Fatal("inventory leaked credentials", secret)
		}
	}
	if len(public) != 1 || len(public[0].Route) != 2 || public[0].Route[0].Host != "50" || configs["saved"].Password != "secret-password" {
		t.Fatal("route/host credentials not preserved independently")
	}
}
func TestWorkspaceCancelledDialCannotPublishLateConnection(t *testing.T) {
	a := newLifecycleApp()
	ctx, cancel := context.WithCancel(context.Background())
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	conn := &lifecycleConn{}
	var result ConnectResult
	go func() {
		result = a.connectTerminalContext(ctx, ConnectConfig{}, "new", func(*remote.ConnectConfig) (remote.Connection, error) { close(entered); <-release; return conn, nil }, nil)
		close(done)
	}()
	<-entered
	cancel()
	// Wait for the cancellation fence, independently of the blocked dial.
	for deadline := time.Now().Add(time.Second); a.terminalLifecycles.ApplyCurrent("new", nil); {
		if time.Now().After(deadline) {
			t.Fatal("cancellation did not fence pending dial")
		}
		time.Sleep(time.Millisecond)
	}
	close(release)
	awaitLifecycle(t, done)
	if result.Success || conn.closed.Load() != 1 || conn.started.Load() != 0 {
		t.Fatalf("late dial became a resource: %+v", result)
	}
	if _, ok := a.sessionMgr.Get("new"); ok {
		t.Fatal("canceled resource published")
	}
}
func TestWorkspaceStaleCleanupNeverClosesReplacement(t *testing.T) {
	a := newLifecycleApp()
	old := installLifecycleTerminal(t, a)
	a.CloseSession("tab")
	next := installLifecycleTerminal(t, a)
	if a.closeSession("tab", old.lifetime, "") {
		t.Fatal("old cancellation closed replacement")
	}
	if a.closeSession("tab", nil, old.connectionID) {
		t.Fatal("stale discard closed replacement")
	}
	if _, err := a.terminalControlService().Inspect(terminalcontrol.Target{SessionID: "tab", ConnectionID: next.connectionID}, terminalcontrol.ReadOptions{Limit: 1}); err != nil {
		t.Fatal(err)
	}
	a.CloseSession("tab")
}
