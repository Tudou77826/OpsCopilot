package assistanttools

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/terminalcontrol"
	"opscopilot/pkg/workspace"
)

func TestWorkspaceSwitchRevokesOldConsentAndCapturedCalls(t *testing.T) {
	c := &assistantcontrol.Coordinator{}
	turn, _ := c.Begin(context.Background(), assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	factory := func(target terminalcontrol.Target) *TerminalTools { return &TerminalTools{Turn: turn, Target: target} }
	w := NewWorkspaceTools(turn, terminalcontrol.Target{SessionID: "old", ConnectionID: "old-conn"}, nil, nil, func(context.Context, string) (workspace.Tab, error) {
		return workspace.Tab{Target: workspace.Target{TabID: "new", ConnectionID: "new-conn"}}, nil
	}, factory)
	defer w.Close()
	w.current.readAllowed = true
	w.current.denied = true
	old := w.CaptureCall()
	if _, err := w.Call(turn.Context(), "workspace_select", "new"); err != nil {
		t.Fatal(err)
	}
	if w.current.readAllowed || w.current.denied || w.current.Target.SessionID != "new" || turn.Snapshot().Target.TabID != "new" {
		t.Fatal("new target inherited authorization")
	}
	if _, err := old(turn.Context(), "terminal_take", "", ""); !errors.Is(err, assistantcontrol.ErrStale) {
		t.Fatal("old target generation inherited new target", err)
	}
}

func TestWorkspaceCreatesSavedSSHWithoutAnOpenTerminal(t *testing.T) {
	for _, selection := range []string{"reject", "allow"} {
		t.Run(selection, func(t *testing.T) {
			const id = "31dde072-a161-4d5f-9ab5-047644be052d"
			c := &assistantcontrol.Coordinator{}
			turn, err := c.Begin(context.Background(), assistantcontrol.Target{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer turn.Finish("completed", "")
			w := NewWorkspaceTools(turn, terminalcontrol.Target{}, nil, func(_ context.Context, got string) (ConnectionPlan, error) {
				if got != id {
					return ConnectionPlan{}, errors.New("unexpected saved profile")
				}
				return ConnectionPlan{Connection: workspace.Connection{ID: got, Name: "New SSH"}}, nil
			}, nil, func(terminalcontrol.Target) *TerminalTools { return nil })
			defer w.Close()
			done := make(chan error, 1)
			go func() {
				value, err := w.Call(turn.Context(), "workspace_create", `{"host":"test.example","user":"ops"}`)
				if err == nil && value.(map[string]any)["connection_id"] != id {
					err = errors.New("missing saved profile ID")
				}
				done <- err
			}()
			prompt := waitPrompt(t, c)
			if prompt.Prompt.Kind != "ssh_connection" {
				t.Fatal(prompt.Prompt.Kind)
			}
			answer := assistantcontrol.Answer{Selected: []string{selection}}
			if selection == "allow" {
				answer.Text = id
			}
			if err := c.Respond(turn.ID(), prompt.Prompt.ID, answer); err != nil {
				t.Fatal(err)
			}
			err = <-done
			if selection == "allow" && err != nil || selection == "reject" && !errors.Is(err, assistantcontrol.ErrActionDenied) {
				t.Fatal(err)
			}
		})
	}
}
func TestWorkspaceOpenNeedsExactConfirmationAndStopPreventsDial(t *testing.T) {
	for _, answer := range []string{"reject", "stop", "allow"} {
		t.Run(answer, func(t *testing.T) {
			c := &assistantcontrol.Coordinator{}
			turn, _ := c.Begin(context.Background(), assistantcontrol.Target{}, nil)
			defer turn.Finish("completed", "")
			var opened atomic.Int32
			w := NewWorkspaceTools(turn, terminalcontrol.Target{}, nil, func(context.Context, string) (ConnectionPlan, error) {
				return ConnectionPlan{Connection: workspace.Connection{ID: "saved", Name: "safe", Route: []workspace.Endpoint{{Host: "host", Port: 22, User: "user"}}}, Open: func(context.Context) (workspace.Tab, error) {
					opened.Add(1)
					return workspace.Tab{Target: workspace.Target{TabID: "new", ConnectionID: "new-conn"}}, nil
				}}, nil
			}, nil, func(target terminalcontrol.Target) *TerminalTools { return &TerminalTools{Turn: turn, Target: target} })
			defer w.Close()
			done := make(chan error, 1)
			go func() { _, err := w.Call(turn.Context(), "workspace_open", "saved"); done <- err }()
			prompt := waitPrompt(t, c)
			if opened.Load() != 0 {
				t.Fatal("dial before approval")
			}
			if answer == "stop" {
				_ = c.Stop(turn.ID())
			} else {
				if err := c.Respond(turn.ID(), prompt.Prompt.ID, assistantcontrol.Answer{Selected: []string{answer}}); err != nil {
					t.Fatal(err)
				}
			}
			err := <-done
			if answer == "allow" {
				if err != nil || opened.Load() != 1 {
					t.Fatal(err, opened.Load())
				}
			} else if err == nil || opened.Load() != 0 {
				t.Fatal("denied/stopped open dialed", err)
			}
		})
	}
}

func TestWorkspaceReselectCannotResetRefusedPermission(t *testing.T) {
	c := &assistantcontrol.Coordinator{}
	turn, _ := c.Begin(context.Background(), assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	w := NewWorkspaceTools(turn, terminalcontrol.Target{SessionID: "old", ConnectionID: "old-conn"}, nil, nil, func(_ context.Context, id string) (workspace.Tab, error) {
		return workspace.Tab{Target: workspace.Target{TabID: id, ConnectionID: id + "-conn"}}, nil
	}, func(target terminalcontrol.Target) *TerminalTools { return &TerminalTools{Turn: turn, Target: target} })
	defer w.Close()
	w.current.readDenied = true
	w.current.denied = true
	for _, id := range []string{"new", "old"} {
		if _, err := w.Call(turn.Context(), "workspace_select", id); err != nil {
			t.Fatal(err)
		}
	}
	if !w.current.readDenied || !w.current.denied || w.current.readAllowed || w.current.access != nil {
		t.Fatal("reselection reset refusal or inherited authority")
	}
}
