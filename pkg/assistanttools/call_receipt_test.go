package assistanttools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"opscopilot/pkg/agenttransport/mcp"
	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/terminalcontrol"
)

func TestEveryApplicationToolHasReceiptCoverage(t *testing.T) {
	names := []string{"workspace_list", "workspace_open", "workspace_select", "terminal_context", "terminal_execute", "terminal_take", "terminal_release"}
	names = append(names, mcp.DatabaseToolNames()...)
	names = append(names, mcp.FileToolNames()...)
	names = append(names, mcp.LocalDirectoryToolNames()...)
	names = append(names, mcp.ReferenceToolNames()...)
	for _, name := range names {
		if !hasDetailedReceipt(name) && callTitles[name] == "" {
			t.Error("missing business receipt title", name)
		}
	}
}

func TestCommonReadReceiptsIncludeResultsAndDurationWithoutLeakingSecrets(t *testing.T) {
	r, _ := referenceFixture(t)
	w := NewWorkspaceTools(r.Turn, terminalcontrol.Target{}, nil, nil, nil, func(target terminalcontrol.Target) *TerminalTools {
		return &TerminalTools{Turn: r.Turn, Target: target}
	})
	w.References = r
	defer w.Close()
	for _, call := range []struct{ name, args string }{{"quick_command_list", "{}"}, {"quick_command_read", `{"kind":"terminal","id":"same"}`}, {"knowledge_list", "{}"}} {
		if _, err := w.Call(context.Background(), call.name, call.args); err != nil {
			t.Fatal(err)
		}
	}
	var receipts []assistantcontrol.Message
	for _, m := range r.Turn.Snapshot().Messages {
		if m.Kind == "operation" {
			receipts = append(receipts, m)
		}
	}
	if len(receipts) != 3 {
		t.Fatalf("missing or duplicated receipts: %d", len(receipts))
	}
	for _, m := range receipts {
		if m.Operation.Status != "completed" || !m.Operation.Measured || m.Operation.Action == "" || strings.Contains(m.Operation.Result, "hidden-secret") {
			t.Fatal("incorrect or unredacted receipt")
		}
	}
	if !strings.Contains(receipts[0].Operation.Result, "共 2 条") || !strings.Contains(receipts[1].Operation.Result, "tail -n 20") {
		t.Fatal("useful evidence missing")
	}
	r.Commands = func() ([]ReferenceCommand, error) { return nil, errors.New("source unavailable") }
	if _, err := w.Call(context.Background(), "quick_command_list", "{}"); err == nil {
		t.Fatal("source failure hidden")
	}
	last := r.Turn.Snapshot().Messages[len(r.Turn.Snapshot().Messages)-1]
	if last.Operation.Status != "failed" || !strings.Contains(last.Operation.Detail, "source unavailable") {
		t.Fatal(last)
	}
}

func TestCommonReceiptNeverDuplicatesDetailedCalls(t *testing.T) {
	r, _ := referenceFixture(t)
	w := &WorkspaceTools{turn: r.Turn}
	before := len(r.Turn.Snapshot().Messages)
	finish := w.beginCallReceipt("database_execute", `{"sql":"select 1"}`)
	r.Turn.ObserveOperation("sql-one", "SQL 完成", assistantcontrol.Operation{Kind: "database", Status: "completed", Action: "select 1"})
	finish(nil, nil)
	if len(r.Turn.Snapshot().Messages) != before+1 {
		t.Fatal("duplicate generic SQL receipt")
	}
	failed := w.beginCallReceipt("terminal_execute", "pwd")
	failed(nil, errors.New("connection unavailable"))
	lastFailed := r.Turn.Snapshot().Messages[len(r.Turn.Snapshot().Messages)-1]
	if lastFailed.Operation.Status != "failed" || lastFailed.Operation.Detail != "connection unavailable" {
		t.Fatal("missing pre-dispatch failure")
	}
	finish = w.beginCallReceipt("database_create", `{"host":"node","password":"do-not-display"}`)
	finish(map[string]string{"password": "do-not-display", "privateKey": "do-not-display"}, nil)
	last := r.Turn.Snapshot().Messages[len(r.Turn.Snapshot().Messages)-1]
	if strings.Contains(last.Operation.Action+last.Operation.Result, "do-not-display") {
		t.Fatal("connection form leaked")
	}
}
