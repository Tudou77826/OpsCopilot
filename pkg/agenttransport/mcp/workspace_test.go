package mcp

import (
	"context"
	"slices"
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
		`{"name":"workspace_create","arguments":{"host":"host","password":"secret"}}`,
		`{"name":"workspace_create","arguments":{"host":"root:secret@host"}}`,
		`{"name":"workspace_create","arguments":{"host":"host:2222"}}`,
		`{"name":"workspace_create","arguments":{"host":"host","port":0}}`,
		`{"name":"workspace_create","arguments":{"host":"host","port":65536}}`,
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
	_, response = requestTo(t, server, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"workspace_create","arguments":{"host":"2001:db8::1","port":2222,"user":"ops"}}}`, "Bearer "+server.Token, "")
	if response["error"] != nil || f.calls.Load() != 2 {
		t.Fatal("valid unsaved SSH draft rejected", response)
	}
}

func TestToolCatalogMatchesAdvertisedTools(t *testing.T) {
	server, err := Start(context.Background(), &fixtureCaller{})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	_, response := requestTo(t, server, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, "Bearer "+server.Token, "")
	result := response["result"].(map[string]any)
	names := []string{}
	for _, value := range result["tools"].([]any) {
		names = append(names, value.(map[string]any)["name"].(string))
	}
	want := ToolNames()
	slices.Sort(names)
	slices.Sort(want)
	if !slices.Equal(names, want) {
		t.Fatalf("advertised %v, catalog %v", names, want)
	}
}
