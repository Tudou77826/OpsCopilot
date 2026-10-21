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
