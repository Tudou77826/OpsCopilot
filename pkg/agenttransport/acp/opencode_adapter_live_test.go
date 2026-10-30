package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/assistantskills"
)

func TestLiveOpenCodeAdapterConversation(t *testing.T) {
	exe := os.Getenv("OPSCOPILOT_OPENCODE_TEST_EXE")
	if exe == "" {
		t.Skip("opt-in OpenCode")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("PROJECT_TEST_RULE_ALPHA"), 0600); err != nil {
		t.Fatal(err)
	}
	commandDir := filepath.Join(dir, ".opencode", "commands")
	if err := os.MkdirAll(commandDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(commandDir, "probe.md"), []byte("---\ndescription: Test command\n---\nPROJECT_COMMAND_INSTRUCTION_X\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(commandDir, "expanded.md"), []byte("Result: !`echo EXPANSION_MARKER_Z`\n"), 0600); err != nil {
		t.Fatal(err)
	}
	skillSource := t.TempDir()
	if err := os.WriteFile(filepath.Join(skillSource, "SKILL.md"), []byte("---\nname: workspace-sample\ndescription: CUSTOM_SKILL_DESC_ZETA\n---\nInstructions\n"), 0600); err != nil {
		t.Fatal(err)
	}
	library := assistantskills.Library{Directory: filepath.Join(t.TempDir(), "library")}
	if err := library.Import(skillSource, false); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var requests []string
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body any
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			w.WriteHeader(400)
			return
		}
		data, _ := json.Marshal(body)
		mu.Lock()
		requests = append(requests, string(data))
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		write := func(v any) { b, _ := json.Marshal(v); _, _ = fmt.Fprintf(w, "data: %s\n\n", b) }
		write(map[string]any{"id": "fixture", "object": "chat.completion.chunk", "created": 1, "model": "test", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": "FIXTURE_OK"}, "finish_reason": nil}}})
		write(map[string]any{"id": "fixture", "object": "chat.completion.chunk", "created": 1, "model": "test", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 30, "completion_tokens": 4, "total_tokens": 34}})
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer model.Close()
	extra := t.TempDir()
	workspace, _ := json.Marshal([]string{dir, extra})
	cfg := ManagedConfig{AgentKind: "opencode", Executable: exe, BaseURL: model.URL + "/v1", APIKey: "fixture", Model: "test", ModelContextSize: 64000, WorkspaceJSON: string(workspace), SkillsDirectory: library.Directory}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c := &Conversation{}
	defer c.Close()
	co := &assistantcontrol.Coordinator{}
	for _, prompt := range []string{"FIRST_MARKER", "SECOND_MARKER"} {
		turn, err := co.Begin(ctx, assistantcontrol.Target{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		turn.SetApprovalMode("auto")
		err = c.Run(ctx, cfg, turn, &managedFixtureTools{}, prompt)
		turn.Finish("completed", "")
		if err != nil {
			t.Fatal(err)
		}
	}
	if !c.AgentCommandMatches("/probe") {
		t.Fatal("project command was not advertised")
	}
	var foundUnavailable bool
	for _, command := range c.SessionFeatures().Commands {
		if command.Name == "expanded" && command.Unavailable != "" {
			foundUnavailable = true
		}
	}
	if !foundUnavailable {
		t.Fatal("reviewed mode must identify unsupported command expansion")
	}
	commandTurn, err := co.Begin(ctx, assistantcontrol.Target{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	commandTurn.SetApprovalMode("auto")
	err = c.Run(ctx, cfg, commandTurn, &managedFixtureTools{}, "/probe")
	commandTurn.Finish("completed", "")
	if err != nil {
		t.Fatal(err)
	}
	full := &Conversation{}
	defer full.Close()
	fullConfig := cfg
	fullConfig.ApprovalMode = "bypass"
	fullTurn, err := co.Begin(ctx, assistantcontrol.Target{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	fullTurn.SetApprovalMode("bypass")
	err = full.Run(ctx, fullConfig, fullTurn, &managedFixtureTools{}, "/expanded")
	fullTurn.Finish("completed", "")
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) < 2 {
		t.Fatalf("model calls = %d", len(requests))
	}
	var last string
	for _, request := range requests {
		if strings.Contains(request, "PROJECT_COMMAND_INSTRUCTION_X") {
			last = request
		}
	}
	if !strings.Contains(requests[len(requests)-1], "Result: EXPANSION_MARKER_Z") {
		t.Fatal("full access did not expand native command")
	}
	if !strings.Contains(last, "PROJECT_COMMAND_INSTRUCTION_X") {
		t.Fatal("advertised command was not expanded by the agent")
	}
	if !strings.Contains(last, "PROJECT_TEST_RULE_ALPHA") || !strings.Contains(last, "FIRST_MARKER") || !strings.Contains(last, "SECOND_MARKER") {
		t.Fatalf("project rule=%v first=%v second=%v, calls=%d request-rules=%v", strings.Contains(last, "PROJECT_TEST_RULE_ALPHA"), strings.Contains(last, "FIRST_MARKER"), strings.Contains(last, "SECOND_MARKER"), len(requests), func() []bool {
			var v []bool
			for _, r := range requests {
				v = append(v, strings.Contains(r, "PROJECT_TEST_RULE_ALPHA"))
			}
			return v
		}())
	}
	if !strings.Contains(last, "CUSTOM_SKILL_DESC_ZETA") {
		t.Fatal("enabled Ops skill was not made available")
	}
	if !strings.Contains(last, filepath.Base(extra)) {
		t.Fatal("additional project directory was not sent")
	}
	if !c.ContextUsage().Known {
		t.Fatal("context usage not received")
	}
	if len(c.SessionFeatures().Options) == 0 {
		t.Fatal("model options not received")
	}
}

func TestLiveOpenCodeAdapterAutomaticReview(t *testing.T) {
	exe := os.Getenv("OPSCOPILOT_OPENCODE_TEST_EXE")
	if exe == "" {
		t.Skip("opt-in OpenCode")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("local fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	arguments, _ := json.Marshal(map[string]string{"filePath": path})
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Role string `json:"role"`
			} `json:"messages"`
			Tools []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
		}
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			w.WriteHeader(400)
			return
		}
		name := ""
		for _, tool := range req.Tools {
			if tool.Function.Name == "read" {
				name = tool.Function.Name
				break
			}
		}
		done := false
		for _, msg := range req.Messages {
			if msg.Role == "tool" {
				done = true
			}
		}
		delta := map[string]any{"role": "assistant", "content": "done"}
		finish := "stop"
		if name != "" && !done {
			delta = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": "one", "type": "function", "function": map[string]string{"name": name, "arguments": string(arguments)}}}}
			finish = "tool_calls"
		}
		w.Header().Set("Content-Type", "text/event-stream")
		write := func(v any) { b, _ := json.Marshal(v); _, _ = fmt.Fprintf(w, "data: %s\n\n", b) }
		write(map[string]any{"id": "fixture", "object": "chat.completion.chunk", "created": 1, "model": "test", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": nil}}})
		write(map[string]any{"id": "fixture", "object": "chat.completion.chunk", "created": 1, "model": "test", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": finish}}, "usage": map[string]int{"prompt_tokens": 30, "completion_tokens": 4, "total_tokens": 34}})
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer model.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	co := &assistantcontrol.Coordinator{}
	turn, err := co.Begin(ctx, assistantcontrol.Target{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	turn.SetApprovalMode("auto")
	defer turn.Finish("completed", "")
	var reviews atomic.Int32
	c := &Conversation{}
	defer c.Close()
	c.SetStandardReviewer(func(_ context.Context, input assistantcontrol.ReviewInput) assistantcontrol.ReviewVerdict {
		if input.UserRequest != "读取测试文件" {
			t.Errorf("wrong request: %q", input.UserRequest)
		}
		reviews.Add(1)
		return assistantcontrol.ReviewVerdict{LowRisk: true, InScope: true, Reason: "只读查看"}
	})
	tools := &managedFixtureTools{}
	workspace, _ := json.Marshal([]string{dir})
	cfg := ManagedConfig{AgentKind: "opencode", Executable: exe, BaseURL: model.URL + "/v1", APIKey: "fixture", Model: "test", ModelContextSize: 64000, ApprovalMode: "auto", WorkspaceJSON: string(workspace)}
	if err = c.Run(ctx, cfg, turn, tools, "读取测试文件"); err != nil {
		t.Fatal(err)
	}
	if reviews.Load() != 1 || tools.calls.Load() != 0 || co.Snapshot().Prompt != nil {
		t.Fatalf("reviews=%d calls=%d prompt=%v", reviews.Load(), tools.calls.Load(), co.Snapshot().Prompt != nil)
	}
}

func TestLiveOpenCodeAdapterUsesLocalModelConfig(t *testing.T) {
	exe := os.Getenv("OPSCOPILOT_OPENCODE_TEST_EXE")
	if exe == "" {
		t.Skip("opt-in OpenCode")
	}
	root := t.TempDir()
	configDir, dataDir := filepath.Join(root, "config"), filepath.Join(root, "data")
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("XDG_DATA_HOME", dataDir)
	if err := os.MkdirAll(filepath.Join(configDir, "opencode"), 0700); err != nil {
		t.Fatal(err)
	}
	config := `{"model":"fixture/native","provider":{"fixture":{"npm":"@ai-sdk/openai-compatible","name":"Fixture","options":{"baseURL":"http://127.0.0.1:1/v1","apiKey":"secret-fixture"},"models":{"native":{"name":"Native"}}}},"agent":{"build":{"permission":"allow"}}}`
	if err := os.WriteFile(filepath.Join(configDir, "opencode", "opencode.json"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	c := &Conversation{}
	defer c.Close()
	if err := c.start(ctx, ctx, ManagedConfig{AgentKind: "opencode", Executable: exe}); err != nil {
		t.Fatal(err)
	}
	for _, option := range c.SessionFeatures().Options {
		if option.Category == "model" && option.Current == "fixture/native" {
			return
		}
	}
	t.Fatal("local model selection was not preserved")
}

// Explicit opt-in: uses the user's configured model, but no SSH, workspace,
// application tools, or user data are sent in the prompt.
func TestLiveOpenCodeUserModelChat(t *testing.T) {
	exe := os.Getenv("OPSCOPILOT_OPENCODE_TEST_EXE")
	if exe == "" || os.Getenv("OPSCOPILOT_OPENCODE_REAL_MODEL") != "1" {
		t.Skip("opt-in real model")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	co := &assistantcontrol.Coordinator{}
	turn, err := co.Begin(ctx, assistantcontrol.Target{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	turn.SetApprovalMode("manual")
	defer turn.Finish("completed", "")
	c := &Conversation{}
	defer c.Close()
	cfg := ManagedConfig{AgentKind: "opencode", Executable: exe, ApprovalMode: "manual"}
	if err := c.Run(ctx, cfg, turn, &managedFixtureTools{}, "请只回复 OK，不要调用工具。"); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(co.Snapshot().Output) == "" {
		t.Fatal("model returned no answer")
	}
}

func TestLiveOpenCodeHostToolRequiresOnlyOneManualConfirmation(t *testing.T) {
	exe := os.Getenv("OPSCOPILOT_OPENCODE_TEST_EXE")
	if exe == "" {
		t.Skip("opt-in OpenCode")
	}
	for _, tc := range []struct {
		name, mode, answer string
		wantCalls          int32
	}{{"allow", "manual", "allow", 1}, {"reject", "manual", "reject", 0}, {"full", "bypass", "", 1}} {
		t.Run(tc.name, func(t *testing.T) {
			model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Messages []struct {
						Role string `json:"role"`
					} `json:"messages"`
					Tools []struct {
						Function struct {
							Name string `json:"name"`
						} `json:"function"`
					} `json:"tools"`
				}
				if json.NewDecoder(r.Body).Decode(&req) != nil {
					w.WriteHeader(400)
					return
				}
				name := ""
				for _, tool := range req.Tools {
					if strings.HasSuffix(tool.Function.Name, "workspace_open") {
						name = tool.Function.Name
						break
					}
				}
				done := false
				for _, msg := range req.Messages {
					if msg.Role == "tool" {
						done = true
					}
				}
				delta := map[string]any{"role": "assistant", "content": "done"}
				finish := "stop"
				if name != "" && !done {
					delta = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": "one", "type": "function", "function": map[string]string{"name": name, "arguments": `{"connection_id":"fixture"}`}}}}
					finish = "tool_calls"
				}
				w.Header().Set("Content-Type", "text/event-stream")
				write := func(v any) { b, _ := json.Marshal(v); _, _ = fmt.Fprintf(w, "data: %s\n\n", b) }
				write(map[string]any{"id": "fixture", "object": "chat.completion.chunk", "created": 1, "model": "test", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": nil}}})
				write(map[string]any{"id": "fixture", "object": "chat.completion.chunk", "created": 1, "model": "test", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": finish}}, "usage": map[string]int{"prompt_tokens": 30, "completion_tokens": 4, "total_tokens": 34}})
				_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
			}))
			defer model.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			co := &assistantcontrol.Coordinator{}
			turn, err := co.Begin(ctx, assistantcontrol.Target{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			turn.SetApprovalMode(tc.mode)
			defer turn.Finish("completed", "")
			conversation := &Conversation{}
			defer conversation.Close()
			tools := &managedFixtureTools{}
			cfg := ManagedConfig{AgentKind: "opencode", Executable: exe, BaseURL: model.URL + "/v1", APIKey: "fixture", Model: "test", ModelContextSize: 64000, ApprovalMode: tc.mode}
			done := make(chan error, 1)
			go func() { done <- conversation.Run(ctx, cfg, turn, tools, "打开测试连接") }()
			if tc.answer != "" {
				deadline := time.Now().Add(30 * time.Second)
				for co.Snapshot().Prompt == nil && time.Now().Before(deadline) {
					time.Sleep(20 * time.Millisecond)
				}
				prompt := co.Snapshot().Prompt
				if prompt == nil {
					t.Fatal("missing host approval")
				}
				if err := co.Respond(turn.ID(), prompt.ID, assistantcontrol.Answer{Selected: []string{tc.answer}}); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal("assistant did not complete; possible duplicate approval")
			}
			if tools.calls.Load() != tc.wantCalls {
				t.Fatalf("tool calls=%d", tools.calls.Load())
			}
		})
	}
}

func TestLiveCustomOpenCodeStdioTool(t *testing.T) {
	exe := os.Getenv("OPSCOPILOT_OPENCODE_TEST_EXE")
	if exe == "" {
		t.Skip("opt-in OpenCode")
	}
	var sawTool atomic.Bool
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Role string `json:"role"`
			} `json:"messages"`
			Tools []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
		}
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			w.WriteHeader(400)
			return
		}
		var name string
		for _, tool := range req.Tools {
			if strings.HasSuffix(tool.Function.Name, "workspace_list") {
				name = tool.Function.Name
				sawTool.Store(true)
				break
			}
		}
		done := false
		for _, message := range req.Messages {
			if message.Role == "tool" {
				done = true
			}
		}
		delta := map[string]any{"role": "assistant", "content": "done"}
		finish := "stop"
		if name != "" && !done {
			delta = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": "fixture-one", "type": "function", "function": map[string]string{"name": name, "arguments": "{}"}}}}
			finish = "tool_calls"
		}
		w.Header().Set("Content-Type", "text/event-stream")
		write := func(value any) { data, _ := json.Marshal(value); _, _ = fmt.Fprintf(w, "data: %s\n\n", data) }
		write(map[string]any{"id": "fixture", "object": "chat.completion.chunk", "created": 1, "model": "test", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": nil}}})
		write(map[string]any{"id": "fixture", "object": "chat.completion.chunk", "created": 1, "model": "test", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": finish}}, "usage": map[string]int{"prompt_tokens": 30, "completion_tokens": 4, "total_tokens": 34}})
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer model.Close()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("OPENCODE_DISABLE_PROJECT_CONFIG", "1")
	t.Setenv("OPENCODE_PURE", "1")
	configDir := filepath.Join(root, "config", "opencode")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	config, _ := json.Marshal(map[string]any{"model": "fixture/test", "permission": "allow", "provider": map[string]any{"fixture": map[string]any{"npm": "@ai-sdk/openai-compatible", "name": "Fixture", "options": map[string]string{"baseURL": model.URL + "/v1", "apiKey": "fixture"}, "models": map[string]any{"test": map[string]any{"name": "test", "limit": map[string]int{"context": 64000, "output": 4096}}}}}})
	if err := os.WriteFile(filepath.Join(configDir, "opencode.json"), config, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	co := &assistantcontrol.Coordinator{}
	turn, err := co.Begin(ctx, assistantcontrol.Target{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	turn.SetApprovalMode("bypass")
	defer turn.Finish("completed", "")
	conversation := &Conversation{}
	defer conversation.Close()
	tools := &managedFixtureTools{}
	err = conversation.Run(ctx, ManagedConfig{AgentKind: "custom", Executable: exe, ArgumentsJSON: `["acp"]`, ApprovalMode: "bypass"}, turn, tools, "查看测试工作区")
	if err != nil {
		t.Fatal(err)
	}
	if !sawTool.Load() || tools.calls.Load() != 1 {
		t.Fatalf("stdio tool observed=%v calls=%d", sawTool.Load(), tools.calls.Load())
	}
}
