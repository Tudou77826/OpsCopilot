package opscli

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"opscopilot/pkg/dbworkspace"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// This test is opt-in and uses only the repository's isolated database fixture.
// It never prints the endpoint or password and never removes fixture data.
func TestDatabaseLiveRunJDBCWorkflow(t *testing.T) {
	runtimeDir := os.Getenv("OPSCOPILOT_TEST_JDBC_RUNTIME")
	jar := os.Getenv("OPSCOPILOT_TEST_JDBC_JAR")
	portText := os.Getenv("OPSCOPILOT_TEST_MYSQL_PORT")
	password := os.Getenv("OPSCOPILOT_TEST_MYSQL_PASSWORD")
	host := os.Getenv("OPSCOPILOT_TEST_MYSQL_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	if runtimeDir == "" || jar == "" || portText == "" || password == "" {
		t.Skip("set OPSCOPILOT_TEST_JDBC_RUNTIME, OPSCOPILOT_TEST_JDBC_JAR, OPSCOPILOT_TEST_MYSQL_PORT and OPSCOPILOT_TEST_MYSQL_PASSWORD for the isolated live test")
	}
	if _, err := os.Stat(filepath.Join(runtimeDir, "jdbc-helper.jar")); err != nil {
		t.Fatal("configured JDBC runtime is unavailable")
	}
	if _, err := os.Stat(jar); err != nil {
		t.Fatal("configured JDBC driver JAR is unavailable")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		t.Fatal("invalid isolated MySQL port")
	}

	root := t.TempDir()
	keyPath := filepath.Join(root, "server.key")
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, key, 0600); err != nil {
		t.Fatal(err)
	}
	driverDir := filepath.Join(root, "jdbc-drivers")
	if err := os.MkdirAll(driverDir, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPSCOPILOT_SERVER_ROOT", filepath.Join(root, "server"))
	t.Setenv("OPSCOPILOT_SERVER_KEY_FILE", keyPath)
	t.Setenv("OPSCOPILOT_JDBC_RUNTIME_DIR", runtimeDir)
	t.Setenv("OPSCOPILOT_JDBC_DRIVER_DIR", driverDir)

	run := func(args []string, input string) response {
		t.Helper()
		var out bytes.Buffer
		code := Run(context.Background(), args, strings.NewReader(input), &out, &bytes.Buffer{})
		var result response
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatalf("command %v returned invalid response: %v", args, err)
		}
		if strings.Contains(out.String(), password) {
			t.Fatalf("command %v leaked the database credential", args)
		}
		if code != 0 || !result.Success {
			t.Fatalf("command %v failed: code=%d status=%s error=%s", args, code, result.Status, result.Error)
		}
		return result
	}
	runExpectFailure := func(args []string, input string) response {
		t.Helper()
		var out bytes.Buffer
		code := Run(context.Background(), args, strings.NewReader(input), &out, &bytes.Buffer{})
		var result response
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatalf("command %v returned invalid response: %v", args, err)
		}
		if strings.Contains(out.String(), password) {
			t.Fatalf("failed command %v leaked the database credential", args)
		}
		if code == 0 || result.Success {
			t.Fatalf("command %v unexpectedly succeeded", args)
		}
		return result
	}

	run([]string{"session", "init", "--session", "live-jdbc", "--environment", "isolated-test"}, "")
	driver := run([]string{"driver", "import", "--input", "-"}, fmt.Sprintf(`{"name":"live mysql","className":"com.mysql.cj.jdbc.Driver","urlTemplate":"jdbc:mysql://{host}:{port}/{database}","dialect":"mysql","paths":[%q]}`, jar))
	driverData, ok := driver.Data.(map[string]any)
	if !ok || strings.TrimSpace(fmt.Sprint(driverData["id"])) == "" {
		t.Fatalf("driver import did not return an id")
	}
	driverID := fmt.Sprint(driverData["id"])
	nodeInput := fmt.Sprintf(`{"id":"live-db","kind":"database","password":%q,"database":{"engine":"jdbc","driverId":%q,"host":%q,"port":%d,"user":"ops_review","database":"opscopilot_review","jdbcAllowPublicKeyRetrieval":true,"readOnly":true}}`, password, driverID, host, port)
	run([]string{"connection", "put", "--session", "live-jdbc", "--input", "-"}, nodeInput)

	query := run([]string{"database", "query", "--session", "live-jdbc", "--node", "live-db", "--database", "opscopilot_review", "--sql", "SELECT COUNT(*) AS n FROM service_nodes"}, "")
	// Both file and stdin sources go through the same query and readonly path.
	sqlFile := filepath.Join(root, "query.sql")
	if err := os.WriteFile(sqlFile, []byte("SELECT COUNT(*) AS n\nFROM service_nodes"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{sqlFile, "-"} {
		fromFile := run([]string{"database", "query", "--session", "live-jdbc", "--node", "live-db", "--database", "opscopilot_review", "--sql-file", source}, "SELECT COUNT(*) AS n\nFROM service_nodes")
		want, _ := json.Marshal(query.Data)
		got, _ := json.Marshal(fromFile.Data)
		var expected, actual dbworkspace.QueryResult
		if err := json.Unmarshal(want, &expected); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(got, &actual); err != nil {
			t.Fatal(err)
		}
		if len(actual.Sets) != 1 || len(expected.Sets) != 1 || !reflect.DeepEqual(actual.Sets[0].Rows, expected.Sets[0].Rows) {
			t.Fatal("SQL input source returned a different result")
		}
	}
	qb, _ := json.Marshal(query.Data)
	var selected dbworkspace.QueryResult
	if err := json.Unmarshal(qb, &selected); err != nil {
		t.Fatal(err)
	}
	if len(selected.Sets) != 1 || len(selected.Sets[0].Rows) != 1 || len(selected.Sets[0].Rows[0]) != 1 || selected.Sets[0].Rows[0][0] == nil {
		t.Fatal("COUNT result missing")
	}
	if count, err := strconv.Atoi(*selected.Sets[0].Rows[0][0]); err != nil || count < 1 {
		t.Fatal("isolated fixture count was not returned")
	}
	run([]string{"database", "schemas", "--session", "live-jdbc", "--node", "live-db"}, "")
	run([]string{"database", "tables", "--session", "live-jdbc", "--node", "live-db", "--database", "opscopilot_review"}, "")
	run([]string{"database", "structure", "--session", "live-jdbc", "--node", "live-db", "--database", "opscopilot_review", "--table", "service_nodes"}, "")

	writeDenied := runExpectFailure([]string{"database", "query", "--session", "live-jdbc", "--node", "live-db", "--database", "opscopilot_review", "--sql", "DELETE FROM query_scratch"}, "")
	if !strings.Contains(writeDenied.Error, "只读") {
		t.Fatal("write failed for a reason other than read-only policy")
	}
	stdinWriteDenied := runExpectFailure([]string{"database", "query", "--session", "live-jdbc", "--node", "live-db", "--sql-file", "-"}, "DELETE FROM query_scratch")
	if !strings.Contains(stdinWriteDenied.Error, "只读") {
		t.Fatal("SQL stdin bypassed readonly validation")
	}
	runExpectFailure([]string{"database", "query", "--session", "live-jdbc", "--node", "live-db", "--database", "opscopilot_review", "--sql", "SELECT * FROM table_that_does_not_exist"}, "")
	long := run([]string{"database", "query", "--session", "live-jdbc", "--node", "live-db", "--database", "opscopilot_review", "--sql", "SELECT id, REPEAT('x', 200) AS payload FROM job_runs"}, "")
	longJSON, _ := json.Marshal(long.Data)
	if len(longJSON) > databaseOutputLimit || !strings.Contains(string(longJSON), `"truncated":true`) {
		t.Fatalf("long result was not bounded: size=%d", len(longJSON))
	}
	timedOut := runExpectFailure([]string{"database", "query", "--session", "live-jdbc", "--node", "live-db", "--database", "opscopilot_review", "--timeout-sec", "1", "--sql", "SELECT SLEEP(5)"}, "")
	if timedOut.Status != "timeout" {
		t.Fatalf("query did not fail with a timeout: %s", timedOut.Status)
	}

	run([]string{"session", "close", "--session", "live-jdbc"}, "")
	closed := run([]string{"session", "status", "--session", "live-jdbc"}, "")
	closedData, ok := closed.Data.(map[string]any)
	if !ok || closedData["closed"] != true {
		t.Fatalf("closed session status did not report closed=true")
	}
}
