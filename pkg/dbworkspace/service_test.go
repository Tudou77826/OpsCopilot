package dbworkspace

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"opscopilot/pkg/secretstore"
)

// SSH channels implement net.Conn but explicitly reject deadline methods.
type deadlineUnsupportedConn struct{ net.Conn }

func (deadlineUnsupportedConn) SetDeadline(time.Time) error {
	return fmt.Errorf("SSH channel deadlines unsupported")
}
func (deadlineUnsupportedConn) SetReadDeadline(time.Time) error {
	return fmt.Errorf("SSH channel deadlines unsupported")
}
func (deadlineUnsupportedConn) SetWriteDeadline(time.Time) error {
	return fmt.Errorf("SSH channel deadlines unsupported")
}

func TestMySQLChannelTransportIntegration(t *testing.T) {
	portText := os.Getenv("OPSCOPILOT_TEST_MYSQL_PORT")
	if portText == "" {
		t.Skip("set OPSCOPILOT_TEST_MYSQL_PORT for real MySQL integration")
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dial := func(ctx context.Context, addr string) (net.Conn, error) {
		conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
		if err != nil {
			return nil, err
		}
		return deadlineUnsupportedConn{conn}, nil
	}
	db, cleanup, err := (MySQL{}).Open(ctx, Profile{Engine: "mysql", Host: "127.0.0.1", Port: port, User: "root"}, os.Getenv("OPSCOPILOT_TEST_MYSQL_PASSWORD"), dial)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	defer db.Close()
	var one int
	if err = db.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil || one != 1 {
		t.Fatalf("channel query failed: %v", err)
	}
	timeout, stop := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer stop()
	start := time.Now()
	err = db.QueryRowContext(timeout, "SELECT SLEEP(3)").Scan(&one)
	if err == nil || time.Since(start) > 2*time.Second {
		t.Fatalf("channel ignored cancellation: %v", err)
	}
}

type memorySecrets struct {
	mu     sync.Mutex
	values map[string]string
}

func (m *memorySecrets) Set(_, key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[key] = value
	return nil
}
func (m *memorySecrets) Get(_, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.values[key]
	if !ok {
		return "", secretstore.ErrNotFound
	}
	return v, nil
}
func (m *memorySecrets) Delete(_, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.values, key)
	return nil
}

func TestProfileSecretsAndConcurrentEdits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database.json")
	secrets := &memorySecrets{values: map[string]string{}}
	s := newNativeTestService(path, secrets)
	password := "private-db-password"
	p, err := s.Save(ProfileInput{Profile: Profile{Engine: "mysql", Name: "test", Host: "localhost", User: "tester", ReadOnly: true}, Password: &password})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), password) {
		t.Fatal("password leaked to disk")
	}
	public, _ := json.Marshal(p)
	if strings.Contains(string(public), "secretId") || strings.Contains(string(public), password) {
		t.Fatal("credential reference leaked to frontend")
	}
	other := newNativeTestService(path, secrets)
	p.Name = "updated"
	p.Group = "订单系统"
	updated, err := other.Save(ProfileInput{Profile: p})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.PasswordSet || updated.Group != "订单系统" {
		t.Fatal("editing cleared password")
	}
	_, value, err := s.profile(p.ID)
	if err != nil || value != password {
		t.Fatalf("credential not preserved: %v", err)
	}
	if _, err = s.Save(ProfileInput{Profile: p}); err == nil {
		t.Fatal("stale revision accepted")
	}
	if err = s.Delete(p.ID, p.Revision); err == nil {
		t.Fatal("stale delete accepted")
	}
	empty := ""
	updated, err = s.Save(ProfileInput{Profile: updated, Password: &empty})
	if err != nil {
		t.Fatal(err)
	}
	if updated.PasswordSet || len(secrets.values) != 0 {
		t.Fatal("credential not cleared")
	}
	if err = s.Delete(updated.ID, updated.Revision); err != nil {
		t.Fatal(err)
	}
	list, _ := other.List()
	if len(list) != 0 {
		t.Fatal("cross-process delete not visible")
	}
	os.WriteFile(path, []byte("broken"), 0600)
	if _, err = s.Save(ProfileInput{Profile: Profile{Engine: "mysql", Name: "x", Host: "localhost", User: "u"}}); err == nil {
		t.Fatal("overwrote corrupt store")
	}
}

