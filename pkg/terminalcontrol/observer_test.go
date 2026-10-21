package terminalcontrol

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestObserverReadsUnknownForegroundWithoutWritingOrOwning(t *testing.T) {
	service := NewService(nil)
	writer := &notifyingWriter{make(chan string, 10)}
	controller, _, err := service.AttachConnection("tab", "connection", writer)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Detach("tab", nil)
	controller.Feed([]byte("visible screen, secret stays out of audit"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	journal := &memoryJournal{}
	observer, err := NewObserver(ctx, service, ObservationScope{Actor: "test", Target: Target{"tab", "connection"}, Expires: time.Now().Add(time.Minute)}, journal)
	if err != nil {
		t.Fatal(err)
	}
	view, err := observer.Inspect(ReadOptions{Limit: 1000})
	if err != nil || view.Ready || view.Owned || view.LastPrompt != nil || !strings.Contains(view.Output.Text, "visible screen") {
		t.Fatal(view, err)
	}
	select {
	case write := <-writer.writes:
		t.Fatalf("passive read wrote %q", write)
	default:
	}
	if err = controller.HumanWrite([]byte("human input")); err != nil {
		t.Fatal("read blocked the user", err)
	}
	if write := nextWrite(t, writer); write != "human input" {
		t.Fatal(write)
	}
	events := journal.snapshot()
	if len(events) != 1 || events[0].Phase != "screen_read" || events[0].UID != -1 {
		t.Fatal(events)
	}
	data, _ := json.Marshal(events)
	if strings.Contains(string(data), "secret") {
		t.Fatal("audit retained screen content")
	}
	cancel()
	if _, err = observer.Inspect(ReadOptions{}); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
}
func TestObserverRefusesStaleConnectionAndAuditFailure(t *testing.T) {
	service := NewService(nil)
	writer := &notifyingWriter{make(chan string, 10)}
	_, _, err := service.AttachConnection("tab", "actual", writer)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Detach("tab", nil)
	scope := ObservationScope{Actor: "test", Target: Target{"tab", "stale"}, Expires: time.Now().Add(time.Minute)}
	observer, err := NewObserver(context.Background(), service, scope, &memoryJournal{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = observer.Inspect(ReadOptions{}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	scope.Target.ConnectionID = "actual"
	observer, err = NewObserver(context.Background(), service, scope, &memoryJournal{hook: func(ExecutionAudit) error { return errors.New("journal unavailable") }})
	if err != nil {
		t.Fatal(err)
	}
	if view, err := observer.Inspect(ReadOptions{}); !errors.Is(err, ErrExecutionAudit) || view.Target.SessionID != "" {
		t.Fatal(view, err)
	}
}
