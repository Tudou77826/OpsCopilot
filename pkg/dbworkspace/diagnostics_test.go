package dbworkspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"opscopilot/pkg/connectiondiag"
)

func TestDiagnosticPreservesCauseAndRecordsEvidence(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "database.json"), nil)
	s.BuildVersion = "diagnostic-test"
	ctx, d := s.beginDiagnostic(context.Background(), "测试连接", Profile{Host: "127.0.0.1", Port: 32063, User: "db-reader"}, "db-secret")
	connectiondiag.AddSecrets(ctx, "ssh-secret")
	connectiondiag.Event(ctx, "ssh.route", "本机 → node51:22 → node55:22")
	connectiondiag.Event(ctx, "ssh.database.channel.rejected", "reason=1 administratively prohibited password=ssh-secret")
	cause := errors.New("driver reset db-secret db-reader")
	err := s.finishDiagnostic(ctx, d, cause)
	if !errors.Is(err, cause) {
		t.Fatal("lost original cause")
	}
	for _, expected := range []string{"数据库连接诊断 v3", "diagnostic-test", "node51:22", "node55:22", "拒绝 direct-tcpip", "具体服务端规则", "诊断日志："} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("missing evidence: %s", expected)
		}
	}
	paths, _ := filepath.Glob(filepath.Join(filepath.Dir(s.path), "database-diagnostics", "*.log"))
	if len(paths) != 1 {
		t.Fatalf("expected isolated diagnostic file: %v", paths)
	}
	data, readErr := os.ReadFile(paths[0])
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, secret := range []string{"db-secret", "db-reader", "ssh-secret"} {
		if strings.Contains(err.Error(), secret) || strings.Contains(string(data), secret) {
			t.Fatal("diagnostic leaked credentials")
		}
	}
}

func TestDiagnosticRetentionAndWriteFailure(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "database.json"), nil)
	for n := 0; n < 23; n++ {
		ctx, d := s.beginDiagnostic(context.Background(), fmt.Sprint(n), Profile{}, "")
		if err := s.finishDiagnostic(ctx, d, nil); err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.Join(filepath.Dir(s.path), "database-diagnostics")
	_ = os.WriteFile(filepath.Join(dir, "keep-user-file.log"), []byte("keep"), 0600)
	ctx, d := s.beginDiagnostic(context.Background(), "new", Profile{}, "")
	_ = s.finishDiagnostic(ctx, d, nil)
	entries, _ := os.ReadDir(dir)
	if len(entries) != 21 {
		t.Fatalf("expected 20 generated logs and user file: %d", len(entries))
	}
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	_ = os.WriteFile(blocked, []byte("blocked"), 0600)
	s.path = filepath.Join(blocked, "database.json")
	ctx, d = s.beginDiagnostic(context.Background(), "blocked", Profile{}, "")
	if err := s.finishDiagnostic(ctx, d, nil); err != nil {
		t.Fatal("diagnostic write failure changed successful connection")
	}
	cause := errors.New("original network failure")
	ctx, d = s.beginDiagnostic(context.Background(), "blocked", Profile{}, "")
	err := s.finishDiagnostic(ctx, d, cause)
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "诊断文件保存失败") {
		t.Fatal("write failure obscured original error")
	}
}

func TestDiagnosticDoesNotMisclassifyMixedChannelAttempts(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "database.json"), nil)
	ctx, d := s.beginDiagnostic(context.Background(), "测试连接", Profile{}, "")
	connectiondiag.Event(ctx, "ssh.database.channel.rejected", "first attempt")
	connectiondiag.Event(ctx, "ssh.database.channel.ok", "second attempt")
	err := s.finishDiagnostic(ctx, d, errors.New("later handshake failed"))
	if strings.Contains(err.Error(), "数据库通道未建立") || !strings.Contains(err.Error(), "至少一次确认数据库 TCP 通道") || !strings.Contains(err.Error(), "同次操作也有失败") {
		t.Fatal("one rejection obscured another established channel")
	}
}

func TestDiagnosticJDBCSeparatesConnectedFromMetadataFailure(t *testing.T) {
	root := os.Getenv("OPSCOPILOT_TEST_JDBC_RUNTIME")
	if root == "" {
		t.Skip("set JDBC runtime path for real helper phase diagnostics")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	classes := t.TempDir()
	if out, err := exec.CommandContext(ctx, "javac", "--release", "8", "-d", classes, filepath.Join("testdata", "MetadataFailureDriver.java")).CombinedOutput(); err != nil {
		t.Fatalf("compile fixture: %v %s", err, out)
	}
	jar := filepath.Join(t.TempDir(), "metadata-fixture.jar")
	if out, err := exec.CommandContext(ctx, "jar", "--create", "--file", jar, "-C", classes, ".").CombinedOutput(); err != nil {
		t.Fatalf("package fixture: %v %s", err, out)
	}
	s := New(filepath.Join(t.TempDir(), "database.json"), nil)
	s.JDBCRuntimeDir = root
	t.Cleanup(s.Close)
	driver, err := s.ImportJDBC(ctx, JDBCImport{Name: "metadata fixture", ClassName: "fixtures.MetadataFailureDriver", URLTemplate: "jdbc:metadata-fixture://{host}:{port}", Dialect: "generic", Paths: []string{jar}})
	if err != nil {
		t.Fatal(err)
	}
	password := "metadata-private-password"
	_, err = s.Test(ctx, ProfileInput{Profile: Profile{Engine: "jdbc", DriverID: driver.ID, Host: "127.0.0.1", Port: 32063, User: "metadata-private-user"}, Password: &password})
	if err == nil {
		t.Fatal("metadata error was ignored")
	}
	for _, expected := range []string{"jdbc.driver.connected", "jdbc.metadata.version", "driver.connect 已返回连接", "fixture metadata denied", "读取数据库版本", "SQLState HY000", "代码 777", "Java版本=21", "SHA256="} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("missing %s in diagnostic: %s", expected, err)
		}
	}
	if strings.Contains(err.Error(), "metadata-private-user") || strings.Contains(err.Error(), password) || strings.Contains(err.Error(), "jdbc.metadata.version.ok") {
		t.Fatal("metadata failure trace is misleading or leaks credentials")
	}
}
