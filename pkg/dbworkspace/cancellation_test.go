package dbworkspace

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

type idleSQLConnector struct{}

func TestAlreadyCancelledJDBCCallNeverTouchesTransport(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// No helper or pipes exist: a cancelled operation must return before
	// allocating a request or trying to send a SQL frame.
	if err := (&jdbcProcess{}).call(ctx, map[string]any{"op": "query", "sql": "SELECT 1"}, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func (idleSQLConnector) Connect(context.Context) (driver.Conn, error) { return idleSQLConn{}, nil }
func (idleSQLConnector) Driver() driver.Driver                        { return idleSQLDriver{} }

type idleSQLDriver struct{}

func (idleSQLDriver) Open(string) (driver.Conn, error) { return idleSQLConn{}, nil }

type idleSQLConn struct{}

func (idleSQLConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (idleSQLConn) Close() error                        { return nil }
func (idleSQLConn) Begin() (driver.Tx, error)           { return nil, errors.New("unused") }

type cancelledPreparation struct {
	Connector
	entered chan struct{}
}

func (c cancelledPreparation) SelectDatabase(ctx context.Context, _ *sql.Conn, _ string) error {
	close(c.entered)
	<-ctx.Done()
	return ctx.Err()
}
func (c cancelledPreparation) Schemas(ctx context.Context, _ *sql.Conn) ([]string, error) {
	close(c.entered)
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestCancellationDuringPreparationRetiresExactPhysicalSession(t *testing.T) {
	for _, stage := range []string{"database", "metadata"} {
		t.Run(stage, func(t *testing.T) {
			s := New("", nil)
			db := sql.OpenDB(idleSQLConnector{})
			conn, err := db.Conn(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			life, finish := context.WithCancel(context.Background())
			entered := make(chan struct{})
			var retired atomic.Int32
			v := &session{db: db, conn: conn, connector: cancelledPreparation{entered: entered}, ctx: life, cancel: finish, cleanup: func() { retired.Add(1) }, gate: make(chan struct{}, 1)}
			s.sessions["physical"] = v
			t.Cleanup(s.Close)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				if stage == "database" {
					result, err := s.RunContext(ctx, QueryRequest{SessionID: "physical", RequestID: uuid.NewString(), SQL: "SELECT 1", Database: "orders"})
					if err == nil && (!result.Cancelled || !result.Disconnected) {
						err = errors.New("early cancellation reported usable connection")
					}
					done <- err
				} else {
					_, err := s.SchemasContext(ctx, "physical")
					if !errors.Is(err, context.Canceled) {
						done <- errors.New("metadata cancellation lost")
					} else {
						done <- nil
					}
				}
			}()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("preparation never started")
			}
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("cancellation did not drain")
			}
			if _, err := s.Session("physical"); err == nil || retired.Load() != 1 {
				t.Fatal("dead session/helper remained registered")
			}
		})
	}
}
