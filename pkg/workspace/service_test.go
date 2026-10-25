package workspace

import (
	"context"
	"errors"
	"testing"
	"time"
)

func pendingPresentation(t *testing.T, s *Service, client string, view Snapshot) Presentation {
	t.Helper()
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		commands, err := s.Pull(client, view)
		if err != nil {
			t.Fatal(err)
		}
		if len(commands) > 0 {
			return commands[0]
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("presentation not requested")
	return Presentation{}
}
func TestPresentationRequiresExactMountedAndSelectedConnection(t *testing.T) {
	s := &Service{}
	client := s.Attach()
	tab := Tab{Target: Target{TabID: "tab", ConnectionID: "actual"}, Title: "test", Status: "connected"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Present(ctx, client, "open", tab) }()
	command := pendingPresentation(t, s, client, Snapshot{})
	for _, view := range []Snapshot{
		{Tabs: []Tab{tab}, Active: tab.Target},
		{Tabs: []Tab{{Target: tab.Target, Status: "connected", Ready: true}}},
		{Tabs: []Tab{{Target: Target{TabID: "tab", ConnectionID: "replacement"}, Ready: true, Status: "connected"}}, Active: tab.Target},
	} {
		_, _ = s.Pull(client, view)
		if s.Acknowledge(client, command.ID, true) == nil {
			t.Fatal("accepted incomplete/changed presentation")
		}
	}
	tab.Ready = true
	_, _ = s.Pull(client, Snapshot{Tabs: []Tab{tab}, Active: tab.Target})
	if err := s.Acknowledge(client, command.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := s.Acknowledge(client, command.ID, true); !errors.Is(err, ErrStale) {
		t.Fatal("duplicate receipt accepted", err)
	}
}
func TestCancelledAndReplacedPagesCannotAcknowledge(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "replace"}[replace], func(t *testing.T) {
			s := &Service{}
			client := s.Attach()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				done <- s.Present(ctx, client, "select", Tab{Target: Target{TabID: "tab", ConnectionID: "conn"}})
			}()
			command := pendingPresentation(t, s, client, Snapshot{})
			if replace {
				s.Attach()
			} else {
				cancel()
			}
			if err := <-done; err == nil {
				t.Fatal("canceled presentation succeeded")
			}
			if s.Acknowledge(client, command.ID, true) == nil {
				t.Fatal("late receipt accepted")
			}
			if replace {
				if _, err := s.Pull(client, Snapshot{}); !errors.Is(err, ErrStale) {
					t.Fatal(err)
				}
				if s.Detach(client) {
					t.Fatal("old page detached replacement")
				}
			}
		})
	}
}
func TestSnapshotIsCopiedAndStaleUIIsUnavailable(t *testing.T) {
	s := &Service{}
	client := s.Attach()
	tabs := []Tab{{Target: Target{TabID: "one"}}}
	_, _ = s.Pull(client, Snapshot{Tabs: tabs})
	tabs[0].Title = "mutated"
	_, view, _ := s.Current()
	if view.Tabs[0].Title != "" {
		t.Fatal("caller mutated snapshot")
	}
	view.Tabs[0].Title = "also mutated"
	_, view, _ = s.Current()
	if view.Tabs[0].Title != "" {
		t.Fatal("reader mutated snapshot")
	}
	s.seen = time.Now().Add(-6 * time.Second)
	if _, _, err := s.Current(); !errors.Is(err, ErrUnavailable) {
		t.Fatal("stale UI accepted")
	}
}

func TestRegisteredObserverCannotStealSenderPresentation(t *testing.T) {
	s := &Service{}
	sender, _, err := s.RegisterPage("sender-tab")
	if err != nil {
		t.Fatal(err)
	}
	tab := Tab{Target: Target{TabID: "sender-ssh", ConnectionID: "sender-physical"}, Title: "sender node", Status: "connected", Ready: true}
	_, _ = s.Pull(sender, Snapshot{Tabs: []Tab{tab}, Active: tab.Target})
	if err = s.Activate(sender); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Present(ctx, sender, "select", tab) }()
	command := pendingPresentation(t, s, sender, Snapshot{Tabs: []Tab{tab}, Active: tab.Target})
	observer, replaced, err := s.RegisterPage("other-window")
	if err != nil || replaced {
		t.Fatal("observer replaced sender", err)
	}
	commands, err := s.Pull(observer, Snapshot{Tabs: []Tab{{Target: Target{TabID: "other-ssh", ConnectionID: "other-physical"}}}})
	if err != nil || len(commands) != 0 {
		t.Fatal("observer received sender operation", err)
	}
	if s.Acknowledge(observer, command.ID, true) == nil {
		t.Fatal("observer acknowledged sender operation")
	}
	client, view, err := s.Current()
	if err != nil || client != sender || view.Active != tab.Target {
		t.Fatal("observer changed sender context")
	}
	if err = s.Acknowledge(sender, command.ID, true); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if err = s.Activate(observer); err != nil {
		t.Fatal(err)
	}
	if err = s.Registered(sender); err != nil {
		t.Fatal("sender cannot submit another message", err)
	}
	if err = s.Activate(sender); err != nil {
		t.Fatal(err)
	}
}

func TestReloadReplacesOnlyItsOwnPageRegistration(t *testing.T) {
	s := &Service{}
	sender, _, _ := s.RegisterPage("sender")
	other, _, _ := s.RegisterPage("other")
	_, _ = s.Pull(sender, Snapshot{})
	_ = s.Activate(sender)
	next, replaced, err := s.RegisterPage("sender")
	if err != nil || !replaced {
		t.Fatal("reload did not replace active sender")
	}
	if s.Registered(sender) == nil {
		t.Fatal("old document registration survived reload")
	}
	if err = s.Registered(other); err != nil {
		t.Fatal("reload invalidated unrelated window")
	}
	active, _, err := s.Current()
	if err != nil || active != next {
		t.Fatal("reload did not retain correct page")
	}
}
