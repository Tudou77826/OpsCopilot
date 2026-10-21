package session

import (
	"errors"
	"testing"
	"time"
)

func TestLifecycleRejectsDuplicateAndLateCallbacks(t *testing.T) {
	var l Lifecycle
	old, err := l.Begin("tab")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Begin("tab"); !errors.Is(err, ErrConnectingOrActive) {
		t.Fatal(err)
	}
	l.Close("tab", nil)
	next, err := l.Begin("tab")
	if err != nil {
		t.Fatal(err)
	}
	defer next.Finish(nil)
	if old.Apply(func() { t.Fatal("old publication accepted") }) {
		t.Fatal("closed lifetime accepted")
	}
	if old.Finish(func() { t.Fatal("old cleanup accepted") }) {
		t.Fatal("closed lifetime finished twice")
	}
	if !next.Apply(nil) {
		t.Fatal("old callback invalidated replacement")
	}
}

func TestLifecycleCloseWaitsForPublicationNotOtherTabs(t *testing.T) {
	var l Lifecycle
	v, _ := l.Begin("one")
	entered, release, applied, closed := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() { v.Apply(func() { close(entered); <-release }); close(applied) }()
	<-entered
	go func() { l.Close("one", nil); close(closed) }()
	other, err := l.Begin("two")
	if err != nil || !other.Apply(nil) {
		t.Fatal("other tab blocked")
	}
	other.Finish(nil)
	select {
	case <-closed:
		t.Fatal("close overtook publication")
	default:
	}
	close(release)
	for _, done := range []<-chan struct{}{applied, closed} {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("lifecycle deadlock")
		}
	}
	if v.Apply(nil) {
		t.Fatal("close left publication enabled")
	}
	if len(l.current) != 0 {
		t.Fatal("closed lifetimes leaked in registry")
	}
}

func TestLifecycleFinishKeepsReservationThroughCleanup(t *testing.T) {
	var l Lifecycle
	v, _ := l.Begin("one")
	v.Finish(func() {
		if _, err := l.Begin("one"); !errors.Is(err, ErrConnectingOrActive) {
			t.Fatal("reconnect admitted during cleanup")
		}
	})
	next, err := l.Begin("one")
	if err != nil {
		t.Fatal(err)
	}
	next.Finish(nil)
	l.Close("one", nil)
	if len(l.current) != 0 {
		t.Fatal("closing absent tab leaked reservation")
	}
}
