package shellsidecar

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestQuickCommandReorderRPCPreservesOtherGroupsAndPersists(t *testing.T) {
	dir := t.TempDir()
	service, err := NewQuickCmdService(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []QuickCommand{
		{ID: "a", Name: "A", Content: "pwd", Group: "ops"},
		{ID: "b", Name: "B", Content: "date", Group: "other"},
		{ID: "c", Name: "C", Content: "ls", Group: "ops"},
	} {
		if _, err := service.Save(command); err != nil {
			t.Fatal(err)
		}
	}
	api := &ControlAPI{QuickCmds: service}
	_, rpcErr := api.dispatch(context.Background(), &rpcRequest{Method: "shell.quickcmds.reorder", Params: json.RawMessage(`{"ids":["c","missing","c","a"]}`)})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	restored, err := NewQuickCmdService(dir)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, command := range restored.List() {
		ids = append(ids, command.ID)
	}
	if !reflect.DeepEqual(ids, []string{"c", "b", "a"}) {
		t.Fatalf("unexpected order: %v", ids)
	}
	_, rpcErr = api.dispatch(context.Background(), &rpcRequest{Method: "shell.quickcmds.reorder", Params: json.RawMessage(`{}`)})
	if rpcErr == nil {
		t.Fatal("missing IDs should be rejected")
	}
}
