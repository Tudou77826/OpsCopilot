package mcp

import (
	"encoding/json"
	"testing"
)

func TestFileArgumentsRejectExtraAuthorityAndMalformedPaths(t *testing.T) {
	for _, in := range []string{`{"session_id":"ssh","approved":true}`, `{"session_id":null}`, `{"session_id":"ssh\ncommand"}`, `{"connection_id":"invented"}`} {
		var args map[string]json.RawMessage
		_ = json.Unmarshal([]byte(in), &args)
		if _, ok := parseFileArguments("file_open", args); ok {
			t.Fatal("accepted", in)
		}
	}
	for _, name := range FileToolNames() {
		fields := fileFields[name]
		args := map[string]json.RawMessage{}
		for _, f := range fields {
			args[f] = json.RawMessage(`"test"`)
		}
		if _, ok := parseFileArguments(name, args); !ok {
			t.Fatal("valid args rejected", name)
		}
		args["credentials"] = json.RawMessage(`"no"`)
		if _, ok := parseFileArguments(name, args); ok {
			t.Fatal("credential argument accepted", name)
		}
	}
}