func TestAutomaticProfileName(t *testing.T) {
	s := newNativeTestService(filepath.Join(t.TempDir(), "profiles.json"), &memorySecrets{values: map[string]string{}})
	p, err := s.Save(ProfileInput{Profile: Profile{Engine: "mysql", Host: "db.internal", Port: 3306, User: "ops", Database: "orders", ReadOnly: true}})
	if err != nil || p.Name != "orders @ db.internal:3306" || p.Environment != "" || !p.ReadOnly {
		t.Fatalf("automatic connection defaults: %#v %v", p, err)
	}
}

func TestSplitMySQLStatements(t *testing.T) {
	input := "-- heading;\nSELECT 'a;b', `semi;colon`; /* ; */ SELECT 'it''s;fine', \"x;z\"; # tail;\nSELECT 3;"
	statements, err := SplitSQL(input)
	if err != nil || len(statements) != 3 {
		t.Fatalf("split: %#v %v", statements, err)
	}
	for _, bad := range []string{"SELECT 'broken", "/* open", "DELIMITER\nSELECT 1", "/*!50000 DROP TABLE x */", "-- only comments"} {
		if _, err = SplitSQL(bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
	if !returnsRows("/* comment */\nSELECT 1") || returnsRows("# hello\nDELETE FROM x") {
		t.Fatal("classification failed")
	}
	q := (MySQL{}).BrowseSQL("db`name", "a; DROP TABLE x", 100, 100)
	if q != "SELECT * FROM `db``name`.`a; DROP TABLE x` LIMIT 101 OFFSET 100" {
		t.Fatal(q)
	}
}

// This integration test requires an explicitly provided isolated MySQL endpoint.
// It creates and drops only a uniquely named database it owns.
func TestMySQLIntegration(t *testing.T) {
	portText := os.Getenv("OPSCOPILOT_TEST_MYSQL_PORT")
	if portText == "" {
		t.Skip("set OPSCOPILOT_TEST_MYSQL_PORT for real MySQL integration")
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	password := os.Getenv("OPSCOPILOT_TEST_MYSQL_PASSWORD")
	secrets := &memorySecrets{values: map[string]string{}}
	s := newNativeTestService(filepath.Join(t.TempDir(), "profiles.json"), secrets)
	defer s.Close()
	p, err := s.Save(ProfileInput{Profile: Profile{Name: "integration", Host: "127.0.0.1", Port: port, User: "root", Engine: "mysql"}, Password: &password})
	if err != nil {
		t.Fatal(err)
	}
	info, err := s.Connect(context.Background(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(s.path)
	secretCount := len(secrets.values)
	draft := p
	draft.Name = "unsaved draft"
	version, testErr := s.Test(context.Background(), ProfileInput{Profile: draft})
	if testErr != nil || version == "" {
		t.Fatalf("test saved credentials: %q %v", version, testErr)
	}
	draft.ID, draft.Revision, draft.Name = "", 0, ""
	if _, testErr = s.Test(context.Background(), ProfileInput{Profile: draft, Password: &password}); testErr != nil {
		t.Fatalf("test unsaved connection: %v", testErr)
	}
	badPassword := "wrong-test-password"
	if _, testErr = s.Test(context.Background(), ProfileInput{Profile: p, Password: &badPassword}); testErr == nil {
		t.Fatal("test ignored draft password")
	}
	after, _ := os.ReadFile(s.path)
	if string(before) != string(after) || len(secrets.values) != secretCount || len(s.sessions) != 1 || s.sessions[info.ID] == nil || s.opening != 0 {
		t.Fatal("test mutated profiles, credentials, or live sessions")
	}
	schema := "ops_workbench_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	run := func(session, query string, limit int) QueryResult {
		t.Helper()
		r, e := s.Run(QueryRequest{SessionID: session, RequestID: uuid.NewString(), SQL: query, Limit: limit, TimeoutSeconds: 10})
		if e != nil || r.Error != "" {
			t.Fatalf("query failed %s: %v %s", query, e, r.Error)
		}
		return r
	}
	run(info.ID, "CREATE DATABASE "+quoteIdentifier(schema), 100)
	defer func() { run(info.ID, "DROP DATABASE "+quoteIdentifier(schema), 100) }()
	qname := qualified(schema, "events")
	run(info.ID, "CREATE TABLE "+qname+" (id BIGINT PRIMARY KEY, amount DECIMAL(30,10), note TEXT NULL, payload TEXT); INSERT INTO "+qname+" VALUES (9007199254740993,12345678901234567890.1234567890,NULL,'line1\\nline2'),(2,1.5,'','a;b')", 100)
	result := run(info.ID, "SELECT id,amount,note,payload FROM "+qname+" ORDER BY id DESC", 100)
	row := result.Sets[0].Rows[0]
	if *row[0] != "9007199254740993" || *row[1] != "12345678901234567890.1234567890" || row[2] != nil {
		t.Fatalf("lossy cells: %#v", row)
	}
	if !run(info.ID, "SELECT * FROM "+qname, 1).Sets[0].Truncated {
		t.Fatal("row limit not enforced")
	}
	partial, e := s.Run(QueryRequest{SessionID: info.ID, RequestID: uuid.NewString(), SQL: "SELECT 1; SELECT invalid_column FROM " + qname})
	if e != nil || len(partial.Sets) != 1 || partial.Error == "" {
		t.Fatalf("partial result lost: %#v %v", partial, e)
	}
	tables, e := s.Tables(info.ID, schema)
	if e != nil || len(tables) != 1 {
		t.Fatalf("tables: %v %v", tables, e)
	}
	structure, e := s.Structure(TableRequest{SessionID: info.ID, Database: schema, Table: "events"})
	if e != nil || len(structure.Columns.Rows) != 4 || structure.DDL == "" {
		t.Fatalf("structure: %#v %v", structure, e)
	}
	for _, kind := range []string{"select", "insert", "update", "delete", "name", "qualified-name"} {
		text, err := s.GenerateSQL(SQLTemplateRequest{SessionID: info.ID, Database: schema, Table: "events", Kind: kind})
		if err != nil || text == "" {
			t.Fatalf("generate %s: %q %v", kind, text, err)
		}
		if (kind == "update" || kind == "delete") && !strings.Contains(text, "WHERE 1 = 0") {
			t.Fatalf("write template lacks explicit target placeholder: %s", text)
		}
		if kind == "select" && !strings.Contains(text, "`amount`") {
			t.Fatalf("select template lost fields: %s", text)
		}
	}
	if _, err = s.GenerateSQL(SQLTemplateRequest{SessionID: info.ID, Database: schema, Table: "events", Kind: "drop"}); err == nil {
		t.Fatal("unsupported template accepted")
	}
	quoted, err := s.GenerateSQL(SQLTemplateRequest{SessionID: info.ID, Database: "a`b", Table: "t`x", Column: "c`d", Kind: "qualified-name"})
	if err != nil || quoted != "`a``b`.`t``x`.`c``d`" {
		t.Fatalf("identifier escaping: %q %v", quoted, err)
	}
	if len(run(info.ID, "SELECT * FROM "+qname, 100).Sets[0].Rows) != 2 {
		t.Fatal("generating SQL changed table data")
	}
	readOnly := p
	readOnly.ID = ""
	readOnly.Revision = 0
	readOnly.Name = "readonly"
	readOnly.ReadOnly = true
	ro, e := s.Save(ProfileInput{Profile: readOnly, Password: &password})
	if e != nil {
		t.Fatal(e)
	}
	rsession, e := s.Connect(context.Background(), ro.ID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Run(QueryRequest{SessionID: rsession.ID, RequestID: uuid.NewString(), SQL: "DELETE FROM " + qname}); e == nil {
		t.Fatal("readonly accepted write")
	}
	run(rsession.ID, "SELECT * FROM "+qname, 10)
	cancelSession, e := s.Connect(context.Background(), p.ID)
	if e != nil {
		t.Fatal(e)
	}
	s.mu.Lock()
	cancelledServerID := s.sessions[cancelSession.ID].serverID
	s.mu.Unlock()
	requestID := uuid.NewString()
	done := make(chan QueryResult, 1)
	go func() {
		r, _ := s.Run(QueryRequest{SessionID: cancelSession.ID, RequestID: requestID, SQL: "SELECT SLEEP(20)", TimeoutSeconds: 30})
		done <- r
	}()
	deadline := time.Now().Add(5 * time.Second)
	for !s.Cancel(requestID) {
		if time.Now().After(deadline) {
			t.Fatal("request did not register")
		}
		time.Sleep(10 * time.Millisecond)
	}
	select {
	case cancelled := <-done:
		if !cancelled.Cancelled || !cancelled.Disconnected {
			t.Fatalf("cancel not reported: %#v", cancelled)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("cancel hung")
	}
	if _, _, e = s.acquire(cancelSession.ID); e == nil {
		t.Fatal("cancelled connection reusable")
	}
	remaining := run(info.ID, "SELECT COUNT(*) FROM information_schema.PROCESSLIST WHERE ID="+cancelledServerID+" AND COMMAND='Query'", 1)
	if *remaining.Sets[0].Rows[0][0] != "0" {
		t.Fatal("server query remained active after cancellation")
	}
}
