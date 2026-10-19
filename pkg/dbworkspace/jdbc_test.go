package dbworkspace

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestJDBCResultValidation(t *testing.T) {
	value := strings.Repeat("x", (1<<20)+1)
	for _, result := range []ResultSet{{Columns: []Column{{Name: "a"}}, Rows: [][]*string{{nil, nil}}}, {Columns: []Column{{Name: "a"}}, Rows: [][]*string{{&value}}}} {
		if validateJDBCResult(result) == nil {
			t.Fatal("invalid helper result accepted")
		}
	}
	if err := validateJDBCResult(ResultSet{Columns: []Column{{Name: "a"}}, Rows: [][]*string{{nil}}}); err != nil {
		t.Fatal(err)
	}
}

func TestJDBCMetadataColumnContract(t *testing.T) {
	schema, name, kind := "app", "events", "BIGINT"
	raw := ResultSet{Columns: []Column{{Name: "TABLE_SCHEM"}, {Name: "COLUMN_NAME"}, {Name: "TYPE_NAME"}}, Rows: [][]*string{{&schema, &name, &kind}}}
	projected := jdbcProject(raw, []string{"COLUMN_NAME", "TYPE_NAME", "IS_NULLABLE"}, []string{"字段", "类型", "可空"})
	if *projected.Rows[0][0] != "events" || *projected.Rows[0][1] != "BIGINT" || projected.Rows[0][2] != nil {
		t.Fatalf("vendor metadata leaked into frontend contract: %#v", projected)
	}
}

