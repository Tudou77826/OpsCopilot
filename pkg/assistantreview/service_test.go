package assistantreview

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"opscopilot/pkg/assistantcontrol"
)

func TestOnlyMissingOrFailedFastRequestsFallBack(t *testing.T) {
	for _, tc := range []struct {
		name, choice                               string
		missing, failed, wantAllowed, wantFallback bool
	}{
		{name: "read only", choice: "A", wantAllowed: true},
		{name: "mutation", choice: "B"}, {name: "sensitive", choice: "C"}, {name: "scope", choice: "D"}, {name: "uncertain", choice: "E"},
		{name: "invalid", choice: "A because it is safe"}, {name: "empty"},
		{name: "missing", missing: true, wantAllowed: true, wantFallback: true},
		{name: "access error", failed: true, wantAllowed: true, wantFallback: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fallbackCalls, notifications := 0, 0
			s := Service{Fallback: func(_ context.Context, input assistantcontrol.ReviewInput) assistantcontrol.ReviewVerdict {
				fallbackCalls++
				if notifications != 1 {
					t.Fatal("fallback launched without visible notice")
				}
				if input.Action != "original command" {
					t.Fatal("action changed")
				}
				return assistantcontrol.ReviewVerdict{ReadOnly: true, InScope: true, Reason: "fixture review"}
			}, OnFallback: func(message string) {
				notifications++
				if !strings.Contains(message, "快速模型") || !strings.Contains(message, "iCode") || strings.Contains(message, "private-error") {
					t.Fatal(message)
				}
			}}
			if !tc.missing {
				s.Fast = func(ctx context.Context, _ assistantcontrol.ReviewInput) (string, error) {
					deadline, ok := ctx.Deadline()
					if !ok || time.Until(deadline) > FastTimeout {
						t.Fatal("missing bounded deadline")
					}
					if tc.failed {
						return "", errors.New("private-error API-key body")
					}
					return tc.choice, nil
				}
			}
			v := s.Review(context.Background(), assistantcontrol.ReviewInput{Action: "original command"})
			if v.Allowed() != tc.wantAllowed || (fallbackCalls == 1) != tc.wantFallback || (v.Notice != "") != tc.wantFallback {
				t.Fatal(v, fallbackCalls, notifications)
			}
		})
	}
}

func TestCancellationDoesNotStartFallbackOrAcceptLateResult(t *testing.T) {
	for _, where := range []string{"before", "fast", "fallback"} {
		t.Run(where, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			s := Service{Fallback: func(context.Context, assistantcontrol.ReviewInput) assistantcontrol.ReviewVerdict {
				calls++
				cancel()
				return assistantcontrol.ReviewVerdict{ReadOnly: true, InScope: true, Reason: "late"}
			}}
			if where == "before" {
				cancel()
			}
			if where == "fast" {
				s.Fast = func(context.Context, assistantcontrol.ReviewInput) (string, error) { cancel(); return "A", nil }
			}
			v := s.Review(ctx, assistantcontrol.ReviewInput{})
			if v.Allowed() || where != "fallback" && calls != 0 {
				t.Fatal(v, calls)
			}
		})
	}
}
