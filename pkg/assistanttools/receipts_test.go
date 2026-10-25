package assistanttools

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"opscopilot/pkg/assistantcontrol"
	"opscopilot/pkg/dbcontrol"
	"opscopilot/pkg/dbworkspace"
	"opscopilot/pkg/filecontrol"
	"opscopilot/pkg/filetransfer"
)

type gatedReceiptBackend struct {
	fileBackendFixture
	started, finish chan struct{}
}

func (b *gatedReceiptBackend) Transfer(ctx context.Context, _ filecontrol.Endpoint, _ filecontrol.Task, _ func(filetransfer.Progress)) (filetransfer.TransferResult, error) {
	close(b.started)
	select {
	case <-ctx.Done():
		return filetransfer.TransferResult{}, ctx.Err()
	case <-b.finish:
		return filetransfer.TransferResult{Bytes: 7}, nil
	}
}

type receiptFileJournal struct{ fail bool }

func (j receiptFileJournal) Append(_ context.Context, e filecontrol.Event) error {
	if j.fail && e.Action == "transfer_result" {
		return errors.New("audit disk full")
	}
	return nil
}
func TestFileReceiptRequiresActualCompletionAndKeepsSuccessWhenAuditFails(t *testing.T) {
	for _, auditFail := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "audit-failed"}[auditFail], func(t *testing.T) {
			c := &assistantcontrol.Coordinator{}
			turn, _ := c.Begin(context.Background(), assistantcontrol.Target{}, nil)
			defer turn.Finish("completed", "")
			backend := &gatedReceiptBackend{started: make(chan struct{}), finish: make(chan struct{})}
			s := filecontrol.New(backend, receiptFileJournal{fail: auditFail})
			w, err := s.Open(turn.Context(), filecontrol.Endpoint{SessionID: "ssh", ConnectionID: "physical"})
			if err != nil {
				t.Fatal(err)
			}
			if err = s.Take(turn.Context(), w.ID, turn.ID()); err != nil {
				t.Fatal(err)
			}
			task, err := s.Start(turn.Context(), w.ID, turn.ID(), filecontrol.Task{Direction: "download", LocalPath: "sample.log", RemotePath: "/sample.log"})
			if err != nil {
				t.Fatal(err)
			}
			<-backend.started
			f := &FileTools{Turn: turn, Service: s, current: w.ID, tasks: map[string]string{task.ID: w.ID}}
			defer f.Close()
			f.observeTasks(w.ID)
			if c.Snapshot().Messages[0].Operation.Status != "running" {
				t.Fatal("started task incorrectly completed")
			}
			close(backend.finish)
			for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
				got, _ := s.Snapshot(w.ID)
				if got.Tasks[0].Status != "running" {
					break
				}
				time.Sleep(time.Millisecond)
			}
			f.observeTasks(w.ID)
			f.observeTasks(w.ID)
			got := c.Snapshot().Messages
			want := "done"
			if auditFail {
				want = "warning"
			}
			if len(got) != 1 || got[0].Operation.Status != want || !strings.Contains(got[0].Operation.Outcome, "7 字节") {
				t.Fatal(got)
			}
		})
	}
}

type receiptDBJournal struct{}

func (receiptDBJournal) Append(_ context.Context, e dbcontrol.Event) error {
	if e.Phase == "completed" {
		return errors.New("audit disk full")
	}
	return nil
}
func TestSQLReceiptKeepsObservedExecutionWhenFinalAuditFails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c := &assistantcontrol.Coordinator{}
	turn, _ := c.Begin(ctx, assistantcontrol.Target{}, nil)
	db := &databaseToolFixture{}
	s := dbcontrol.New(db, receiptDBJournal{})
	go func() {
		for ctx.Err() == nil {
			_, items := s.List(0)
			for _, w := range items {
				if w.Pending != "" {
					_ = s.Acknowledge(w.ID, w.Pending, w.Session.ID, w.SQL)
				}
			}
			time.Sleep(time.Millisecond)
		}
	}()
	w, err := s.Open(ctx, dbworkspace.Profile{ID: "profile"})
	if err != nil {
		t.Fatal(err)
	}
	lease, err := s.Take(ctx, w.ID, turn.ID())
	if err != nil {
		t.Fatal(err)
	}
	d := &DatabaseTools{Turn: turn, Service: s, current: w.ID, lease: lease}
	defer d.Close()
	_, err = d.Call(ctx, "database_execute", `{"sql":"select 1","purpose":"确认连接"}`)
	if !errors.Is(err, dbcontrol.ErrAudit) || db.queries.Load() != 1 {
		t.Fatal(err, db.queries.Load())
	}
	got := c.Snapshot().Messages[0].Operation
	if got.Status != "warning" || !strings.Contains(got.Outcome, "已收到数据库执行结果") || !strings.Contains(got.Next, "不要直接重复执行") {
		t.Fatal(got)
	}
}

func TestDatabasePreviewIncludesValuesAndKeepsCredentialColumnsHidden(t *testing.T) {
	count, secret := "12", "never-display-password"
	result := dbworkspace.QueryResult{Sets: []dbworkspace.ResultSet{{Columns: []dbworkspace.Column{{Name: "timeout_count"}, {Name: "password"}, {Name: "optional"}}, Rows: [][]*string{{&count, &secret, nil}}}}, DurationMS: 18}
	preview := databaseResultPreview(result)
	if !strings.Contains(preview, "12 | [已隐藏] | NULL") || !strings.Contains(preview, "18 ms") || strings.Contains(preview, secret) {
		t.Fatal(preview)
	}
}
