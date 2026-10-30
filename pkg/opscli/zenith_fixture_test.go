package opscli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Opt-in end-to-end bridge test using a mock driver. This does not establish
// compatibility with an actual GaussDB server or vendor JAR.
func TestZenithFixtureWorkflow(t *testing.T) {
	runtimeDir := os.Getenv("OPSCOPILOT_TEST_JDBC_RUNTIME")
	if runtimeDir == "" {
		t.Skip("set OPSCOPILOT_TEST_JDBC_RUNTIME for bridge fixture")
	}
	for _, tool := range []string{"javac", "jar"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatalf("%s: %v", tool, err)
		}
	}
	dir := t.TempDir()
	classes := filepath.Join(dir, "classes")
	if err := os.MkdirAll(classes, 0700); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("javac", "-d", classes, filepath.Join("testdata", "ZenithDriver.java")).CombinedOutput(); err != nil {
		t.Fatalf("javac: %v: %s", err, out)
	}
	jar := filepath.Join(dir, "zenith-fixture.jar")
	if out, err := exec.Command("jar", "cf", jar, "-C", classes, ".").CombinedOutput(); err != nil {
		t.Fatalf("jar: %v: %s", err, out)
	}
	key := filepath.Join(dir, "key")
	if err := os.WriteFile(key, []byte("01234567890123456789012345678901"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPSCOPILOT_JDBC_RUNTIME_DIR", runtimeDir)
	t.Setenv("OPSCOPILOT_JDBC_DRIVER_DIR", filepath.Join(dir, "drivers"))
	t.Setenv("OPSCOPILOT_SERVER_ROOT", filepath.Join(dir, "sessions"))
	t.Setenv("OPSCOPILOT_SERVER_KEY_FILE", key)
	run := func(args []string, input string, success bool) response {
		t.Helper()
		var out bytes.Buffer
		code := Run(context.Background(), args, strings.NewReader(input), &out, &bytes.Buffer{})
		var result response
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatalf("invalid JSON %s: %v", out.String(), err)
		}
		if (code == 0 && result.Success) != success {
			t.Fatalf("%v: %s", args, out.String())
		}
		return result
	}
	driver := run([]string{"driver", "import", "--input", "-"}, fmt.Sprintf(`{"name":"fixture","className":"com.huawei.gauss.jdbc.ZenithDriver","urlTemplate":"jdbc:zenith:@{host}:{port}","dialect":"generic","paths":[%q]}`, jar), true)
	id := fmt.Sprint(driver.Data.(map[string]any)["id"])
	status := run([]string{"driver", "list"}, "", true)
	encoded, _ := json.Marshal(status.Data)
	if !strings.Contains(string(encoded), `"querySupported":true`) || !strings.Contains(string(encoded), `"readOnlyMode":"sql_guard"`) {
		t.Fatalf("incorrect driver capabilities: %s", encoded)
	}
	run([]string{"session", "init", "--session", "fixture", "--environment", "test"}, "", true)
	run([]string{"connection", "put", "--session", "fixture", "--input", "-"}, fmt.Sprintf(`{"id":"db","kind":"database","database":{"engine":"jdbc","driverId":%q,"host":"127.0.0.1","port":1888,"user":"reader"}}`, id), true)
	result := run([]string{"database", "query", "--session", "fixture", "--node", "db", "--sql", "SELECT 42 FROM dual"}, "", true)
	encoded, _ = json.Marshal(result.Data)
	if !strings.Contains(string(encoded), `"42"`) {
		t.Fatalf("query result missing: %s", encoded)
	}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"database", "schemas", "--session", "fixture", "--node", "db"}, "APP"},
		{[]string{"database", "tables", "--session", "fixture", "--node", "db", "--database", "APP"}, "ORDERS"},
		{[]string{"database", "structure", "--session", "fixture", "--node", "db", "--database", "APP", "--table", "ORDERS"}, "IDX_ID"},
	} {
		result := run(tc.args, "", true)
		encoded, _ := json.Marshal(result.Data)
		if !strings.Contains(string(encoded), tc.want) {
			t.Fatalf("metadata %v missing %q: %s", tc.args, tc.want, encoded)
		}
	}
	run([]string{"database", "query", "--session", "fixture", "--node", "db", "--sql", "DELETE FROM dual"}, "", false)
}
