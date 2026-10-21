package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type fixtureCaller struct {
	calls            atomic.Int32
	entered, release chan struct{}
}

func (f *fixtureCaller) Call(ctx context.Context, name, command string) (any, error) {
	f.calls.Add(1)
	if f.entered != nil {
		close(f.entered)
		select {
		case <-f.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return map[string]string{"submission": "submitted", "completion": "unknown"}, nil
}
func requestTo(t *testing.T, s *Server, body, token, origin string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest("POST", s.URL, strings.NewReader(body))
	req.Header.Set("Authorization", token)
	req.Header.Set("Origin", origin)
	client := http.Client{Timeout: time.Second}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	result := map[string]any{}
	_ = json.Unmarshal(data, &result)
	return response.StatusCode, result
}
func TestServerCapabilityAndStrictToolArguments(t *testing.T) {
	caller := &fixtureCaller{}
	server, err := Start(context.Background(), caller)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	call := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"terminal_execute","arguments":{"command":"pwd"}}}`
	for _, test := range []struct {
		body, token, origin string
		status              int
	}{
		{call, "", "", 401}, {call, "Bearer wrong", "", 401}, {call, "Bearer " + server.Token, "https://untrusted.example", 401},
		{`[{}]`, "Bearer " + server.Token, "", 400}, {call + call, "Bearer " + server.Token, "", 400},
	} {
		status, _ := requestTo(t, server, test.body, test.token, test.origin)
		if status != test.status {
			t.Fatal(status, test.status)
		}
	}
	for _, body := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"terminal_execute","arguments":{"command":"pwd","uid":0}}}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"terminal_context","arguments":{"target":"other"}}}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"shell","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"terminal_execute","arguments":{"command":"pwd","purpose":null}}}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"terminal_execute","arguments":{"command":"pwd","purpose":{},"approved":true}}}`,
	} {
		_, response := requestTo(t, server, body, "Bearer "+server.Token, "")
		if response["error"] == nil {
			t.Fatal(response)
		}
	}
	if caller.calls.Load() != 0 {
		t.Fatal("untrusted request reached application")
	}
	_, response := requestTo(t, server, call, "Bearer "+server.Token, "")
	if response["result"] == nil || caller.calls.Load() != 1 {
		t.Fatal(response)
	}
}

type explainedFixture struct {
	fixtureCaller
	purpose string
}

func (f *explainedFixture) CallWithPurpose(ctx context.Context, name, command, purpose string) (any, error) {
	f.purpose = purpose
	return f.Call(ctx, name, command)
}
func TestPurposeIsExplanatoryAndCannotCarryAuthority(t *testing.T) {
	f := &explainedFixture{}
	s, err := Start(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"terminal_execute","arguments":{"command":"pwd","purpose":"确认当前工作目录"}}}`
	_, response := requestTo(t, s, body, "Bearer "+s.Token, "")
	if response["result"] == nil || f.purpose != "确认当前工作目录" || f.calls.Load() != 1 {
		t.Fatal(response, f.purpose)
	}
	_, response = requestTo(t, s, strings.Replace(body, `"purpose":"确认当前工作目录"`, `"purpose":"确认当前工作目录","approved":true`, 1), "Bearer "+s.Token, "")
	if response["error"] == nil || f.calls.Load() != 1 {
		t.Fatal("model authority argument accepted", response)
	}
}
func TestServerRejectsConcurrentCommandsAndCancelsInFlight(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	caller := &fixtureCaller{entered: make(chan struct{}), release: make(chan struct{})}
	server, err := Start(ctx, caller)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	call := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"terminal_context","arguments":{}}}`
	done := make(chan struct{})
	go func() {
		defer close(done)
		request, _ := http.NewRequest("POST", server.URL, strings.NewReader(call))
		request.Header.Set("Authorization", "Bearer "+server.Token)
		response, _ := http.DefaultClient.Do(request)
		if response != nil {
			response.Body.Close()
		}
	}()
	select {
	case <-caller.entered:
	case <-time.After(time.Second):
		t.Fatal("not called")
	}
	_, response := requestTo(t, server, call, "Bearer "+server.Token, "")
	if response["error"] == nil || caller.calls.Load() != 1 {
		t.Fatal(response)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stopped turn retained HTTP request")
	}
}
