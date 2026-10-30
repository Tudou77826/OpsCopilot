package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type stdioFixture struct{}

func (stdioFixture) Call(_ context.Context, name, _ string) (any, error) {
	return "fixture:" + name, nil
}

func TestStdioBridgeListsAndCallsTools(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bridge, err := StartStdioBridge(ctx, stdioFixture{})
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.Close()
	for _, message := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"workspace_list","arguments":{}}}`,
	} {
		var output bytes.Buffer
		if err := RunStdioProxy(ctx, strings.NewReader(message+"\n"), &output, bridge.Address, bridge.Token); err != nil {
			t.Fatal(err)
		}
		var reply map[string]any
		if json.Unmarshal(output.Bytes(), &reply) != nil || reply["result"] == nil {
			t.Fatalf("invalid MCP response: %s", output.String())
		}
		if strings.Contains(message, "tools/call") && !strings.Contains(output.String(), "fixture:workspace_list") {
			t.Fatal("tool was not dispatched")
		}
	}
}
