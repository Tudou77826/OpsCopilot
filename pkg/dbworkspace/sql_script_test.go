package dbworkspace

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/google/uuid"
)

func TestSQLScriptBoundaries(t *testing.T) {
	cases := []struct {
		name, dialect, script string
		count                 int
		body                  string
	}{
		{"mysql quotes", "mysql", "SELECT 'a;b', `x;y`; -- tail;\nSELECT 2;", 2, "SELECT 'a;b'"},
		{"mysql routine", "mysql", "CREATE PROCEDURE p() BEGIN SELECT 1; SELECT 2; END; CALL p();", 2, "SELECT 1; SELECT 2; END"},
		{"mysql delimiter", "mysql", "DELIMITER $$\nCREATE PROCEDURE p() BEGIN SELECT 1; END$$\nDELIMITER ;\nCALL p();", 2, "BEGIN SELECT 1; END"},
		{"mysql nested control", "mysql", "CREATE PROCEDURE p() BEGIN IF 1 THEN WHILE 0 DO SELECT 1; END WHILE; ELSE SELECT IF(1,2,3); END IF; END; SELECT 9;", 2, "END IF; END"},
		{"mysql case", "mysql", "CREATE PROCEDURE p() BEGIN CASE 1 WHEN 1 THEN SELECT CASE WHEN 1 THEN 2 ELSE 3 END; ELSE SELECT 0; END CASE; END; SELECT 9;", 2, "END CASE; END"},
		{"plsql case", "generic", "BEGIN CASE WHEN 1=1 THEN NULL; ELSE NULL; END CASE; END;\n/", 1, "END CASE; END;"},
		{"mysql repeat", "mysql", "CREATE PROCEDURE p() BEGIN REPEAT SELECT 1; UNTIL 1 END REPEAT; END;", 1, "END REPEAT; END"},
		{"unicode identifiers", "mysql", "CREATE PROCEDURE p() BEGIN SET 变量BEGIN = 1; SET 变量END = 2; END;", 1, "变量END"},
		{"transaction", "mysql", "BEGIN; SELECT 1; COMMIT;", 3, "BEGIN"},
		{"anonymous plsql", "generic", "DECLARE n INT := 1; BEGIN n := n+1; END;\n/\nSELECT 3;", 2, "n := n+1; END;"},
		{"nested plsql", "generic", "BEGIN IF 1=1 THEN BEGIN NULL; END; ELSE NULL; END IF; END;\n/", 1, "END IF; END;"},
		{"plsql loops", "generic", "BEGIN FOR i IN 1..2 LOOP NULL; END LOOP; WHILE 1=0 LOOP NULL; END LOOP; END;", 1, "END LOOP; END;"},
		{"create plsql", "generic", "CREATE OR REPLACE PROCEDURE p AS n INT := 1; BEGIN n := 2; END;\n/\nSELECT 1;", 2, "BEGIN n := 2; END;"},
		{"alternative literals", "generic", "BEGIN x := q'[it's;a;value]'; x := q'{b;c}'; END;\n/", 1, "q'[it's;a;value]'"},
		{"quoted body", "generic", "CREATE FUNCTION f() RETURNS void AS $body$ BEGIN PERFORM 1; END; $body$ LANGUAGE plpgsql; SELECT 2;", 2, "PERFORM 1;"},
		{"generic comments", "generic", "--heading;\nSELECT 'a;b'; /*skip*/ SELECT 2;", 2, "SELECT 'a;b'"},
		{"generic transaction", "generic", "BEGIN TRANSACTION; SELECT 1; COMMIT;", 3, "BEGIN TRANSACTION"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := executionSQL(QueryRequest{SQL: tc.script, Mode: "script"}, tc.dialect)
			if err != nil || len(result) != tc.count {
				t.Fatalf("got %#v, %v", result, err)
			}
			if !strings.Contains(result[0], tc.body) {
				t.Fatalf("body was split: %s", result[0])
			}
			for _, sql := range result {
				if strings.Contains(sql, "DELIMITER") || strings.TrimSpace(sql) == "/" {
					t.Fatalf("client directive submitted: %s", sql)
				}
			}
		})
	}
}

