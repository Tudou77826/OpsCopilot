package filecontrol

import (
	"context"
	"testing"
)

func TestRevealFileTaskOnlyPresentsRecordedTaskEvenWithDisconnectedChannel(t *testing.T) {
	s, b, w := fileFixture(t)
	// A retained finished task does not require a live remote endpoint to view.
	s.mu.Lock()
	s.items[w.ID].jobs["done"] = &job{task: Task{ID: "done", Status: "done"}}
	s.mu.Unlock()
	b.checkErr = ErrStale
	acknowledge(t, s, w.ID, func() error { return s.Reveal(context.Background(), w.ID, "done") })
	got, _ := s.Snapshot(w.ID)
	if got.OwnerTurn != "" || got.RevealTask != "done" || b.lists != 0 {
		t.Fatal("reveal accessed remote channel", got)
	}
	if s.Reveal(context.Background(), w.ID, "missing") == nil {
		t.Fatal("missing task accepted")
	}
	if err := s.Dismiss(w.ID); err != nil {
		t.Fatal(err)
	}
	if s.Reveal(context.Background(), w.ID, "done") == nil {
		t.Fatal("dismissed workspace reopened")
	}
}
