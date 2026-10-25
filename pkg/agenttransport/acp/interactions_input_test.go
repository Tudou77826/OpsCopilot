package acp

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"opscopilot/pkg/assistantcontrol"
)

func TestStructuredInputUsesValuesForFreeTextAndNoteForSelectedOptions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		options string
		answer  assistantcontrol.Answer
		values  []string
		note    string
	}{
		{"free-text", `[]`, assistantcontrol.Answer{Text: "orders-api"}, []string{"orders-api"}, ""},
		{"typed-other", `[{"label":"A"}]`, assistantcontrol.Answer{Text: "B"}, []string{"B"}, ""},
		{"selected-note", `[{"label":"A"}]`, assistantcontrol.Answer{Selected: []string{"0"}, Text: "today"}, []string{"A"}, "today"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &assistantcontrol.Coordinator{}
			turn, _ := c.Begin(context.Background(), assistantcontrol.Target{}, nil)
			defer turn.Finish("completed", "")
			bridge := &Interactions{Turn: turn, SessionID: "bound"}
			done := make(chan struct {
				result any
				err    error
			}, 1)
			go func() {
				result, err := bridge.Request(turn.Context(), "_chrys/request_input", json.RawMessage(`{"sessionId":"bound","questions":[{"question":"Service?","options":`+tc.options+`}]}`))
				done <- struct {
					result any
					err    error
				}{result, err}
			}()
			state := awaitInteraction(t, c)
			if err := c.Respond(turn.ID(), state.Prompt.ID, tc.answer); err != nil {
				t.Fatal(err)
			}
			result := <-done
			if result.err != nil {
				t.Fatal(result.err)
			}
			payload, _ := json.Marshal(result.result)
			var response struct {
				Answers []struct {
					Values []string `json:"values"`
					Note   string   `json:"note"`
				} `json:"answers"`
				Cancelled bool `json:"cancelled"`
			}
			if err := json.Unmarshal(payload, &response); err != nil || len(response.Answers) != 1 || response.Cancelled {
				t.Fatal(string(payload), err)
			}
			if !reflect.DeepEqual(response.Answers[0].Values, tc.values) || response.Answers[0].Note != tc.note {
				t.Fatal(string(payload))
			}
		})
	}
}
