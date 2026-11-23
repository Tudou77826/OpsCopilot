package assistantcontrol

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestAgentActionReviewerRequiresUnambiguousDecisionAndContext(t *testing.T) {
	input := ReviewInput{UserRequest: "检查服务状态", Tool: "terminal_execute", Location: "server", Action: "systemctl status demo"}
	tests := []struct {
		name, response string
		err            error
		allowed        bool
	}{
		{"allow", `{"decision":"allow","reason":"只读且符合请求"}`, nil, true},
		{"ask", `{"decision":"ask","reason":"可能修改数据"}`, nil, false},
		{"failure", "", errors.New("unavailable"), false},
		{"empty reason", `{"decision":"allow","reason":""}`, nil, false},
		{"unexpected field", `{"decision":"allow","reason":"ok","extra":true}`, nil, false},
		{"trailing JSON", `{"decision":"allow","reason":"ok"} {}`, nil, false},
		{"duplicate decision", `{"decision":"ask","decision":"allow","reason":"ok"}`, nil, false},
		{"unknown decision", `{"decision":"maybe","reason":"ok"}`, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			judge := AgentActionReviewer(func(_ context.Context, instructions, data string) (string, error) {
				called = true
				if !strings.Contains(instructions, "untrusted") || !strings.Contains(data, input.Action) || !strings.Contains(data, input.UserRequest) {
					t.Fatal("review did not receive the action and user request")
				}
				return tt.response, tt.err
			})
			if got := judge(context.Background(), input); got.Allowed() != tt.allowed || !called {
				t.Fatalf("verdict = %+v, called = %v", got, called)
			}
		})
	}
	called := false
	judge := AgentActionReviewer(func(context.Context, string, string) (string, error) { called = true; return "", nil })
	input.UserRequest = ""
	if judge(context.Background(), input).Allowed() || called {
		t.Fatal("missing user request must not be auto-approved")
	}
}
