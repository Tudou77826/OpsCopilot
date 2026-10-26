package opscli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"opscopilot/pkg/dbworkspace"
)

func clearCLIEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{"OPSCOPILOT_SERVER_ROOT", "OPSCOPILOT_SERVER_KEY_FILE", "OPSCOPILOT_KNOWLEDGE_DIR", "OPSCOPILOT_JDBC_DRIVER_DIR", "OPSCOPILOT_JDBC_RUNTIME_DIR"} {
		t.Setenv(name, "")
	}
}

func runCLIForTest(t *testing.T, args []string, input string) (int, string) {
	t.Helper()
	var out, diagnostic bytes.Buffer
	code := Run(context.Background(), args, strings.NewReader(input), &out, &diagnostic)
	if diagnostic.Len() != 0 {
		t.Fatalf("unexpected diagnostic output: %s", diagnostic.String())
	}
	return code, out.String()
}

func TestHelpWorksWithoutEnvironmentForEveryCommand(t *testing.T) {
	clearCLIEnvironment(t)
	for _, doc := range commandDocs {
		for _, args := range [][]string{append(strings.Fields(doc.path), "--help"), append([]string{"help"}, strings.Fields(doc.path)...)} {
			code, out := runCLIForTest(t, args, "")
			if code != 0 || !strings.Contains(out, "opscopilot-cli "+doc.path) || !strings.Contains(out, doc.description) {
				t.Fatalf("help %v failed: code=%d %s", args, code, out)
			}
		}
	}
	for _, group := range []string{"session", "connection", "database", "file", "knowledge", "driver"} {
		code, out := runCLIForTest(t, []string{group}, "")
		if code != 0 || !strings.Contains(out, group+" 子命令") {
			t.Fatalf("group help failed: %s", out)
		}
	}
}

func TestInvalidInvocationReportsParametersBeforeEnvironment(t *testing.T) {
	clearCLIEnvironment(t)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"unknown-secret-value"}, "未知命令"},
		{[]string{"database", "drop"}, "未知子命令"},
		{[]string{"session", "init", "--session", "s"}, "--environment"},
		{[]string{"connection", "put", "--session", "s"}, "--input"},
		{[]string{"exec", "--session", "s", "--node", "n"}, "--command"},
		{[]string{"database", "query", "--session", "s", "--node", "n"}, "--sql"},
		{[]string{"database", "structure", "--session", "s", "--node", "n"}, "--table"},
		{[]string{"database", "query", "--session", "s", "--node", "n", "--sql", "SELECT 1", "--timeout-sec", "121"}, "1–120"},
		{[]string{"file", "download", "--session", "s", "--node", "n", "--remote", "/tmp/a", "--local", "../outside"}, "相对文件名"},
		{[]string{"driver", "import"}, "--input"},
		{[]string{"knowledge", "search"}, "--query"},
		{[]string{"knowledge", "read"}, "--id"},
	} {
		code, out := runCLIForTest(t, tc.args, "")
		var result response
		if err := json.Unmarshal([]byte(out), &result); err != nil {
			t.Fatal(err, out)
		}
		if code == 0 || result.Success || !strings.Contains(result.Error, tc.want) || result.Hint == "" || strings.Contains(out, "unknown-secret-value") {
			t.Fatalf("invocation %v: code=%d %s", tc.args, code, out)
		}
	}
}

func TestHelpTokenAsOptionValueIsNotLocalHelp(t *testing.T) {
	clearCLIEnvironment(t)
	code, out := runCLIForTest(t, []string{"exec", "--session", "s", "--node", "n", "--command", "--help"}, "")
	if code == 0 || !strings.Contains(out, "OPSCOPILOT_SERVER_KEY_FILE") || !json.Valid([]byte(out)) {
		t.Fatalf("command value was consumed as help: %s", out)
	}
}

func TestSQLFileInputAndConflicts(t *testing.T) {
	clearCLIEnvironment(t)
	path := filepath.Join(t.TempDir(), "query.sql")
	sql := "\ufeffSELECT '中文';\n"
	if err := os.WriteFile(path, []byte(sql), 0600); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{path, "-"} {
		got, err := readSQL(source, strings.NewReader(sql))
		if err != nil || got != strings.TrimPrefix(sql, "\ufeff") {
			t.Fatalf("SQL input source %q: %q %v", source, got, err)
		}
	}
	base := []string{"database", "query", "--session", "s", "--node", "n"}
	for _, input := range []string{"", strings.Repeat("x", (1<<20)+1), "SELECT '\x00'", string([]byte{0xff})} {
		code, out := runCLIForTest(t, append(append([]string{}, base...), "--sql-file", "-"), input)
		if code == 0 || strings.Contains(out, "OPSCOPILOT_SERVER_KEY_FILE") {
			t.Fatalf("invalid SQL reached environment loading: %s", out)
		}
	}
	for _, options := range [][]string{{"--sql", "SELECT 1", "--sql-file", path}, {"--sql=", "--sql-file=-"}} {
		code, out := runCLIForTest(t, append(append([]string{}, base...), options...), "SELECT 2")
		if code == 0 || !strings.Contains(out, "不能同时提供") {
			t.Fatalf("conflicting SQL inputs accepted: %s", out)
		}
	}
	code, out := runCLIForTest(t, append(base, "--sql-file", path), "")
	if code == 0 || !strings.Contains(out, "OPSCOPILOT_SERVER_KEY_FILE") {
		t.Fatalf("valid SQL file was not accepted before environment loading: %s", out)
	}
}

func TestDriverListReportsAdaptationSeparatelyFromRuntime(t *testing.T) {
	clearCLIEnvironment(t)
	dir := t.TempDir()
	t.Setenv("OPSCOPILOT_JDBC_DRIVER_DIR", dir)
	for i, dialect := range []string{"mysql", "generic"} {
		id := []string{"791203c7-6702-4635-b871-d4378620fd36", "891203c7-6702-4635-b871-d4378620fd36"}[i]
		if err := os.MkdirAll(filepath.Join(dir, id), 0700); err != nil {
			t.Fatal(err)
		}
		driver := dbworkspace.JDBCDriver{ID: id, Name: dialect, ClassName: "fixture.Driver", URLTemplate: "jdbc:fixture://{host}:{port}", Dialect: dialect}
		driver.Jars = []dbworkspace.JDBCJar{{Name: "fixture.jar", SHA256: strings.Repeat("0", 64)}}
		if dialect == "mysql" {
			driver.URLTemplate = "jdbc:mysql://{host}:{port}/{database}"
		}
		data, _ := json.Marshal(driver)
		if err := os.WriteFile(filepath.Join(dir, id, "driver.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	value, err := driverCommand(context.Background(), []string{"list"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	result := value.(serverDriverStatus)
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var public map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &public); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"available", "message", "drivers", "serverCapabilities"} {
		if public[key] == nil {
			t.Fatalf("driver list lost public field %q", key)
		}
	}
	if result.Available || len(result.Drivers) != 2 || len(result.ServerCapabilities) != 2 {
		t.Fatalf("driver/runtime distinction lost: %+v", result)
	}
	for _, item := range result.ServerCapabilities {
		if item.ReadOnlyAdapted != strings.HasPrefix(item.DriverID, "7") || item.Reason == "" {
			t.Fatalf("wrong capability: %+v", item)
		}
	}
}
