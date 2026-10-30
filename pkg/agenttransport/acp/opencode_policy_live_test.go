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
	"sync/atomic"
	"testing"
	"time"

	"opscopilot/pkg/agenttransport/mcp"
)

type opencodeProbeCaller struct{ calls atomic.Int32 }

func (c *opencodeProbeCaller) Call(_ context.Context, name, _ string) (any, error) {
	if name != "workspace_list" {
		return nil, fmt.Errorf("unexpected tool: %s", name)
	}
	c.calls.Add(1)
	return map[string]any{"fixture": "bridge-ok"}, nil
}

type opencodePermissionProbe struct {
	decision string
	requests atomic.Int32
}

func (p *opencodePermissionProbe) Notify(context.Context, string, json.RawMessage) {}

func (p *opencodePermissionProbe) Request(_ context.Context, method string, data json.RawMessage) (any, error) {
	if method != "session/request_permission" {
		return nil, &RPCError{Code: -32601, Message: "unsupported"}
	}
	var request struct {
		Options []struct{ Kind, OptionID string } `json:"options"`
	}
	if err := json.Unmarshal(data, &request); err != nil {
		return nil, err
	}
	p.requests.Add(1)
	for _, option := range request.Options {
		if option.Kind == p.decision {
			return map[string]any{"outcome": map[string]string{"outcome": "selected", "optionId": option.OptionID}}, nil
		}
	}
	return map[string]any{"outcome": map[string]string{"outcome": "cancelled"}}, nil
}

// Real OpenCode + Ops MCP, using a deterministic local model and an isolated
// project. The project attempts to allow all actions; the host override must
// still deliver a one-shot approval request and respect the user's decision.
func TestLiveOpenCodeProjectCannotOverrideHostApproval(t *testing.T) {
	exe := os.Getenv("OPSCOPILOT_OPENCODE_TEST_EXE")
	if exe == "" {
		t.Skip("opt-in OpenCode")
	}
	for _, tc := range []struct {
		name, decision string
		wantCalls      int32
	}{{"allow", "allow_once", 1}, {"reject", "reject_once", 0}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "opencode.json"), []byte(`{"agent":{"build":{"permission":{"*":"allow","opscopilot_workspace_list":"allow"}}}}`), 0600); err != nil {
				t.Fatal(err)
			}
			globalConfig := filepath.Join(dir, "xdg_config_home", "opencode")
			if err := os.MkdirAll(globalConfig, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(globalConfig, "opencode.json"), []byte(`{"agent":{"build":{"permission":{"opscopilot_workspace_list":"allow"}}}}`), 0600); err != nil {
				t.Fatal(err)
			}
			caller := &opencodeProbeCaller{}
			server, err := mcp.Start(ctx, caller)
			if err != nil {
				t.Fatal(err)
			}
			defer server.Close()
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
				toolName := ""
				for _, tool := range req.Tools {
					if strings.HasSuffix(tool.Function.Name, "workspace_list") {
						toolName = tool.Function.Name
						break
					}
				}
				hasToolResult := false
				for _, msg := range req.Messages {
					if msg.Role == "tool" {
						hasToolResult = true
					}
				}
				delta := map[string]any{"role": "assistant", "content": "done"}
				finish := "stop"
				if toolName != "" && !hasToolResult {
					delta = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": "probe-call", "type": "function", "function": map[string]string{"name": toolName, "arguments": "{}"}}}}
					finish = "tool_calls"
				}
				w.Header().Set("Content-Type", "text/event-stream")
				write := func(v any) { b, _ := json.Marshal(v); _, _ = fmt.Fprintf(w, "data: %s\n\n", b) }
				write(map[string]any{"id": "fixture", "object": "chat.completion.chunk", "created": 1, "model": "fixture", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": nil}}})
				write(map[string]any{"id": "fixture", "object": "chat.completion.chunk", "created": 1, "model": "fixture", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": finish}}, "usage": map[string]int{"prompt_tokens": 30, "completion_tokens": 4, "total_tokens": 34}})
				_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
			}))
			defer model.Close()
			config := map[string]any{
				"model": "fixture/test", "permission": "ask",
				"agent":    map[string]any{"build": map[string]any{"permission": "ask"}},
				"provider": map[string]any{"fixture": map[string]any{"npm": "@ai-sdk/openai-compatible", "name": "Fixture", "options": map[string]string{"baseURL": model.URL + "/v1", "apiKey": "fixture"}, "models": map[string]any{"test": map[string]any{"name": "Test", "limit": map[string]int{"context": 64000, "output": 4000}}}}},
			}
			settings, _ := json.Marshal(config)
			env := append(probeEnvironment(dir), "OPENCODE_CONFIG_CONTENT="+string(settings), "OPENCODE_PURE=1", "OPENCODE_DISABLE_PROJECT_CONFIG=1")
			handler := &opencodePermissionProbe{decision: tc.decision}
			client, err := Start(ctx, ProcessConfig{Executable: exe, Args: []string{"acp"}, Directory: dir, Environment: env}, handler)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			if _, err = client.Initialize(ctx); err != nil {
				t.Fatal(err)
			}
			var session struct {
				SessionID string `json:"sessionId"`
			}
			servers := []any{map[string]any{"type": "http", "name": "opscopilot", "url": server.URL, "headers": []any{map[string]string{"name": "Authorization", "value": "Bearer " + server.Token}}}}
			if err = client.Call(ctx, "session/new", map[string]any{"cwd": dir, "mcpServers": servers}, &session); err != nil {
				t.Fatal(err)
			}
			var result struct {
				StopReason string `json:"stopReason"`
			}
			if err = client.Call(ctx, "session/prompt", map[string]any{"sessionId": session.SessionID, "prompt": []any{map[string]string{"type": "text", "text": "Call workspace_list."}}}, &result); err != nil {
				t.Fatal(err)
			}
			if handler.requests.Load() != 1 || caller.calls.Load() != tc.wantCalls {
				t.Fatalf("permissions = %d, tool calls = %d", handler.requests.Load(), caller.calls.Load())
			}
		})
	}
}
