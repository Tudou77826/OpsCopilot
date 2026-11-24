package acp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"opscopilot/pkg/assistantcontrol"
)

func TestElicitationConventionalFormAnswersAndMapsValues(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	coordinator := &assistantcontrol.Coordinator{}
	turn, _ := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	bridge := &Interactions{Turn: turn, SessionID: "bound"}
	data := `{"sessionId":"bound","message":"请确认部署参数","requestedSchema":{"type":"object","mode":"form","properties":{"region":{"type":"string","title":"目标区域","description":"选择发布的目标区域","enum":["cn-north","cn-south"]},"confirm":{"type":"boolean","title":"确认执行","description":"是否继续"},"note":{"type":"string","title":"补充说明"}},"required":["region","confirm"]}}`
	finished := make(chan error, 1)
	go func() {
		result, err := bridge.Request(turn.Context(), "elicitation/create", json.RawMessage(data))
		if err != nil {
			finished <- err
			return
		}
		payload, _ := json.Marshal(result)
		var response struct {
			Action  string         `json:"action"`
			Content map[string]any `json:"content"`
		}
		if json.Unmarshal(payload, &response) != nil || response.Action != "accept" {
			finished <- errValidation{payload}
			return
		}
		if response.Content["region"] != "cn-north" || response.Content["confirm"] != false || response.Content["note"] != "请灰度发布" {
			finished <- errValidation{payload}
			return
		}
		finished <- nil
	}()
	first := awaitInteraction(t, coordinator)
	if first.Prompt.Title != "目标区域" || len(first.Prompt.Choices) != 2 {
		t.Fatal("first question missing choices", first.Prompt)
	}
	if err := coordinator.Respond(turn.ID(), first.Prompt.ID, assistantcontrol.Answer{Selected: []string{"0"}}); err != nil {
		t.Fatal(err)
	}
	second := awaitInteraction(t, coordinator)
	if second.Prompt.Title != "确认执行" || len(second.Prompt.Choices) != 2 {
		t.Fatal("boolean question missing choices", second.Prompt)
	}
	if err := coordinator.Respond(turn.ID(), second.Prompt.ID, assistantcontrol.Answer{Selected: []string{"1"}}); err != nil {
		t.Fatal(err)
	}
	third := awaitInteraction(t, coordinator)
	if len(third.Prompt.Choices) != 0 {
		t.Fatal("text question carried options", third.Prompt)
	}
	if err := coordinator.Respond(turn.ID(), third.Prompt.ID, assistantcontrol.Answer{Text: "请灰度发布"}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("elicitation did not complete")
	}
}

// errValidation carries a raw response payload as a test failure.
type errValidation struct{ payload []byte }

func (e errValidation) Error() string { return "unexpected response " + string(e.payload) }

func TestElicitationCodeAgentShapeNormalizesDescriptionsAndConst(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	coordinator := &assistantcontrol.Coordinator{}
	turn, _ := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	bridge := &Interactions{Turn: turn, SessionID: "bound"}
	// CodeAgent forms carry the question in the field description, the option
	// explanation in the option title and the real value in const.
	data := `{"sessionId":"bound","message":"answer questions?","requestedSchema":{"type":"object","properties":{"field":{"type":"string","description":"选择回滚策略","oneOf":[{"const":"rollback","title":"立即回滚到上一版本"},{"const":"hold","title":"保持现状并观察"}]}}}}`
	resultCh := make(chan any, 1)
	failure := make(chan error, 1)
	go func() {
		result, err := bridge.Request(turn.Context(), "elicitation/create", json.RawMessage(data))
		if err != nil {
			failure <- err
			return
		}
		resultCh <- result
	}()
	state := awaitInteraction(t, coordinator)
	if state.Prompt.Title != "选择回滚策略" {
		t.Fatal("field description did not become the question", state.Prompt)
	}
	if len(state.Prompt.Choices) != 2 || state.Prompt.Choices[0].Label == "" {
		t.Fatal("const options missing", state.Prompt)
	}
	// Written supplement merges into the selected value.
	if err := coordinator.Respond(turn.ID(), state.Prompt.ID, assistantcontrol.Answer{Selected: []string{"0"}, Text: "先通知值班"}); err != nil {
		t.Fatal(err)
	}
	var payload []byte
	select {
	case err := <-failure:
		t.Fatal("elicitation failed", err)
	case raw := <-resultCh:
		payload, _ = json.Marshal(raw)
	case <-time.After(5 * time.Second):
		t.Fatal("elicitation did not complete")
	}
	var response struct {
		Content map[string]any `json:"content"`
	}
	if json.Unmarshal(payload, &response) != nil || response.Content["field"] != "rollback\n补充说明：先通知值班" {
		t.Fatal("const value or note lost", string(payload))
	}
}

