package assistanttools

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/dbcontrol"
	"opscopilot/pkg/dbworkspace"
)

type databaseToolFixture struct {
	dbcontrol.Database
	queries atomic.Int32
}

func (f *databaseToolFixture) ConnectRevision(context.Context, string, int) (dbworkspace.SessionInfo, error) {
	return dbworkspace.SessionInfo{ID: "physical", ProfileID: "profile"}, nil
}
func (f *databaseToolFixture) Session(string) (dbworkspace.SessionInfo, error) {
	return dbworkspace.SessionInfo{ID: "physical"}, nil
}
func (f *databaseToolFixture) SchemasContext(context.Context, string) ([]string, error) {
	return []string{"orders"}, nil
}
func (f *databaseToolFixture) Own(context.Context, string, string) (func(), error) {
	return func() {}, nil
}
func (f *databaseToolFixture) Dialect(string) (string, error) { return "mysql", nil }
func (f *databaseToolFixture) RunContext(context.Context, dbworkspace.QueryRequest) (dbworkspace.QueryResult, error) {
	f.queries.Add(1)
	return dbworkspace.QueryResult{Sets: []dbworkspace.ResultSet{}}, nil
}

type databaseJournalFixture struct{}

func (databaseJournalFixture) Append(context.Context, dbcontrol.Event) error { return nil }

func TestDatabaseWritesWaitForExactApprovalAndKeepOriginalOnRefusal(t *testing.T) {
	for _, answer := range []string{"allow", "reject", "stop"} {
		t.Run(answer, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			c := &assistantcontrol.Coordinator{}
			turn, _ := c.Begin(ctx, assistantcontrol.Target{}, nil)
			defer turn.Finish("completed", "")
			db := &databaseToolFixture{}
			service := dbcontrol.New(db, databaseJournalFixture{})
			go func() {
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
			w, err := service.Open(ctx, dbworkspace.Profile{ID: "profile", Name: "orders"})
			if err != nil {
				t.Fatal(err)
			}
			tools := &DatabaseTools{Turn: turn, Service: service}
			defer tools.Close()
			done := make(chan error, 1)
			argument, _ := json.Marshal(map[string]string{"workspace_id": w.ID})
			go func() { _, err := tools.Call(ctx, "database_take", string(argument)); done <- err }()
			prompt := waitPrompt(t, c)
			if err = c.Respond(turn.ID(), prompt.Prompt.ID, assistantcontrol.Answer{Selected: []string{"allow"}}); err != nil {
				t.Fatal(err)
			}
			if err = <-done; err != nil {
				t.Fatal(err)
			}
			const sql = "UPDATE orders SET status='review' WHERE id=17;"
			payload, _ := json.Marshal(map[string]string{"sql": sql, "purpose": "调整指定测试订单"})
			go func() { _, err := tools.Call(ctx, "database_execute", string(payload)); done <- err }()
			prompt = waitPrompt(t, c)
			snapshot, _ := service.Snapshot(w.ID)
			if prompt.Prompt.Command != sql || len(snapshot.Blocks) != 1 || snapshot.Blocks[0].SQL != sql || db.queries.Load() != 0 {
				t.Fatal("approval does not bind original block")
			}
			if answer == "stop" {
				_ = c.Stop(turn.ID())
			} else if err = c.Respond(turn.ID(), prompt.Prompt.ID, assistantcontrol.Answer{Selected: []string{answer}}); err != nil {
				t.Fatal(err)
			}
			err = <-done
			snapshot, _ = service.Snapshot(w.ID)
			if answer != "stop" {
				var receipt assistantcontrol.Operation
				for _, message := range c.Snapshot().Messages {
					if message.Operation.ItemID == snapshot.Blocks[0].ID {
						receipt = message.Operation
					}
				}
				want := "completed"
				if answer == "reject" {
					want = "not_executed"
				}
				if receipt.Status != want {
					t.Fatal("wrong SQL receipt", receipt)
				}
			}
			if answer == "allow" {
				if err != nil || db.queries.Load() != 1 {
					t.Fatal(err, db.queries.Load())
				}
			} else if err == nil || db.queries.Load() != 0 || snapshot.Blocks[0].Status != "not_executed" {
				t.Fatal("denied/stopped SQL dispatched or disappeared", err, snapshot.Blocks)
			}
		})
	}
}