func TestSQLExecutionModesIntegration(t *testing.T) {
	port, err := strconv.Atoi(os.Getenv("OPSCOPILOT_TEST_MYSQL_PORT"))
	if err != nil {
		t.Skip("isolated MySQL endpoint required")
	}
	for _, engine := range []string{"mysql", "jdbc"} {
		t.Run(engine, func(t *testing.T) {
			s := newNativeTestService(filepath.Join(t.TempDir(), "profiles.json"), &memorySecrets{values: map[string]string{}})
			t.Cleanup(s.Close)
			driverID := ""
			if engine == "jdbc" {
				var d JDBCDriver
				s, d = jdbcTestService(t)
				driverID = d.ID
			}
			password := os.Getenv("OPSCOPILOT_TEST_MYSQL_PASSWORD")
			profile, err := s.Save(ProfileInput{Profile: Profile{Engine: engine, DriverID: driverID, Host: "127.0.0.1", Port: port, User: "root", JDBCAllowPublicKeyRetrieval: true}, Password: &password})
			if err != nil {
				t.Fatal(err)
			}
			session, err := s.Connect(context.Background(), profile.ID)
			if err != nil {
				t.Fatal(err)
			}
			run := func(sql, mode string, cursor int) (QueryResult, error) {
				return s.Run(QueryRequest{SessionID: session.ID, RequestID: uuid.NewString(), SQL: sql, Mode: mode, CursorOffset: cursor, Limit: 100, TimeoutSeconds: 10})
			}
			mustRun := func(sql, mode string, cursor int) QueryResult {
				t.Helper()
				r, err := run(sql, mode, cursor)
				if err != nil || r.Error != "" {
					t.Fatalf("execution failed: %v %s", err, r.Error)
				}
				return r
			}
			schema := "ops_sql_modes_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			mustRun("CREATE DATABASE "+quoteIdentifier(schema), "script", 0)
			defer mustRun("DROP DATABASE "+quoteIdentifier(schema), "script", 0)
			table := qualified(schema, "events")
			mustRun("CREATE TABLE "+table+" (id INT PRIMARY KEY)", "script", 0)
			document := "SELECT '第一😀';\nINSERT INTO " + table + " VALUES (99);"
			if result := mustRun(document, "statement", 0); len(result.Sets) != 1 {
				t.Fatal("current statement executed whole document")
			}
			position := len(utf16.Encode([]rune(document[:strings.Index(document, "INSERT")+2])))
			mustRun(document, "statement", position)
			mustRun("INSERT INTO "+table+" VALUES (2); SELECT id FROM "+table, "selection", 0)
			failed, err := run("INSERT INTO "+table+" VALUES (3); SELECT * FROM "+qualified(schema, "missing")+"; INSERT INTO "+table+" VALUES (4);", "script", 0)
			if err != nil || len(failed.Sets) != 1 || !strings.Contains(failed.Error, "第 2 条") {
				t.Fatalf("script did not stop: %#v %v", failed, err)
			}
			procedure := qualified(schema, "insert_pair")
			mustRun("DELIMITER //\nCREATE PROCEDURE "+procedure+"() BEGIN DECLARE n INT DEFAULT 1; WHILE n<=2 DO INSERT INTO "+table+" VALUES (10+n); SET n=n+1; END WHILE; END//\nDELIMITER ;\nCALL "+procedure+"();", "script", 0)
			if _, err := run("INSERT INTO "+table+" VALUES (88); CREATE PROCEDURE "+qualified(schema, "broken")+"() BEGIN SELECT 1;", "script", 0); err == nil {
				t.Fatal("incomplete block accepted")
			}
			rows := mustRun("SELECT id FROM "+table+" ORDER BY id", "script", 0).Sets[0].Rows
			if len(rows) != 5 || *rows[0][0] != "2" || *rows[1][0] != "3" || *rows[2][0] != "11" || *rows[3][0] != "12" || *rows[4][0] != "99" {
				t.Fatalf("wrong execution scope: %#v", rows)
			}
			mustRun("SELECT 1; SELECT 'unfinished", "statement", 0)
		})
	}
}

