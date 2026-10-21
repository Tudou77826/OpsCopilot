package dbcontrol

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"opscopilot/pkg/dbworkspace"
)

type fakeDB struct {
	mu          sync.Mutex
	queries     []dbworkspace.QueryRequest
	closed      bool
	started     chan struct{}
	block       bool
	schemaError error
}

func (f *fakeDB) Connect(context.Context, string) (dbworkspace.SessionInfo, error) {
	return dbworkspace.SessionInfo{ID: "physical", ProfileID: "profile", Database: "orders"}, nil
}
func (f *fakeDB) ConnectRevision(ctx context.Context, id string, _ int) (dbworkspace.SessionInfo, error) {
	return f.Connect(ctx, id)
}
func (f *fakeDB) Disconnect(string) error                         { f.closed = true; return nil }
func (f *fakeDB) DisconnectContext(context.Context, string) error { return f.Disconnect("") }
func (f *fakeDB) Session(string) (dbworkspace.SessionInfo, error) {
	return dbworkspace.SessionInfo{ID: "physical"}, nil
}
func (f *fakeDB) Own(context.Context, string, string) (func(), error) { return func() {}, nil }
func (f *fakeDB) SchemasContext(context.Context, string) ([]string, error) {
	return []string{"orders"}, f.schemaError
}
func (f *fakeDB) List() ([]dbworkspace.Profile, error) { return nil, nil }
func (f *fakeDB) Dialect(string) (string, error)       { return "mysql", nil }
func (f *fakeDB) TablesContext(context.Context, string, string) ([]dbworkspace.Table, error) {
	return nil, nil
}
func (f *fakeDB) StructureContext(context.Context, dbworkspace.TableRequest) (dbworkspace.Structure, error) {
	return dbworkspace.Structure{}, nil
}
func (f *fakeDB) RunContext(ctx context.Context, req dbworkspace.QueryRequest) (dbworkspace.QueryResult, error) {
	f.mu.Lock()
	f.queries = append(f.queries, req)
	f.mu.Unlock()
	if f.started != nil {
		close(f.started)
	}
	if f.block {
		<-ctx.Done()
		return dbworkspace.QueryResult{Cancelled: true}, ctx.Err()
	}
	return dbworkspace.QueryResult{Sets: []dbworkspace.ResultSet{{Statement: req.SQL, Rows: [][]*string{}}}}, nil
}

type memoryJournal struct {
	events    []Event
	fail      bool
	failPhase string
}

func (j *memoryJournal) Append(_ context.Context, e Event) error {
	if j.fail || j.failPhase == e.Phase {
		return errors.New("disk full")
	}
	j.events = append(j.events, e)
	return nil
}
func presentAll(s *Service) func() {
	ctx, cancel := context.WithCancel(context.Background())
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
	return cancel
}
func openTest(t *testing.T) (*Service, *fakeDB, *memoryJournal, Workspace, context.Context) {
	t.Helper()
	db := &fakeDB{}
	journal := &memoryJournal{}
	s := New(db, journal)
	stop := presentAll(s)
	t.Cleanup(stop)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	t.Cleanup(cancel)
	w, err := s.Open(ctx, dbworkspace.Profile{ID: "profile", Name: "orders"})
	if err != nil {
		t.Fatal(err)
	}
	lease, err := s.Take(ctx, w.ID, "turn")
	if err != nil {
		t.Fatal(err)
	}
	return s, db, journal, w, lease
}
func TestSQLHistoryAppendsOriginalAndOnlyExecutesNewBlock(t *testing.T) {
	s, db, j, w, ctx := openTest(t)
	for _, sql := range []string{"SELECT 1;", "SELECT 2;"} {
		b, err := s.Append(ctx, w.ID, "turn", "检查目录\n补充说明", sql)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.Execute(ctx, w.ID, "turn", b, ""); err != nil {
			t.Fatal(err)
		}
	}
	now, _ := s.Snapshot(w.ID)
	if !strings.Contains(now.SQL, "SELECT 1;") || !strings.Contains(now.SQL, "SELECT 2;") || !strings.Contains(now.SQL, "-- 补充说明") || len(now.Blocks) != 2 {
		t.Fatal("history overwritten", now.SQL)
	}
	if len(db.queries) != 2 || db.queries[1].SQL != "SELECT 2;" || db.queries[1].Mode != "selection" || len(j.events) != 4 {
		t.Fatal("old SQL replayed or unaudited", db.queries)
	}
	s.Release(w.ID, "turn")
	now, _ = s.Snapshot(w.ID)
	if now.OwnerTurn != "" || !now.Connected || db.closed {
		t.Fatal("release destroyed user workspace")
	}
	if err := s.Edit(w.ID, now.SQL+"\nSELECT 3;"); err != nil {
		t.Fatal(err)
	}
}
func TestMissingReceiptOrAuditNeverExecutes(t *testing.T) {
	s, db, j, w, ctx := openTest(t)
	b, _ := s.Append(ctx, w.ID, "turn", "检查", "SELECT 1")
	j.fail = true
	if _, err := s.Execute(ctx, w.ID, "turn", b, ""); err == nil || len(db.queries) != 0 {
		t.Fatal("audit failure executed")
	}
	j.fail = false
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.Execute(cancelled, w.ID, "turn", b, ""); err == nil || len(db.queries) != 0 {
		t.Fatal("stopped turn executed")
	}
}
func TestStopCancelsRunningQueryButKeepsOriginalBlock(t *testing.T) {
	s, db, _, w, ctx := openTest(t)
	db.started = make(chan struct{})
	db.block = true
	b, _ := s.Append(ctx, w.ID, "turn", "等待查询", "SELECT 1")
	done := make(chan error, 1)
	go func() { _, err := s.Execute(ctx, w.ID, "turn", b, ""); done <- err }()
	<-db.started
	s.Release(w.ID, "turn")
	if err := <-done; err == nil {
		t.Fatal("query did not cancel")
	}
	now, _ := s.Snapshot(w.ID)
	if len(now.Blocks) != 1 || now.Blocks[0].SQL != "SELECT 1" || now.Blocks[0].Status != "cancelled" || now.OwnerTurn != "" {
		t.Fatal(now)
	}
}