func TestElicitationURLAndOversizedFormsCancelled(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	coordinator := &assistantcontrol.Coordinator{}
	turn, _ := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	bridge := &Interactions{Turn: turn, SessionID: "bound"}
	cases := []string{
		`{"sessionId":"bound","message":"open","requestedSchema":{"type":"object","properties":{"url":{"type":"string","title":"地址"}}}}`,
		`{"sessionId":"bound","message":"form","requestedSchema":{"type":"object","mode":"form","properties":{"one":{"type":"string","title":"一"},"two":{"type":"string","title":"二"},"three":{"type":"string","title":"三"},"four":{"type":"string","title":"四"},"five":{"type":"string","title":"五"},"six":{"type":"string","title":"六"},"seven":{"type":"string","title":"七"},"eight":{"type":"string","title":"八"},"nine":{"type":"string","title":"九"}}}}`,
	}
	for _, data := range cases {
		result, err := bridge.Request(ctx, "elicitation/create", json.RawMessage(data))
		if err != nil {
			t.Fatal(err)
		}
		payload, _ := json.Marshal(result)
		if string(payload) != `{"action":"cancel"}` {
			t.Fatalf("unsafe form was not cancelled: %s", payload)
		}
	}
}

func TestPermissionAlwaysOptionsRememberAndReuse(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	coordinator := &assistantcontrol.Coordinator{}
	turn, _ := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	remembered := map[string]string{}
	bridge := &Interactions{Turn: turn, SessionID: "bound", AgentName: "CodeAgent",
		Remembered: func(key string) string { return remembered[key] },
		Remember:   func(key, decision string) { remembered[key] = decision }}
	data := `{"sessionId":"bound","toolCall":{"name":"bash","title":"执行脚本","kind":"execute","rawInput":{"command":"ls"}},"options":[{"optionId":"a1","kind":"allow_once"},{"optionId":"a2","kind":"allow_always"},{"optionId":"r1","kind":"reject_once"}]}`
	// First request: user picks allow_always; response must carry its option id.
	firstResult := make(chan any, 1)
	firstFailure := make(chan error, 1)
	go func() {
		result, err := bridge.Request(turn.Context(), "session/request_permission", json.RawMessage(data))
		if err != nil {
			firstFailure <- err
			return
		}
		firstResult <- result
	}()
	state := awaitInteraction(t, coordinator)
	if len(state.Prompt.Choices) != 3 {
		t.Fatal("allow_always choice missing", state.Prompt.Choices)
	}
	if err := coordinator.Respond(turn.ID(), state.Prompt.ID, assistantcontrol.Answer{Selected: []string{"allow_always"}}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-firstFailure:
		t.Fatal("permission failed", err)
	case result := <-firstResult:
		payload, _ := json.Marshal(result)
		var response struct {
			Outcome struct {
				Outcome  string `json:"outcome"`
				OptionID string `json:"optionId"`
			} `json:"outcome"`
		}
		if json.Unmarshal(payload, &response) != nil || response.Outcome.Outcome != "selected" || response.Outcome.OptionID != "a2" {
			t.Fatal("always-allow choice lost", string(payload))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("permission did not complete")
	}
	if len(remembered) != 1 {
		t.Fatal("decision not remembered", remembered)
	}
	// Second identical request applies the remembered decision with no dialog.
	// The wire answer carries the allow option the agent offered first.
	assertPermissionOutcome(t, bridge, ctx, data, "a1")
	// A remembered rejection answers with the reject option.
	remembered[permissionKey("bash", json.RawMessage(`{"command":"ls"}`))] = "reject"
	assertPermissionOutcome(t, bridge, ctx, data, "r1")
}

// assertPermissionOutcome checks one remembered-decision permission round trip.
func assertPermissionOutcome(t *testing.T, bridge *Interactions, ctx context.Context, data, expected string) {
	t.Helper()
	result, err := bridge.Request(ctx, "session/request_permission", json.RawMessage(data))
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(result)
	var response struct {
		Outcome struct {
			Outcome  string `json:"outcome"`
			OptionID string `json:"optionId"`
		} `json:"outcome"`
	}
	if json.Unmarshal(payload, &response) != nil || response.Outcome.Outcome != "selected" || response.Outcome.OptionID != expected {
		t.Fatalf("remembered decision mismatch: %s", payload)
	}
}

func TestPermissionUnknownOrMissingKindsCancelled(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	coordinator := &assistantcontrol.Coordinator{}
	turn, _ := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	bridge := &Interactions{Turn: turn, SessionID: "bound"}
	cases := []string{
		// Reject-only option sets leave no answerable direction: cancelled.
		`{"sessionId":"bound","toolCall":{"name":"x","title":"操作"},"options":[{"optionId":"r","kind":"reject_always"}]}`,
		`{"sessionId":"bound","toolCall":{"name":"x","title":"操作"},"options":[{"optionId":"a","kind":"allow_once"},{"optionId":"r","kind":"weird_kind"}]}`,
	}
	for _, data := range cases {
		result, err := bridge.Request(ctx, "session/request_permission", json.RawMessage(data))
		if err != nil {
			t.Fatal(err)
		}
		payload, _ := json.Marshal(result)
		if string(payload) != `{"outcome":{"outcome":"cancelled"}}` {
			t.Fatalf("unknown kinds were not cancelled: %s", payload)
		}
	}
}

func TestCodeAgentToolNameRestoredInPermission(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	coordinator := &assistantcontrol.Coordinator{}
	turn, _ := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	bridge := &Interactions{Turn: turn, SessionID: "bound", AgentName: "CodeAgent"}
	// No agent title: the approval card falls back to the real tool name.
	data := `{"sessionId":"bound","toolCall":{"name":"tool","kind":"edit","rawInput":{},"_meta":{"codeAgentCLI":{"toolName":"Edit"}}},"options":[{"optionId":"a","kind":"allow_once"},{"optionId":"r","kind":"reject_once"}]}`
	completed := make(chan error, 1)
	go func() {
		_, err := bridge.Request(turn.Context(), "session/request_permission", json.RawMessage(data))
		completed <- err
	}()
	state := awaitInteraction(t, coordinator)
	if state.Prompt.Title != "Edit" {
		t.Fatal("codeAgentCLI tool name not restored", state.Prompt)
	}
	if err := coordinator.Respond(turn.ID(), state.Prompt.ID, assistantcontrol.Answer{Selected: []string{"allow"}}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-completed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("permission did not complete")
	}
}

func TestPrivateSubagentNotificationsAreIgnored(t *testing.T) {
	coordinator := &assistantcontrol.Coordinator{}
	turn, _ := coordinator.Begin(context.Background(), assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	updates := 0
	bridge := &Interactions{Turn: turn, SessionID: "bound", OnUpdate: func(string, json.RawMessage) { updates++ }}
	// Bound handler only accepts session/update for the bound session.
	bridge.Notify(context.Background(), "session/update", json.RawMessage(`{"sessionId":"other","update":{"sessionUpdate":"agent_message_chunk"}}`))
	bridge.Notify(context.Background(), "_subagent/update", json.RawMessage(`{"sessionId":"bound","taskId":"1","update":{"sessionUpdate":"agent_message_chunk"}}`))
	if updates != 0 {
		t.Fatal("unbound or private notification reached the turn")
	}
	bridge.Notify(context.Background(), "session/update", json.RawMessage(`{"sessionId":"bound","update":{"sessionUpdate":"agent_message_chunk"}}`))
	if updates != 1 {
		t.Fatal("bound standard update was dropped")
	}
	// The idle handler must also drop private notifications.
	idle := idleFeaturesHandler{router: &managedRouter{sessionID: "s1"}}
	idle.Notify(context.Background(), "_subagent/update", json.RawMessage(`{"sessionId":"s1"}`))
}

func TestStandardACPPredicateCoversAllStandardKinds(t *testing.T) {
	for kind, expected := range map[string]bool{"": false, "icode": false, "opencode": true, "custom": true, "codeagent": true, "unknown": false} {
		if got := (ManagedConfig{AgentKind: kind}).IsStandardACP(); got != expected {
			t.Fatalf("kind %q predicate = %v, want %v", kind, got, expected)
		}
	}
}

func TestElicitationChoiceIndexBoundsAndRequiredAnswers(t *testing.T) {
	if _, ok := elicitationChoiceIndex("2", 2); ok {
		t.Fatal("out-of-range choice accepted")
	}
	if number, ok := elicitationChoiceIndex("1", 2); !ok || number != 1 {
		t.Fatal("valid choice rejected")
	}
	question, ok := parseElicitationField("op", json.RawMessage(`{"type":"string","title":"策略","enum":["fast","slow"]}`), true)
	if !ok || len(question.Options) != 2 || !question.Textual || question.Boolean {
		t.Fatal("enum field misparsed", question)
	}
	if _, ok = parseElicitationField("bad", json.RawMessage(`{"type":"integer","title":"数量"}`), true); ok {
		t.Fatal("numeric field accepted without a bounded shape")
	}
	boolean, ok := parseElicitationField("flag", json.RawMessage(`{"type":"boolean","title":"继续？"}`), false)
	if !ok || len(boolean.Options) != 2 {
		t.Fatal("boolean field misparsed", boolean)
	}
}

func TestResumeTargetRequiresStandardLoadCapability(t *testing.T) {
	withLoad := InitializeResult{ProtocolVersion: 1}
	withLoad.AgentCapabilities.LoadSession = true
	withoutLoad := InitializeResult{ProtocolVersion: 1}
	cases := []struct {
		name         string
		config       ManagedConfig
		runtime      InitializeResult
		sessionID    string
		expectResume bool
	}{
		{"no session yet", ManagedConfig{AgentKind: "codeagent"}, withLoad, "", false},
		{"standard agent with load", ManagedConfig{AgentKind: "codeagent"}, withLoad, "s1", true},
		{"agent without load capability", ManagedConfig{AgentKind: "codeagent"}, withoutLoad, "s1", false},
		{"non-standard agent", ManagedConfig{AgentKind: "icode"}, withLoad, "s1", false},
	}
	for _, tc := range cases {
		c := &Conversation{config: tc.config, runtimeInfo: tc.runtime, sessionID: tc.sessionID}
		if got := c.ResumeTarget(); (got != "") != tc.expectResume {
			t.Fatalf("%s: resume target %q", tc.name, got)
		}
	}
}
