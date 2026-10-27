package assistanttools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/dbcontrol"
	"opscopilot/pkg/dbworkspace"
	"opscopilot/pkg/terminalcontrol"
	"opscopilot/pkg/workspace"
)

func lowRiskFixture(context.Context, assistantcontrol.ReviewInput) assistantcontrol.ReviewVerdict {
	return assistantcontrol.ReviewVerdict{LowRisk: true, InScope: true, Reason: "fixture: within the requested investigation"}
}

func TestRequestedWorkspaceOpenAndReadNeedNoHumanPrompt(t *testing.T) {
	c := &assistantcontrol.Coordinator{}
	turn, _ := c.Begin(context.Background(), assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	observed := 0
	w := NewWorkspaceTools(turn, terminalcontrol.Target{}, nil, func(context.Context, string) (ConnectionPlan, error) {
		return ConnectionPlan{Connection: workspace.Connection{Name: "requested", Route: []workspace.Endpoint{{Host: "requested-node", Port: 22, User: "operator"}}}, Open: func(context.Context) (workspace.Tab, error) {
			return workspace.Tab{Target: workspace.Target{TabID: "tab", ConnectionID: "physical"}}, nil
		}}, nil
	}, nil, func(target terminalcontrol.Target) *TerminalTools {
		return &TerminalTools{Turn: turn, Target: target, Review: lowRiskFixture, Observe: func(context.Context) (terminalcontrol.TerminalContext, error) {
			observed++
			return terminalcontrol.TerminalContext{}, nil
		}}
	})
	w.Review = lowRiskFixture
	defer w.Close()
	ctx, cancel := context.WithTimeout(turn.Context(), time.Second)
	defer cancel()
	if _, err := w.Call(ctx, "workspace_open", "saved"); err != nil {
		t.Fatal("open asked human or failed", err)
	}
	if _, err := w.Call(ctx, "terminal_context", ""); err != nil {
		t.Fatal("read asked human or failed", err)
	}
	if c.Snapshot().Prompt != nil || observed != 1 {
		t.Fatal("unnecessary human interaction", observed)
	}
}

func TestDatabaseInvestigationAutoContinuesButMutationStillWaits(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c := &assistantcontrol.Coordinator{}
	turn, _ := c.Begin(ctx, assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	db := &databaseToolFixture{}
	service := dbcontrol.New(db, databaseJournalFixture{})
	acked := make(chan struct{})
	go func() {
		defer close(acked)
		for ctx.Err() == nil {
			_, items := service.List(0)
			for _, w := range items {
				if w.Pending != "" {
					_ = service.Acknowledge(w.ID, w.Pending, w.Session.ID, w.SQL)
				}
			}
			time.Sleep(time.Millisecond)
		}
	}()
	defer func() { cancel(); <-acked }()
	w, err := service.Open(ctx, dbworkspace.Profile{ID: "profile", Name: "orders", Host: "business-db"})
	if err != nil {
		t.Fatal(err)
	}
	tools := &DatabaseTools{Turn: turn, Service: service, Review: lowRiskFixture}
	defer tools.Close()
	arg, _ := json.Marshal(map[string]string{"workspace_id": w.ID})
	if _, err = tools.Call(ctx, "database_take", string(arg)); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{"SELECT version()", "SELECT * FROM orders WHERE id IN (1, 2)"} {
		arg, _ = json.Marshal(map[string]string{"workspace_id": w.ID, "sql": sql, "purpose": "检查用户要求的数据库信息"})
		if _, err = tools.Call(ctx, "database_execute", string(arg)); err != nil {
			t.Fatal("read-only investigation required human", err)
		}
	}
	if db.queries.Load() != 2 || c.Snapshot().Prompt != nil {
		t.Fatal("queries did not automatically complete")
	}
	arg, _ = json.Marshal(map[string]string{"workspace_id": w.ID, "sql": "DELETE FROM orders", "purpose": "此删除必须等待人工"})
	done := make(chan error, 1)
	go func() { _, err := tools.Call(ctx, "database_execute", string(arg)); done <- err }()
	prompt := waitPrompt(t, c)
	if db.queries.Load() != 2 {
		t.Fatal("mutation executed before approval")
	}
	_ = c.Respond(turn.ID(), prompt.Prompt.ID, assistantcontrol.Answer{Selected: []string{"reject"}})
	if err = <-done; err == nil || db.queries.Load() != 2 {
		t.Fatal("denial executed mutation", err)
	}
}