func TestStoredBlockCannotBeChangedOrDispatchedTwice(t *testing.T) {
	s, db, _, w, ctx := openTest(t)
	b, _ := s.Append(ctx, w.ID, "turn", "核对结果", "SELECT 1")
	forged := b
	forged.SQL = "DELETE FROM orders"
	if _, err := s.Execute(ctx, w.ID, "turn", forged, ""); err == nil {
		t.Fatal("modified block admitted")
	}
	if _, err := s.Execute(ctx, w.ID, "turn", b, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Execute(ctx, w.ID, "turn", b, ""); err == nil || len(db.queries) != 1 {
		t.Fatal("block dispatched twice")
	}
	if _, err := s.Append(ctx, w.ID, "turn", "", "SELECT 2"); err == nil {
		t.Fatal("missing purpose admitted")
	}
}

func TestReleasedTurnCannotCloseAndDismissReclaimsDocument(t *testing.T) {
	s, db, _, w, ctx := openTest(t)
	_, _ = s.Append(ctx, w.ID, "turn", "检查", "SELECT 1")
	s.Release(w.ID, "turn")
	if err := s.Close(context.Background(), w.ID, "turn"); err == nil || db.closed {
		t.Fatal("released turn closed human session")
	}
	if err := s.Close(context.Background(), w.ID, ""); err != nil {
		t.Fatal(err)
	}
	before, _ := s.Snapshot(w.ID)
	if before.SQL == "" {
		t.Fatal("closing connection lost SQL")
	}
	if err := s.Dismiss(w.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Snapshot(w.ID); err == nil {
		t.Fatal("dismissed document retained")
	}
	_, items := s.List(0)
	if len(items) != 0 {
		t.Fatal("dismissed results still copied into polling")
	}
}

func TestOpenMetadataFailureDoesNotPublishOrLeakSession(t *testing.T) {
	db := &fakeDB{schemaError: errors.New("metadata unavailable")}
	s := New(db, &memoryJournal{})
	if _, err := s.Open(context.Background(), dbworkspace.Profile{ID: "profile"}); err == nil || !db.closed {
		t.Fatal("failed initialization left live session")
	}
	_, items := s.List(0)
	if len(items) != 0 {
		t.Fatal("failed initialization published workspace")
	}
}

func TestAuditFailureAfterExecutionKeepsKnownOutcomeAndBlocksReplay(t *testing.T) {
	s, db, j, w, ctx := openTest(t)
	b, _ := s.Append(ctx, w.ID, "turn", "检查结果", "SELECT 1")
	j.failPhase = "completed"
	result, err := s.Execute(ctx, w.ID, "turn", b, "")
	if !errors.Is(err, ErrAudit) || len(db.queries) != 1 {
		t.Fatal("result audit failure not reported", err)
	}
	s.SetBlock(w.ID, b.ID, "failed", dbworkspace.QueryResult{}, err.Error())
	now, _ := s.Snapshot(w.ID)
	if now.Blocks[0].Status != "completed" || len(now.Blocks[0].Result.Sets) != len(result.Sets) || !strings.Contains(now.Blocks[0].Error, "SQL 已执行") {
		t.Fatal("known execution outcome overwritten", now.Blocks[0])
	}
	if _, err = s.Execute(ctx, w.ID, "turn", b, ""); err == nil || len(db.queries) != 1 {
		t.Fatal("audit error replayed SQL")
	}
}

type delayedOwnDB struct {
	*fakeDB
	entered  chan struct{}
	resume   chan struct{}
	released chan struct{}
}

func (d *delayedOwnDB) Own(context.Context, string, string) (func(), error) {
	close(d.entered)
	<-d.resume
	return func() { close(d.released) }, nil
}

func TestCancelledTakeNeverPublishesOwnership(t *testing.T) {
	db := &delayedOwnDB{fakeDB: &fakeDB{}, entered: make(chan struct{}), resume: make(chan struct{}), released: make(chan struct{})}
	s := New(db, &memoryJournal{})
	stop := presentAll(s)
	defer stop()
	w, err := s.Open(context.Background(), dbworkspace.Profile{ID: "profile"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := s.Take(ctx, w.ID, "turn"); done <- err }()
	<-db.entered
	cancel()
	close(db.resume)
	if err := <-done; err == nil {
		t.Fatal("cancelled take succeeded")
	}
	select {
	case <-db.released:
	default:
		t.Fatal("late physical lease leaked")
	}
	now, _ := s.Snapshot(w.ID)
	if now.OwnerTurn != "" {
		t.Fatal("cancelled take published ownership")
	}
}
