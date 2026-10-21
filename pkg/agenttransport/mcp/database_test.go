package mcp

import (
	"encoding/json"
	"testing"
)

func TestDatabaseArgumentsCannotSupplyCredentialsOrAuthority(t *testing.T) {
	for _, test := range []struct{ name, args string }{
		{"database_create", `{"password":"secret"}`}, {"database_create", `{"credential":"secret"}`},
		{"database_execute", `{"sql":"SELECT 1","purpose":"检查","session_id":"human-session"}`},
		{"database_take", `{"workspace_id":"tab","allow_write":true}`},
		{"database_execute", `{"sql":"SELECT 1"}`}, {"database_execute", `{"sql":null,"purpose":"检查"}`},
		{"database_context", `{"workspace_id":"arbitrary"}`},
	} {
		var args map[string]json.RawMessage
		_ = json.Unmarshal([]byte(test.args), &args)
		if _, valid := parseDatabaseArguments(test.name, args); valid {
			t.Fatal("unsafe transport accepted", test.name)
		}
	}
	var args map[string]json.RawMessage
	_ = json.Unmarshal([]byte(`{"sql":"SELECT 1","purpose":"核对连接"}`), &args)
	if _, valid := parseDatabaseArguments("database_execute", args); !valid {
		t.Fatal("normal SQL rejected")
	}
}
