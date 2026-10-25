package dbworkspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"opscopilot/pkg/connectiondiag"
)

func TestRuntimeComparisonNeverExposesUnknownVMOptions(t *testing.T) {
	ctx, trace := connectiondiag.New(context.Background())
	recordJDBCRuntime(ctx, &jdbcRuntimeInfo{JavaVersion: "21.0.7", MaxHeapBytes: 160 << 20, DirectMemoryLimitBytes: 192 << 20, DirectMemorySource: "startup-argument",
		MemoryOptions:     []string{"-Xmx160m", "-XX:MaxDirectMemorySize=192m", "-Dpassword=unrelated-jvm-password", "-Xmx96m -Dtoken=private-token"},
		OptionEnvironment: []string{"_JAVA_OPTIONS", "PRIVATE_TOKEN=hidden"}})
	recordJDBCFailure(ctx, &jdbcFailureInfo{Phase: "建立连接／握手认证", Category: "memory.direct"})
	facts := trace.Facts()
	if !strings.Contains(facts["实际Java堆上限"], "160.00 MiB") || !strings.Contains(facts["直接内存限额证据"], "192.00 MiB") || !strings.Contains(facts["JVM参数环境变量"], "_JAVA_OPTIONS") {
		t.Fatal(facts)
	}
	text := diagnosticFacts(facts) + trace.Finish()
	for _, secret := range []string{"unrelated-jvm-password", "private-token", "PRIVATE_TOKEN", "hidden", "-Dpassword", "-Dtoken"} {
		if strings.Contains(text, secret) {
			t.Fatalf("unknown JVM option leaked: %s", secret)
		}
	}
	recordJDBCFailure(context.Background(), &jdbcFailureInfo{Phase: "untrusted", Category: "password=hidden"})
}

func TestDiagnosticFingerprintsAreBoundedAndCancellationAware(t *testing.T) {
	path := filepath.Join(t.TempDir(), "component.bin")
	_ = os.WriteFile(path, []byte("component"), 0600)
	want := sha256.Sum256([]byte("component"))
	got, err := diagnosticFileHash(context.Background(), path, 100)
	if err != nil || got != hex.EncodeToString(want[:]) {
		t.Fatal(got, err)
	}
	if _, err := diagnosticFileHash(context.Background(), path, 2); err == nil {
		t.Fatal("oversized file accepted")
	}
	if _, err := diagnosticFileHash(context.Background(), filepath.Dir(path), 100); err == nil {
		t.Fatal("directory accepted as component")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := diagnosticFileHash(ctx, path, 100); err == nil {
		t.Fatal("cancelled fingerprint continued")
	}
}

func TestDiagnosticReportBoundedWithLargeMultibyteFields(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "profiles.json"), nil)
	ctx, diag := s.beginDiagnostic(context.Background(), "测试连接", Profile{}, "private-diagnostic-password")
	for i := 0; i < 160; i++ {
		connectiondiag.Event(ctx, "fixture", "%s", strings.Repeat("界", 1500))
	}
	for _, key := range []string{"JDBC实际URL", "数据库目标", "Java版本", "JVM接收的内存参数", "SSH实际路径"} {
		connectiondiag.Fact(ctx, key, strings.Repeat("界", 1500))
	}
	err := s.finishDiagnostic(ctx, diag, &longDiagnosticFailure{})
	if err == nil || !utf8.ValidString(err.Error()) || strings.Contains(err.Error(), "诊断文件保存失败") || strings.Contains(err.Error(), "private-diagnostic-password") {
		t.Fatal("large diagnostic was lost, invalid or unfiltered")
	}
	files, _ := filepath.Glob(filepath.Join(filepath.Dir(s.path), "database-diagnostics", "*.log"))
	if len(files) != 1 {
		t.Fatal("missing diagnostic")
	}
	data, _ := os.ReadFile(files[0])
	if len(data) > connectiondiag.MaxReportBytes || !utf8.Valid(data) || !strings.Contains(string(data), "内容已截断") {
		t.Fatal("invalid or unbounded report")
	}
}

type longDiagnosticFailure struct{}

func (*longDiagnosticFailure) Error() string {
	return strings.Repeat("界", 10000) + "private-diagnostic-password"
}
