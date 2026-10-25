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

func TestFactsAreFilteredIsolatedBoundedAndFrozen(t *testing.T) {
	ctx, trace := New(context.Background())
	Fact(ctx, "driver", "password=private-password late-secret\nsecond line")
	AddSecrets(ctx, "late-secret")
	if value := trace.Facts()["driver"]; strings.Contains(value, "private-password") || strings.Contains(value, "late-secret") || strings.Contains(value, "\n") {
		t.Fatal("comparison field leaked or contains a new line", value)
	}
	copy := trace.Facts()
	copy["driver"] = "changed"
	if trace.Facts()["driver"] == "changed" {
		t.Fatal("facts snapshot aliases the attempt")
	}
	for i := 0; i < 100; i++ {
		Fact(ctx, fmt.Sprint(i), strings.Repeat("界", 2000))
	}
	if len(trace.Facts()) > 48 {
		t.Fatal("unbounded facts")
	}
	for _, value := range trace.Facts() {
		if len(value) > 1024 {
			t.Fatal("unbounded comparison value")
		}
	}
	trace.Finish()
	Fact(ctx, "driver", "late update")
	if trace.Facts()["driver"] == "late update" {
		t.Fatal("frozen attempt changed")
	}
}