func jdbcTestService(t *testing.T) (*Service, JDBCDriver) {
	t.Helper()
	root := os.Getenv("OPSCOPILOT_TEST_JDBC_RUNTIME")
	jar := os.Getenv("OPSCOPILOT_TEST_JDBC_JAR")
	if root == "" || jar == "" {
		t.Skip("set JDBC runtime and official MySQL JAR paths for real import tests")
	}
	s := New(filepath.Join(t.TempDir(), "profiles.json"), &memorySecrets{values: map[string]string{}})
	s.JDBCRuntimeDir = root
	t.Cleanup(s.Close)
	d, err := s.ImportJDBC(context.Background(), JDBCImport{Name: "MySQL Connector/J", ClassName: "com.mysql.cj.jdbc.Driver", URLTemplate: "jdbc:mysql://{host}:{port}/{database}", Dialect: "mysql", Paths: []string{jar}})
	if err != nil {
		t.Fatal(err)
	}
	if d.Version == "" || len(d.Jars) != 1 || len(d.Jars[0].SHA256) != 64 {
		t.Fatalf("missing driver provenance: %#v", d)
	}
	return s, d
}
func TestJDBCRegistry(t *testing.T) {
	s, d := jdbcTestService(t)
	probe, err := s.startJDBC(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	var runtimeInfo struct {
		LegacyJaxb  bool   `json:"legacyJaxb"`
		JavaVersion string `json:"javaVersion"`
	}
	if err = probe.call(context.Background(), map[string]any{"op": "runtime"}, &runtimeInfo); err != nil {
		probe.close()
		t.Fatal(err)
	}
	probe.close()
	if !runtimeInfo.LegacyJaxb || runtimeInfo.JavaVersion == "" {
		t.Fatal("packaged Java lacks legacy JAXB compatibility")
	}
	status, err := s.JDBCStatus()
	if err != nil || !status.Available || len(status.Drivers) != 1 {
		t.Fatalf("status: %#v %v", status, err)
	}
	before, _ := os.ReadDir(s.driverDir())
	_, err = s.ImportJDBC(context.Background(), JDBCImport{Name: "bad", ClassName: "missing.Driver", URLTemplate: d.URLTemplate, Dialect: "mysql", Paths: []string{os.Getenv("OPSCOPILOT_TEST_JDBC_JAR")}})
	if err == nil {
		t.Fatal("bad class accepted")
	}
	_, err = s.ImportJDBC(context.Background(), JDBCImport{Name: "wrong protocol", ClassName: d.ClassName, URLTemplate: "jdbc:invalid://{host}:{port}/{database}", Dialect: "generic", Paths: []string{os.Getenv("OPSCOPILOT_TEST_JDBC_JAR")}})
	if err == nil || !strings.Contains(err.Error(), "不接受") {
		t.Fatalf("incompatible URL template accepted: %v", err)
	}
	after, _ := os.ReadDir(s.driverDir())
	if len(before) != len(after) {
		t.Fatal("failed import left files")
	}
	profile, err := s.Save(ProfileInput{Profile: Profile{Host: "127.0.0.1", Port: 3306, User: "test", Engine: "jdbc", DriverID: d.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if s.DeleteJDBC(d.ID) == nil {
		t.Fatal("in-use driver removed")
	}
	if err = s.Delete(profile.ID, profile.Revision); err != nil {
		t.Fatal(err)
	}
	imported := filepath.Join(s.driverDir(), d.ID, d.Jars[0].Name)
	f, err := os.OpenFile(imported, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("tampered")
	f.Close()
	process, err := s.startJDBC(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	defer process.close()
	if err = process.load(context.Background(), s.driverDir(), d, nil); err == nil {
		t.Fatal("tampered JAR accepted")
	}
	if err = s.DeleteJDBC(d.ID); err != nil {
		t.Fatal(err)
	}
}

func TestJDBCConnectionFailureDiagnostics(t *testing.T) {
	s, d := jdbcTestService(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	password := "diagnostic-secret-" + uuid.NewString()
	_, err = s.Test(context.Background(), ProfileInput{Profile: Profile{Engine: "jdbc", DriverID: d.ID, Host: "127.0.0.1", Port: port, User: "diagnostic-reader"}, Password: &password})
	if err == nil {
		t.Fatal("closed test port unexpectedly accepted connection")
	}
	detail := err.Error()
	for _, expected := range []string{"建立连接／握手认证", "SQLState", "ConnectException"} {
		if !strings.Contains(detail, expected) {
			t.Fatalf("missing useful cause %s: %s", expected, detail)
		}
	}
	if strings.Contains(detail, password) || strings.Contains(detail, "diagnostic-reader") {
		t.Fatal("connection credentials leaked into diagnostics")
	}
}

func TestJDBCLegacyNioCompatibility(t *testing.T) {
	root := os.Getenv("OPSCOPILOT_TEST_JDBC_RUNTIME")
	if root == "" {
		t.Skip("set JDBC runtime path for real Java 21 driver compatibility tests")
	}
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Fatal("a JDK compiler is required for the legacy driver fixture")
	}
	jarTool, err := exec.LookPath("jar")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	classes := t.TempDir()
	if out, err := exec.CommandContext(ctx, javac, "--release", "21", "-d", classes, filepath.Join("testdata", "LegacyNioDriver.java")).CombinedOutput(); err != nil {
		t.Fatalf("compile fixture: %v\n%s", err, out)
	}
	jarPath := filepath.Join(t.TempDir(), "legacy-nio-driver.jar")
	if out, err := exec.CommandContext(ctx, jarTool, "--create", "--file", jarPath, "-C", classes, ".").CombinedOutput(); err != nil {
		t.Fatalf("package fixture: %v\n%s", err, out)
	}
	s := New(filepath.Join(t.TempDir(), "profiles.json"), &memorySecrets{values: map[string]string{}})
	s.JDBCRuntimeDir = root
	t.Cleanup(s.Close)
	d, err := s.ImportJDBC(ctx, JDBCImport{Name: "Legacy NIO fixture", ClassName: "fixtures.LegacyNioDriver", URLTemplate: "jdbc:legacy-nio://{host}:{port}/{database}", Dialect: "generic", Paths: []string{jarPath}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Test(ctx, ProfileInput{Profile: Profile{Engine: "jdbc", DriverID: d.ID, Host: "127.0.0.1", Port: 1888, User: "fixture"}})
	if err == nil || !strings.Contains(err.Error(), "legacy-nio-reflection-ok") {
		t.Fatalf("legacy NIO initialization did not finish: %v", err)
	}
}
func TestJDBCMySQLIntegration(t *testing.T) {
	s, d := jdbcTestService(t)
	port, err := strconv.Atoi(os.Getenv("OPSCOPILOT_TEST_MYSQL_PORT"))
	if err != nil {
		t.Skip("isolated MySQL endpoint required")
	}
	password := os.Getenv("OPSCOPILOT_TEST_MYSQL_PASSWORD")
	p, err := s.Save(ProfileInput{Profile: Profile{Name: "jdbc integration", Engine: "jdbc", DriverID: d.ID, Host: "127.0.0.1", Port: port, User: "root", JDBCAllowPublicKeyRetrieval: true}, Password: &password})
	if err != nil {
		t.Fatal(err)
	}
	version, err := s.Test(context.Background(), ProfileInput{Profile: p})
	if err != nil || !strings.Contains(version, "MySQL") {
		t.Fatalf("test: %s %v", version, err)
	}
	info, err := s.Connect(context.Background(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	run := func(query string, limit int) QueryResult {
		t.Helper()
		r, err := s.Run(QueryRequest{SessionID: info.ID, RequestID: uuid.NewString(), SQL: query, Limit: limit, TimeoutSeconds: 10})
		if err != nil || r.Error != "" {
			t.Fatalf("SQL failed: %v %s", err, r.Error)
		}
		return r
	}
	schema := "ops_jdbc_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	run("CREATE DATABASE "+quoteIdentifier(schema), 100)
	dropped := false
	defer func() {
		if !dropped {
			run("DROP DATABASE "+quoteIdentifier(schema), 100)
		}
	}()
	name := qualified(schema, "events")
	run("CREATE TABLE "+name+" (id BIGINT PRIMARY KEY, amount DECIMAL(30,10), note TEXT NULL); INSERT INTO "+name+" VALUES (9007199254740993,12345678901234567890.1234567890,NULL),(2,1.5,'你好; world')", 100)
	result := run("SELECT * FROM "+name+" ORDER BY id DESC", 100)
	row := result.Sets[0].Rows[0]
	if *row[0] != "9007199254740993" || *row[1] != "12345678901234567890.1234567890" || row[2] != nil {
		t.Fatalf("lossy JDBC cells: %#v", row)
	}
	if !run("SELECT * FROM "+name, 1).Sets[0].Truncated {
		t.Fatal("limit lost")
	}
	oversized, err := s.Run(QueryRequest{SessionID: info.ID, RequestID: uuid.NewString(), SQL: "SELECT REPEAT('x', 1100000)"})
	if err != nil || oversized.Error == "" {
		t.Fatalf("oversized cell accepted: %v", err)
	}
	run("SELECT 1", 1)
	partial, err := s.Run(QueryRequest{SessionID: info.ID, RequestID: uuid.NewString(), SQL: "SELECT 1; SELECT missing_column FROM " + name})
	if err != nil || len(partial.Sets) != 1 || partial.Error == "" {
		t.Fatalf("partial: %#v %v", partial, err)
	}
	schemas, err := s.Schemas(info.ID)
	if err != nil || len(schemas) == 0 {
		t.Fatalf("schemas: %v", err)
	}
	tables, err := s.Tables(info.ID, schema)
	if err != nil || len(tables) != 1 || tables[0].Name != "events" {
		t.Fatalf("tables: %#v %v", tables, err)
	}
	structure, err := s.Structure(TableRequest{SessionID: info.ID, Database: schema, Table: "events"})
	if err != nil || len(structure.Columns.Rows) != 3 || structure.DDL == "" {
		t.Fatalf("structure: %#v %v", structure, err)
	}
	for _, kind := range []string{"select", "insert", "update", "delete"} {
		text, err := s.GenerateSQL(SQLTemplateRequest{SessionID: info.ID, Database: schema, Table: "events", Kind: kind})
		if err != nil || text == "" {
			t.Fatalf("template %s: %v", kind, err)
		}
	}
	// Use a newly created, narrowly scoped account to exercise uncached SHA-2
	// authentication, independent of root's existing server auth cache.
	t.Run("explicit RSA authentication", func(t *testing.T) {
		user := "ops_jdbc_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
		pass := uuid.NewString()
		run("CREATE USER '"+user+"'@'%' IDENTIFIED BY '"+pass+"'", 100)
		defer run("DROP USER '"+user+"'@'%'", 100)
		run("GRANT SELECT ON "+name+" TO '"+user+"'@'%'", 100)
		draft := p
		draft.ID = ""
		draft.User = user
		draft.Database = schema
		draft.JDBCAllowPublicKeyRetrieval = false
		if _, err := s.Test(context.Background(), ProfileInput{Profile: draft, Password: &pass}); err == nil {
			t.Fatal("uncached unencrypted auth unexpectedly allowed public key retrieval")
		}
		draft.JDBCAllowPublicKeyRetrieval = true
		if _, err := s.Test(context.Background(), ProfileInput{Profile: draft, Password: &pass}); err != nil {
			t.Fatalf("explicit RSA test: %v", err)
		}
	})
	browsed, err := s.Browse(TableRequest{SessionID: info.ID, Database: schema, Table: "events", Offset: 1, Limit: 1})
	if err != nil || browsed.Error != "" || len(browsed.Sets[0].Rows) != 1 {
		t.Fatalf("browse: %#v %v", browsed, err)
	}
	// A readonly transaction must block a write hidden inside a row-returning statement.
	ro := p
	ro.ID = ""
	ro.Revision = 0
	ro.ReadOnly = true
	rp, err := s.Save(ProfileInput{Profile: ro, Password: &password})
	if err != nil {
		t.Fatal(err)
	}
	ri, err := s.Connect(context.Background(), rp.ID)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.Run(QueryRequest{SessionID: ri.ID, RequestID: uuid.NewString(), Database: schema, SQL: "SELECT * FROM events"})
	if err != nil || out.Error != "" {
		t.Fatalf("readonly select: %#v %v", out, err)
	}
	if _, err = s.Run(QueryRequest{SessionID: ri.ID, RequestID: uuid.NewString(), SQL: "DELETE FROM " + name}); err == nil {
		t.Fatal("readonly write accepted")
	}
	out, err = s.Run(QueryRequest{SessionID: ri.ID, RequestID: uuid.NewString(), SQL: "WITH chosen AS (SELECT 2 AS id) DELETE FROM " + name + " WHERE id IN (SELECT id FROM chosen)"})
	if err == nil && out.Error == "" {
		t.Fatal("readonly server enforcement bypassed")
	}
	_ = s.Disconnect(ri.ID)
	// The TCP forwarder uses the same deadline-free channel interface as SSH.
	s.Gateway = func(context.Context, string, string) (DialContext, func() error, error) {
		return func(ctx context.Context, addr string) (net.Conn, error) {
			c, e := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
			if e != nil {
				return nil, e
			}
			return deadlineUnsupportedConn{c}, nil
		}, func() error { return nil }, nil
	}
	tunnel := p
	tunnel.ID = ""
	tunnel.Revision = 0
	tunnel.GatewayID = "test-owned-channel"
	tp, err := s.Save(ProfileInput{Profile: tunnel, Password: &password})
	if err != nil {
		t.Fatal(err)
	}
	ti, err := s.Connect(context.Background(), tp.ID)
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.Run(QueryRequest{SessionID: ti.ID, RequestID: uuid.NewString(), SQL: "SELECT 1"})
	if err != nil || out.Error != "" {
		t.Fatalf("tunnel: %#v %v", out, err)
	}
	_ = s.Disconnect(ti.ID)
	cancelled, err := s.Connect(context.Background(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := s.Run(QueryRequest{SessionID: cancelled.ID, RequestID: uuid.NewString(), SQL: "SELECT CONNECTION_ID()"})
	if err != nil || identity.Error != "" {
		t.Fatal("could not identify test session")
	}
	serverID := *identity.Sets[0].Rows[0][0]
	request := uuid.NewString()
	done := make(chan QueryResult, 1)
	go func() {
		r, _ := s.Run(QueryRequest{SessionID: cancelled.ID, RequestID: request, SQL: "SELECT SLEEP(20)", TimeoutSeconds: 30})
		done <- r
	}()
	time.Sleep(300 * time.Millisecond)
	deadline := time.Now().Add(5 * time.Second)
	for !s.Cancel(request) {
		if time.Now().After(deadline) {
			t.Fatal("cancel not registered")
		}
		time.Sleep(10 * time.Millisecond)
	}
	select {
	case r := <-done:
		if !r.Cancelled || !r.Disconnected {
			t.Fatalf("cancel: %#v", r)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("cancel hung")
	}
	remainingQuery := run("SELECT COUNT(*) FROM information_schema.PROCESSLIST WHERE ID="+serverID+" AND COMMAND='Query'", 1)
	if *remainingQuery.Sets[0].Rows[0][0] != "0" {
		t.Fatal("server-side query survived cancellation")
	}
	run("DROP DATABASE "+quoteIdentifier(schema), 100)
	dropped = true
	s.Close()
	s.mu.Lock()
	remaining := s.jdbcProcesses
	s.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("helper processes leaked: %d", remaining)
	}
}

func TestZenithURLTemplateNormalization(t *testing.T) {
	d := JDBCDriver{ClassName: "com.huawei.gauss.jdbc.ZenithDriver", URLTemplate: "jdbc:zenith://{host}:{port}/{database}"}
	normalizeJDBC(&d)
	if d.URLTemplate != "jdbc:zenith:@{host}:{port}" {
		t.Fatal(d.URLTemplate)
	}
	other := JDBCDriver{ClassName: "org.example.Driver", URLTemplate: "jdbc:custom://{host}:{port}/{database}"}
	normalizeJDBC(&other)
	if other.URLTemplate != "jdbc:custom://{host}:{port}/{database}" {
		t.Fatal("changed another vendor's template")
	}
}

func TestProductionRequiresImportedDriverPreservingLegacyProfiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")
	secrets := &memorySecrets{values: map[string]string{}}
	legacy := newNativeTestService(path, secrets)
	password := "fixture-secret"
	p, err := legacy.Save(ProfileInput{Profile: Profile{Engine: "mysql", Host: "127.0.0.1", Port: 3306, User: "fixture", GatewayID: "final55"}, Password: &password})
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := New(path, secrets)
	defer s.Close()
	if len(s.connectors) != 0 {
		t.Fatal("production registered native driver")
	}
	if _, err = s.Connect(context.Background(), p.ID); err == nil || !strings.Contains(err.Error(), "导入 JDBC") {
		t.Fatalf("native path accepted: %v", err)
	}
	profiles, err := s.List()
	if err != nil || len(profiles) != 1 || !profiles[0].PasswordSet || profiles[0].GatewayID != "final55" {
		t.Fatalf("legacy config lost: %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("legacy config was changed without explicit save")
	}
	_, saved, err := s.profile(p.ID)
	if err != nil || saved != password {
		t.Fatal("legacy credential lost")
	}
}
