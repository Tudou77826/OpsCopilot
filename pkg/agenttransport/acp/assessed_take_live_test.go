package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"opscopilot/pkg/assistantcontrol"
)

type assessmentFixture struct {
	mu    sync.Mutex
	calls []string
}

func (f *assessmentFixture) Call(_ context.Context, name, argument string) (any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, name+":"+argument)
	if name == "terminal_context" {
		return map[string]any{"output": map[string]string{"text": "user@fixture:~$ "}}, nil
	}
	return map[string]bool{"owned": true}, nil
}

// Real iCode and MCP transport with a deterministic local model and fake host;
// no user terminal or external model service is accessed.
func TestLiveICodeAssessedTakeTransport(t *testing.T) {
	executable := os.Getenv("OPSCOPILOT_CHRYS_TEST_EXE")
	if executable == "" {
		t.Skip("opt-in local iCode")
	}
	var steps atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Tools []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			w.WriteHeader(400)
			return
		}
		step := steps.Add(1)
		message := map[string]any{"role": "assistant", "content": "完成"}
		finish := "stop"
		if step <= 2 {
			target := "terminal_context"
			arguments := `{}`
			if step == 2 {
				target = "terminal_take"
				arguments = `{"shell_state":"idle_shell"}`
			}
			name := ""
			for _, tool := range request.Tools {
				if tool.Function.Name == target || strings.HasSuffix(tool.Function.Name, "_"+target) {
					name = tool.Function.Name
					break
				}
			}
			if name == "" {
				t.Error("missing tool", target)
				w.WriteHeader(500)
				return
			}
			message = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": fmt.Sprintf("call-%d", step), "type": "function", "function": map[string]string{"name": name, "arguments": arguments}}}}
			finish = "tool_calls"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}}})
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	coordinator := &assistantcontrol.Coordinator{}
	turn, _ := coordinator.Begin(ctx, assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	fixture := &assessmentFixture{}
	c := &Conversation{}
	defer c.Close()
	config := ManagedConfig{Executable: executable, BaseURL: server.URL + "/v1", APIKey: "fixture", Model: "fixture", ApprovalMode: "bypass", ParentDirectory: t.TempDir()}
	if err := c.Run(ctx, config, turn, fixture, "先读取终端，然后判断是否可以接管"); err != nil {
		t.Fatal(err)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if len(fixture.calls) != 2 || fixture.calls[0] != "terminal_context:" || fixture.calls[1] != "terminal_take:idle_shell" {
		t.Fatal(fixture.calls)
	}
}
