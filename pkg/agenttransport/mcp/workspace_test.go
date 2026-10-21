package mcp

import (
	"context"
	"testing"
)

func TestWorkspaceArgumentsRejectCredentialsAndExtraTargets(t *testing.T) {
	f := &fixtureCaller{}
	server, err := Start(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	for _, params := range []string{
		`{"name":"workspace_open","arguments":{"connection_id":"saved","password":"secret"}}`,
		`{"name":"workspace_open","arguments":{"host":"host"}}`,
		`{"name":"workspace_select","arguments":{"tab_id":"tab","connectionId":"replacement"}}`,
		`{"name":"workspace_list","arguments":{"include_passwords":true}}`,
	} {
		_, response := requestTo(t, server, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":`+params+`}`, "Bearer "+server.Token, "")
		if response["error"] == nil {
			t.Fatal("invalid workspace request accepted")
		}
	}
	if f.calls.Load() != 0 {
		t.Fatal("invalid arguments reached host")
	}
	_, response := requestTo(t, server, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"workspace_open","arguments":{"connection_id":"saved"}}}`, "Bearer "+server.Token, "")
	if response["error"] != nil || f.calls.Load() != 1 {
		t.Fatal("valid saved connection id rejected")
	}
}
