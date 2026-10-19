package connectiondiag

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func TestTraceRedactsBeforeTruncationAndFreezes(t *testing.T) {
	ctx, trace := New(context.Background())
	secret := "secret words & value"
	AddSecrets(ctx, secret)
	Event(ctx, "connection.failure", "password=%q encoded=%s jdbc:vendor://someone:other-password@host/db?token=private-token %s", secret, url.QueryEscape(secret), strings.Repeat("a", 6000)+secret)
	// Credentials discovered after earlier events are also removed at rendering.
	Event(ctx, "late", "saved credential late-secret")
	AddSecrets(ctx, "late-secret")
	report := trace.Finish()
	for _, forbidden := range []string{secret, url.QueryEscape(secret), "someone", "other-password", "private-token", "late-secret"} {
		if strings.Contains(report, forbidden) {
			t.Fatalf("credential leaked: %s", forbidden)
		}
	}
	Event(ctx, "after.finish", "must not appear")
	if report != trace.Finish() || !trace.Has("connection.failure") || trace.Has("after.finish") {
		t.Fatal("attempt was not frozen")
	}
}

func TestTraceConcurrentAttemptsAndBounds(t *testing.T) {
	ctx, trace := New(context.Background())
	otherCtx, other := New(context.Background())
	var workers sync.WaitGroup
	for i := 0; i < 6; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			for n := 0; n < 100; n++ {
				Event(ctx, "first", "%s", strings.Repeat(fmt.Sprint(i), 2000))
			}
		}(i)
	}
	Event(otherCtx, "second", "isolated")
	workers.Wait()
	if report := trace.Finish(); len(report) > MaxReportBytes || !strings.Contains(report, "日志上限") || strings.Contains(report, "isolated") {
		t.Fatal("unbounded or mixed trace")
	}
	if other.Has("first") || !other.Has("second") {
		t.Fatal("attempts share evidence")
	}
}
