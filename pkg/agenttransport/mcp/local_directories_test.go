package mcp

import (
	"encoding/json"
	"testing"
)

func TestLocalDirectoryArgumentsCannotSelfApprove(t *testing.T) {
	for _, in := range []string{`{"project_id":"a","paths":["C:\\work"],"purpose":"inspect"}`, `{"project_id":"b","paths":["/one","/two"],"purpose":"inspect"}`} {
		var args map[string]json.RawMessage
		_ = json.Unmarshal([]byte(in), &args)
		if _, ok := parseLocalDirectoryArguments("local_add_directories", args); !ok {
			t.Fatal("valid rejected", in)
		}
		args["approved"] = json.RawMessage(`true`)
		if _, ok := parseLocalDirectoryArguments("local_add_directories", args); ok {
			t.Fatal("self authorization accepted")
		}
	}
	for _, in := range []string{`{"project_id":"a","paths":[],"purpose":"x"}`, `{"project_id":"a","paths":[null],"purpose":"x"}`, `{"project_id":"a","paths":["/one\ncommand"],"purpose":"x"}`, `{"project_id":"a","paths":["/one"],"purpose":null}`, `{"paths":["/one"],"purpose":"no project"}`} {
		var args map[string]json.RawMessage
		_ = json.Unmarshal([]byte(in), &args)
		if _, ok := parseLocalDirectoryArguments("local_add_directories", args); ok {
			t.Fatal("invalid accepted", in)
		}
	}
}
