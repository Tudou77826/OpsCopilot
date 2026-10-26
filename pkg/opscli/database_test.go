package opscli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"opscopilot/pkg/dbworkspace"
	"opscopilot/pkg/opssession"
)

func testDatabaseNode(engine, driver string) opssession.Node {
	return opssession.Node{Kind: "database", Database: &dbworkspace.Profile{Engine: engine, DriverID: driver, Host: "127.0.0.1", Port: 3306, User: "reader"}}
}

func TestDriverCommandRejectsInvalidDeploymentInputs(t *testing.T) {
	t.Setenv("OPSCOPILOT_JDBC_DRIVER_DIR", "")
	if _, err := driverCommand(context.Background(), []string{"list"}, nil); err == nil {
		t.Fatal("driver list accepted missing explicit driver directory")
	}
	t.Setenv("OPSCOPILOT_JDBC_DRIVER_DIR", t.TempDir())
	for _, args := range [][]string{{}, {"unknown"}, {"list", "extra"}, {"import"}} {
		if _, err := driverCommand(context.Background(), args, nil); err == nil {
			t.Errorf("driverCommand(%v) unexpectedly succeeded", args)
		}
	}
}

func TestQueryDatabaseRejectsUnsupportedRequestsBeforeEnvironment(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name   string
		node   opssession.Node
		action string
		limit  int
		secs   int
	}{
		{"action", testDatabaseNode("jdbc", "driver"), "drop", 10, 10},
		{"kind", opssession.Node{Kind: "ssh"}, "query", 10, 10},
		{"driver", testDatabaseNode("jdbc", ""), "query", 10, 10},
		{"engine", testDatabaseNode("mysql", "driver"), "query", 10, 10},
		{"limit", testDatabaseNode("jdbc", "driver"), "query", 0, 10},
		{"timeout", testDatabaseNode("jdbc", "driver"), "query", 10, 121},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := queryDatabase(ctx, nil, "session", tc.node, tc.action, "", "", "select 1", tc.limit, tc.secs); err == nil {
				t.Fatal("invalid database request unexpectedly succeeded")
			}
		})
	}
}

func TestQueryDatabaseRequiresSQLAndTable(t *testing.T) {
	node := testDatabaseNode("jdbc", "driver")
	if _, err := queryDatabase(context.Background(), nil, "s", node, "query", "", "", "", 10, 10); err == nil {
		t.Fatal("query without SQL unexpectedly succeeded")
	}
	if _, err := queryDatabase(context.Background(), nil, "s", node, "structure", "", "", "", 10, 10); err == nil {
		t.Fatal("structure without table unexpectedly succeeded")
	}
}

func TestDriverImportUsesInputAndDoesNotRequireSession(t *testing.T) {
	t.Setenv("OPSCOPILOT_JDBC_DRIVER_DIR", t.TempDir())
	if _, err := driverCommand(context.Background(), []string{"import", "--input", "-"}, strings.NewReader(`{"name":"x"}`)); err == nil {
		t.Fatal("invalid JDBC import unexpectedly succeeded")
	}
	if _, err := os.Stat(os.Getenv("OPSCOPILOT_JDBC_DRIVER_DIR")); err != nil {
		t.Fatal(err)
	}
}

func TestDatabaseOutputIsBoundedRedactedAndUTF8Safe(t *testing.T) {
	secret := "private-db-password"
	cell := strings.Repeat("中文 ", 20000) + secret
	value := dbworkspace.QueryResult{
		Error: strings.Repeat("错误 ", 20000) + secret,
		Sets:  []dbworkspace.ResultSet{{Statement: strings.Repeat("select ", 20000) + secret, Columns: []dbworkspace.Column{{Name: "c", Type: "text"}}, Rows: [][]*string{{&cell}}}},
	}
	got := boundOpsCLIQueryResult(value, []string{secret})
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) > databaseOutputLimit || !utf8.Valid(b) {
		t.Fatalf("bounded result size=%d valid=%t", len(b), utf8.Valid(b))
	}
	if strings.Contains(string(b), secret) || !got.Sets[0].Truncated {
		t.Fatalf("result was not safely redacted/truncated: %s", b[:minInt(len(b), 300)])
	}

	structure := boundOpsCLIStructure(dbworkspace.Structure{DDL: strings.Repeat("DDL ", 20000)}, []string{secret})
	sb, err := json.Marshal(structure)
	if err != nil {
		t.Fatal(err)
	}
	if len(sb) > databaseOutputLimit || !utf8.Valid(sb) || !strings.Contains(string(sb), `"truncated":true`) {
		t.Fatalf("structure bound incorrectly: size=%d", len(sb))
	}
}

func TestDatabaseOutputBudgetScalesForManyRowsAndMetadata(t *testing.T) {
	rows := make([][]*string, 5000)
	for i := range rows {
		value := fmt.Sprintf("row-%d", i)
		rows[i] = []*string{&value}
	}
	result := boundOpsCLIQueryResult(dbworkspace.QueryResult{Sets: []dbworkspace.ResultSet{{Columns: []dbworkspace.Column{{Name: "id"}}, Rows: rows}}}, nil)
	b, err := json.Marshal(result)
	if err != nil || len(b) > databaseOutputLimit || !result.Sets[0].Truncated {
		t.Fatalf("5000-row result was not bounded: size=%d err=%v", len(b), err)
	}

	columns := make([]dbworkspace.Column, 5000)
	for i := range columns {
		columns[i] = dbworkspace.Column{Name: fmt.Sprintf("column-%d", i), Type: "varchar"}
	}
	structure := boundOpsCLIStructure(dbworkspace.Structure{Columns: dbworkspace.ResultSet{Columns: columns}}, nil)
	sb, err := json.Marshal(structure)
	if err != nil || len(sb) > databaseOutputLimit || !strings.Contains(string(sb), `"truncated":true`) {
		t.Fatalf("large metadata was not bounded: size=%d err=%v", len(sb), err)
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestUnverifiedDialectCannotDisableReadOnlyProtection(t *testing.T) {
	dir := t.TempDir()
	id := "791203c7-6702-4635-b871-d4378620fd36"
	d := dbworkspace.JDBCDriver{ID: id, Name: "unverified", ClassName: "fixtures.Driver", URLTemplate: "jdbc:unverified://{host}:{port}", Dialect: "generic", Jars: []dbworkspace.JDBCJar{{Name: "fixture.jar", SHA256: strings.Repeat("0", 64)}}}
	if err := os.MkdirAll(filepath.Join(dir, id), 0700); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(d)
	if err := os.WriteFile(filepath.Join(dir, id, "driver.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPSCOPILOT_JDBC_DRIVER_DIR", dir)
	store, err := opssession.New(t.TempDir(), []byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Init("readonly", "test"); err != nil {
		t.Fatal(err)
	}
	_, err = queryDatabase(context.Background(), store, "readonly", testDatabaseNode("jdbc", id), "query", "", "", "SELECT 1", 10, 10)
	if err == nil || !strings.Contains(err.Error(), "只读事务") || strings.Contains(err.Error(), "取消") {
		t.Fatalf("unverified dialect did not fail closed: %v", err)
	}
}
