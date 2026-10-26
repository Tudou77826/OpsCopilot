package opssession

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCloseDeadlineAlsoBoundsMetadataLockWait(t *testing.T) {
	s, _ := newTestStore(t)
	mustInit(t, s, "closing")
	release, err := s.lock("closing")
	if err != nil {
		t.Fatal(err)
	}
	released := make(chan struct{})
	go func() { time.Sleep(400 * time.Millisecond); release(); close(released) }()
	t.Cleanup(func() { <-released })
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	start := time.Now()
	err = s.Close(ctx, "closing")
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 200*time.Millisecond {
		t.Fatalf("close ignored cancellation while waiting for metadata: elapsed=%v err=%v", time.Since(start), err)
	}
}

func TestRunFinishCannotBeHeldByWatcherMetadataLock(t *testing.T) {
	s, _ := newTestStore(t)
	mustInit(t, s, "running")
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, finish, err := s.Begin(parent, "running")
	if err != nil {
		t.Fatal(err)
	}
	release, err := s.lock("running")
	if err != nil {
		t.Fatal(err)
	}
	released := make(chan struct{})
	go func() { time.Sleep(700 * time.Millisecond); release(); close(released) }()
	t.Cleanup(func() { <-released })
	// The watcher has attempted to inspect metadata before cancellation.
	time.Sleep(150 * time.Millisecond)
	cancel()
	start := time.Now()
	finish()
	if time.Since(start) > 200*time.Millisecond {
		t.Fatalf("watcher delayed cancellation cleanup: %v", time.Since(start))
	}
}
