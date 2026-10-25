package filecontrol

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"opscopilot/pkg/filetransfer"
)

type memoryJournal struct {
	mu     sync.Mutex
	events []Event
	fail   bool
}

func (j *memoryJournal) Append(ctx context.Context, e Event) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.fail {
		return errors.New("audit unavailable")
	}
	j.events = append(j.events, e)
	return ctx.Err()
}

type fakeBackend struct {
	checkErr error
	started  chan struct{}
	finish   chan struct{}
	lists    int
}

func (b *fakeBackend) Check(ctx context.Context, e Endpoint) error {
	if b.checkErr != nil {
		return b.checkErr
	}
	return ctx.Err()
}
func (b *fakeBackend) List(ctx context.Context, e Endpoint, p string, local bool) ([]filetransfer.Entry, error) {
	b.lists++
	return []filetransfer.Entry{{Name: "example.log", Path: p + "/example.log"}}, nil
}
func (b *fakeBackend) Transfer(ctx context.Context, e Endpoint, task Task, emit func(filetransfer.Progress)) (filetransfer.TransferResult, error) {
	emit(filetransfer.Progress{BytesDone: 4, BytesTotal: 10})
	close(b.started)
	select {
	case <-ctx.Done():
		return filetransfer.TransferResult{}, ctx.Err()
	case <-b.finish:
		return filetransfer.TransferResult{Bytes: 10}, nil
	}
}
func fileFixture(t *testing.T) (*Service, *fakeBackend, Workspace) {
	t.Helper()
	b := &fakeBackend{started: make(chan struct{}), finish: make(chan struct{})}
	s := New(b, &memoryJournal{})
	w, err := s.Open(context.Background(), Endpoint{SessionID: "ssh", ConnectionID: "physical-1", Identity: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	return s, b, w
}
func acknowledge(t *testing.T, s *Service, id string, fn func() error) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()
	deadline := time.After(time.Second)
	for {
		w, _ := s.Snapshot(id)
		if w.Pending != "" {
			if err := s.Acknowledge(id, w.Pending, w.Endpoint.ConnectionID); err != nil {
				t.Fatal(err)
			}
			break
		}
		select {
		case <-deadline:
			t.Fatal("missing presentation")
		case <-time.After(time.Millisecond):
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
func TestPresentationRequiresExactReceiptAndCancelledReceiptCannotReplay(t *testing.T) {
	s, _, w := fileFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Present(ctx, w.ID) }()
	var nonce string
	for i := 0; i < 100; i++ {
		v, _ := s.Snapshot(w.ID)
		nonce = v.Pending
		if nonce != "" {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if nonce == "" {
		t.Fatal("no nonce")
	}
	if s.Acknowledge(w.ID, nonce, "another-connection") == nil {
		t.Fatal("wrong connection accepted")
	}
	cancel()
	if <-done == nil {
		t.Fatal("cancel ignored")
	}
	if s.Acknowledge(w.ID, nonce, w.Endpoint.ConnectionID) == nil {
		t.Fatal("late receipt accepted")
	}
	acknowledge(t, s, w.ID, func() error { return s.Present(context.Background(), w.ID) })
}
func TestReleaseCancelsDrainsAndKeepsActualResult(t *testing.T) {
	s, b, w := fileFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Take(ctx, w.ID, "turn"); err != nil {
		t.Fatal(err)
	}
	task, err := s.Start(ctx, w.ID, "turn", Task{Direction: "download", LocalPath: "destination", RemotePath: "source"})
	if err != nil {
		t.Fatal(err)
	}
	<-b.started
	if err = s.Cancel(w.ID, "foreign-turn", task.ID); !errors.Is(err, ErrStale) {
		t.Fatal("foreign task cancellation", err)
	}
	s.Release(w.ID, "turn")
	after, _ := s.Snapshot(w.ID)
	if after.OwnerTurn != "" || after.Tasks[0].Status != "cancelled" {
		t.Fatalf("not drained: %+v", after)
	}
	if after.Tasks[0].Bytes != 4 {
		t.Fatal("lost partial progress")
	}
	if _, err = s.Start(ctx, w.ID, "turn", Task{Direction: "upload"}); !errors.Is(err, ErrStale) {
		t.Fatal("late dispatch", err)
	}
	after.Tasks[0].Status = "modified"
	fresh, _ := s.Snapshot(w.ID)
	if fresh.Tasks[0].Status != "cancelled" {
		t.Fatal("snapshot aliases state")
	}
}
func TestCompletedTransferStaysDoneAfterRelease(t *testing.T) {
	s, b, w := fileFixture(t)
	ctx := context.Background()
	if err := s.Take(ctx, w.ID, "turn"); err != nil {
		t.Fatal(err)
	}
	task, err := s.Start(ctx, w.ID, "turn", Task{Direction: "upload", LocalPath: "local", RemotePath: "remote"})
	if err != nil {
		t.Fatal(err)
	}
	<-b.started
	close(b.finish)
	for i := 0; i < 100; i++ {
		after, _ := s.Snapshot(w.ID)
		if after.Tasks[0].Status == "done" {
			break
		}
		time.Sleep(time.Millisecond)
	}
	s.Release(w.ID, "turn")
	after, _ := s.Snapshot(w.ID)
	if after.Tasks[0].ID != task.ID || after.Tasks[0].Status != "done" || after.Tasks[0].Bytes != 10 {
		t.Fatalf("bad completion %+v", after.Tasks)
	}
}
func TestBrowseUsesAuditOwnershipAndPresentation(t *testing.T) {
	s, b, w := fileFixture(t)
	ctx := context.Background()
	if _, err := s.Browse(ctx, w.ID, "turn", "/logs", false); !errors.Is(err, ErrStale) {
		t.Fatal("unowned browse", err)
	}
	if err := s.Take(ctx, w.ID, "turn"); err != nil {
		t.Fatal(err)
	}
	acknowledge(t, s, w.ID, func() error { _, err := s.Browse(ctx, w.ID, "turn", "/logs", false); return err })
	after, _ := s.Snapshot(w.ID)
	if after.RemotePath != "/logs" || len(after.RemoteEntries) != 1 {
		t.Fatal("directory not presented")
	}
	j := s.Journal.(*memoryJournal)
	j.mu.Lock()
	j.fail = true
	j.mu.Unlock()
	if _, err := s.Browse(ctx, w.ID, "turn", "/secret", false); err == nil {
		t.Fatal("audit failure allowed browse")
	}
	if b.lists != 1 {
		t.Fatal("dispatched despite audit failure")
	}
	s.Release(w.ID, "turn")
}
func TestChangedConnectionCannotTransferAndDismissCannotBypassOwner(t *testing.T) {
	s, b, w := fileFixture(t)
	if err := s.Take(context.Background(), w.ID, "turn"); err != nil {
		t.Fatal(err)
	}
	if s.Dismiss(w.ID) == nil {
		t.Fatal("dismiss bypassed owner")
	}
	b.checkErr = errors.New("connection replaced")
	if _, err := s.Start(context.Background(), w.ID, "turn", Task{Direction: "upload"}); err == nil {
		t.Fatal("changed identity accepted")
	}
	s.Release(w.ID, "turn")
	if err := s.Dismiss(w.ID); err != nil {
		t.Fatal(err)
	}
}

func TestMissingResultAuditBlocksSubsequentAutomaticOperations(t *testing.T) {
	s, b, w := fileFixture(t)
	ctx := context.Background()
	if err := s.Take(ctx, w.ID, "turn"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Start(ctx, w.ID, "turn", Task{Direction: "upload", LocalPath: "local", RemotePath: "remote"}); err != nil {
		t.Fatal(err)
	}
	<-b.started
	j := s.Journal.(*memoryJournal)
	j.mu.Lock()
	j.fail = true
	j.mu.Unlock()
	close(b.finish)
	for i := 0; i < 100; i++ {
		after, _ := s.Snapshot(w.ID)
		if after.Error != "" {
			break
		}
		time.Sleep(time.Millisecond)
	}
	after, _ := s.Snapshot(w.ID)
	if after.Error == "" || after.Tasks[0].Status != "failed" || !after.Tasks[0].Transferred {
		t.Fatal("audit failure was hidden")
	}
	j.mu.Lock()
	j.fail = false
	j.mu.Unlock()
	if _, err := s.Start(ctx, w.ID, "turn", Task{Direction: "upload"}); err == nil {
		t.Fatal("automatic operation resumed after an unrecorded result")
	}
	s.Release(w.ID, "turn")
}