func TestSQLScriptRejectsIncompleteBlocks(t *testing.T) {
	for _, script := range []string{
		"SELECT 1; BEGIN NULL;", "DECLARE x INT;", "BEGIN IF 1=1 THEN NULL; END IF;", "BEGIN x := q'[broken'; END;",
		"CREATE PROCEDURE p AS x INT;", "BEGIN NULL;\n/", "CREATE PACKAGE p AS PROCEDURE a; END;", "SELECT $tag$broken", "DECLARE FUNCTION f RETURN INT AS BEGIN RETURN 1; END; BEGIN NULL; END;",
	} {
		if _, err := executionSQL(QueryRequest{SQL: script, Mode: "script"}, "generic"); err == nil {
			t.Fatalf("accepted incomplete script: %s", script)
		}
	}
	if _, err := executionSQL(QueryRequest{SQL: "SELECT 1", Mode: "unknown"}, "mysql"); err == nil {
		t.Fatal("unknown mode accepted")
	}
}

func TestGenericSQLLeadingCommentClassification(t *testing.T) {
	if !returnsRowsFor("--heading\nSELECT 1", "generic") {
		t.Fatal("generic query after comment was treated as a write")
	}
}

func TestCurrentStatementRange(t *testing.T) {
	text := "SELECT '中文😀;文本';\n--说明\nSELECT 2;\nSELECT 'unfinished"
	position := len(utf16.Encode([]rune(text[:strings.Index(text, "SELECT 2")+4])))
	result, err := executionSQL(QueryRequest{SQL: text, Mode: "statement", CursorOffset: position}, "generic")
	if err != nil || len(result) != 1 || !strings.Contains(result[0], "SELECT 2") {
		t.Fatalf("wrong current statement: %#v %v", result, err)
	}
	result, err = executionSQL(QueryRequest{SQL: "SELECT 1; SELECT 2;", Mode: "statement", CursorOffset: 8}, "mysql")
	if err != nil || len(result) != 1 || result[0] != "SELECT 1" {
		t.Fatalf("semicolon boundary: %#v %v", result, err)
	}
	result, err = executionSQL(QueryRequest{SQL: "SELECT 1;  ", Mode: "statement", CursorOffset: 11}, "mysql")
	if err != nil || result[0] != "SELECT 1" {
		t.Fatalf("trailing space: %#v %v", result, err)
	}
	result, err = executionSQL(QueryRequest{SQL: "BEGIN NULL; NULL; END;", Mode: "statement", CursorOffset: 15}, "generic")
	if err != nil || len(result) != 1 || result[0] != "BEGIN NULL; NULL; END;" {
		t.Fatalf("block was split: %#v %v", result, err)
	}
	for _, pos := range []int{-1, 999, 9} {
		// UTF-16 offset 9 lies inside the surrogate pair after "SELECT '".
		if _, err := executionSQL(QueryRequest{SQL: "SELECT '😀'", Mode: "statement", CursorOffset: pos}, "generic"); err == nil {
			t.Fatalf("invalid cursor accepted: %d", pos)
		}
	}
	if _, err := executionSQL(QueryRequest{SQL: strings.Repeat("SELECT 1;", 33), Mode: "script"}, "mysql"); err == nil {
		t.Fatal("script limit ignored")
	}
	result, err = executionSQL(QueryRequest{SQL: strings.Repeat("SELECT 1;", 33), Mode: "statement", CursorOffset: 0}, "mysql")
	if err != nil || len(result) != 1 {
		t.Fatalf("unrelated statement count prevented current execution: %v", err)
	}
}
