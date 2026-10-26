package opscli

import (
	"context"
	"errors"
	"opscopilot/pkg/core/ops"
	"strings"
	"testing"
)

func TestRedactingErrorsPreservesDecisionStatus(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status string
	}{{ops.ErrPolicy, "denied"}, {context.Canceled, "cancelled"}, {context.DeadlineExceeded, "timeout"}} {
		redacted := redactedError{message: "安全错误摘要", cause: tc.err}
		if !errors.Is(redacted, tc.err) || errorStatus(redacted) != tc.status {
			t.Fatal("redaction lost machine-readable error status")
		}
	}
}
func TestInputDoesNotEchoInvalidCredentialJSON(t *testing.T) {
	var value map[string]any
	for _, input := range []string{`{"password":"private"`, `{} {"password":"private"}`, strings.Repeat("private", 200000)} {
		err := decodeInput("-", strings.NewReader(input), &value)
		if err == nil || strings.Contains(err.Error(), "private") {
			t.Fatal("invalid input accepted or leaked content")
		}
	}
}

func TestRedactionDoesNotExposeOverlappingCredentialSuffixes(t *testing.T) {
	got := redactText("prefix-abcdef", []string{"abc", "abcdef"})
	if strings.Contains(got, "def") {
		t.Fatalf("long credential suffix survived redaction: %s", got)
	}
}
