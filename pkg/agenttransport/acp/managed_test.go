package acp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"opscopilot/pkg/agenttransport/mcp"
	"opscopilot/pkg/assistantcontrol"
)

type managedFixtureTools struct{ calls atomic.Int32 }

func (f *managedFixtureTools) Yield() error { return nil }
func (f *managedFixtureTools) Call(ctx context.Context, name, command string) (any, error) {
	f.calls.Add(1)
	return map[string]string{"directory": "/fixture", "source": "host-tool"}, nil
}

func TestLiveChrysManagedTools(t *testing.T) {
	executable := os.Getenv("OPSCOPILOT_CHRYS_TEST_EXE")
	if executable == "" {
		t.Skip("opt-in local Chrys + simulated model")
	}
	var calls atomic.Int32
	var toolName string
	key := "private-fixture-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if key != "" && r.Header.Get("Authorization") != "Bearer "+key {
			t.Error("model credential was not resolved correctly")
		}
		var request struct {
			Messages []json.RawMessage `json:"messages"`
			Tools    []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			t.Error("bad model request")
			w.WriteHeader(400)
			return
		}
		for _, tool := range request.Tools {
			name := tool.Function.Name
			if name == "read_file" || name == "write_file" || name == "edit_file" || name == "glob" || name == "grep" || name == "list_workspace_dirs" || name == "view_image" || name == "pwsh" || name == "powershell" || name == "git_bash" || name == "bash" || name == "zsh" || name == "sh" || name == "cmd" {
				continue
			}
			databaseTool := false
			for _, allowed := range append(append(append(mcp.DatabaseToolNames(), mcp.FileToolNames()...), mcp.LocalDirectoryToolNames()...), mcp.ReferenceToolNames()...) {
				databaseTool = databaseTool || name == allowed
			}
			if databaseTool {
				continue
			}
			if name == "workspace_list" || name == "workspace_open" || name == "workspace_select" {
				continue
			}
			if name == "terminal_context" || name == "opscopilot_terminal_context" || name == "opscopilot__terminal_context" {
				toolName = name
			}
			if name != "ask_user" && name != "terminal_context" && name != "terminal_execute" && name != "terminal_take" && name != "terminal_release" && name != "opscopilot_terminal_context" && name != "opscopilot_terminal_execute" && name != "opscopilot__terminal_context" && name != "opscopilot__terminal_execute" && name != "compress_context" && name != "recall_context" && name != "list_compressed_contexts" && name != "load_skill" && name != "read_skill_resource" && name != "run_skill_script" {
				t.Errorf("unexpected bypass tool %q", name)
			}
		}
		message := map[string]any{"role": "assistant", "content": "MANAGED_CHAIN_OK"}
		reason := "stop"
		if serveReviewFixture(w, request) {
			return
		}
		step := calls.Add(1)
		if step == 1 {
			if toolName == "" {
				t.Error("missing host context tool")
			}
			message = map[string]any{"role": "assistant", "content": nil, "tool_calls": []any{map[string]any{"id": "fixture-1", "type": "function", "function": map[string]string{"name": toolName, "arguments": "{}"}}}}
			reason = "tool_calls"
		}
		if step == 2 {
			message = map[string]any{"role": "assistant", "content": nil, "tool_calls": []any{map[string]any{"id": "fixture-question", "type": "function", "function": map[string]string{"name": "ask_user", "arguments": `{"questions":[{"question":"Which day?","header":"Day","options":[{"label":"Today","description":"Today's logs"},{"label":"Yesterday","description":"Yesterday's logs"}]}]}`}}}}
			reason = "tool_calls"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "created": 1, "model": "fixture", "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": reason}}, "usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 10, "total_tokens": 20}})
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()
	coordinator := &assistantcontrol.Coordinator{}
	turn, _ := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	tools := &managedFixtureTools{}
	answered := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				answered <- ctx.Err()
				return
			case <-ticker.C:
				state := coordinator.Snapshot()
				if state.Prompt != nil {
					answered <- coordinator.Respond(state.TurnID, state.Prompt.ID, assistantcontrol.Answer{Selected: []string{"0"}})
					return
				}
			}
		}
	}()
	err := RunManaged(ctx, ManagedConfig{Executable: executable, BaseURL: server.URL + "/v1", APIKey: key, Model: "fixture", ParentDirectory: t.TempDir()}, turn, tools, "Inspect the current terminal using terminal_context, then respond.")
	if err != nil {
		var rpc *RPCError
		if errors.As(err, &rpc) {
			t.Logf("fixture-only RPC diagnostic: code=%d data=%s", rpc.Code, rpc.Data)
		}
		t.Fatal(err)
	}
	if tools.calls.Load() != 1 || calls.Load() != 3 || turn.Snapshot().Output != "MANAGED_CHAIN_OK" {
		t.Fatalf("calls=%d model=%d state=%+v", tools.calls.Load(), calls.Load(), turn.Snapshot())
	}
	select {
	case err := <-answered:
		if err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("real Chrys question never reached host UI")
	}
}
