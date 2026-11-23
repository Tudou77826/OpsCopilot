package acp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"opscopilot/pkg/assistantcontrol"
)

func TestStandardAgentPermissionReviewModes(t *testing.T) {
	for _, tc := range []struct {
		mode       string
		allowed    bool
		wantPrompt bool
		wantReview bool
	}{{"auto", true, false, true}, {"auto", false, true, true}, {"manual", true, true, false}, {"bypass", false, false, false}} {
		t.Run(tc.mode+"/"+map[bool]string{true: "allowed", false: "uncertain"}[tc.allowed], func(t *testing.T) {
			co := &assistantcontrol.Coordinator{}
			turn, err := co.Begin(context.Background(), assistantcontrol.Target{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			turn.SetApprovalMode(tc.mode)
			defer turn.Finish("completed", "")
			called := make(chan assistantcontrol.ReviewInput, 1)
			bridge := &Interactions{Turn: turn, SessionID: "s", UserRequest: "检查状态", AgentName: "OpenCode", PermissionReview: func(_ context.Context, input assistantcontrol.ReviewInput) assistantcontrol.ReviewVerdict {
				called <- input
				return assistantcontrol.ReviewVerdict{LowRisk: tc.allowed, InScope: tc.allowed, Reason: "test"}
			}}
			done := make(chan any, 1)
			go func() {
				result, err := bridge.Request(turn.Context(), "session/request_permission", json.RawMessage(`{"sessionId":"s","toolCall":{"kind":"execute","title":"check","rawInput":{"command":"ls"}},"options":[{"optionId":"yes","kind":"allow_once"},{"optionId":"no","kind":"reject_once"}]}`))
				if err != nil {
					done <- err
				} else {
					done <- result
				}
			}()
			if tc.wantPrompt {
				deadline := time.Now().Add(time.Second)
				for co.Snapshot().Prompt == nil && time.Now().Before(deadline) {
					time.Sleep(time.Millisecond)
				}
				p := co.Snapshot().Prompt
				if p == nil {
					t.Fatal("expected human confirmation")
				}
				if err := co.Respond(turn.ID(), p.ID, assistantcontrol.Answer{Selected: []string{"reject"}}); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case result := <-done:
				body, _ := json.Marshal(result)
				want := `"optionId":"yes"`
				if tc.wantPrompt {
					want = `"optionId":"no"`
				}
				if !strings.Contains(string(body), want) {
					t.Fatalf("result = %s", body)
				}
			case <-time.After(time.Second):
				t.Fatal("permission did not complete")
			}
			select {
			case input := <-called:
				if !tc.wantReview || input.UserRequest != "检查状态" || !strings.Contains(input.Action, "ls") {
					t.Fatalf("review input = %+v", input)
				}
			default:
				if tc.wantReview {
					t.Fatal("review was not called")
				}
			}
		})
	}
}
