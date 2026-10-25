package dbworkspace

import (
	"context"
	"opscopilot/pkg/connectiondiag"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestJDBCDirectMemoryOptionsAreBoundedAndDoNotChangeHeap(t *testing.T) {
	for _, tc := range []struct {
		value, flag string
		invalid     bool
	}{{"", "", false}, {"1024", "1024m", false}, {" 256 ", "256m", false}, {"15", "", true}, {"1025", "", true}, {"-1", "", true}, {"96 -Xmx8g", "", true}} {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv(jdbcDirectMemoryEnv, tc.value)
			options, err := jdbcJVMOptions()
			if tc.invalid {
				if err == nil || len(options) != 0 {
					t.Fatal("invalid memory override accepted")
				}
				return
			}
			joined := strings.Join(options, " ")
			if err != nil || !strings.Contains(joined, "-Xmx96m") || (tc.flag == "" && strings.Contains(joined, "MaxDirectMemorySize")) || (tc.flag != "" && !strings.Contains(joined, "-XX:MaxDirectMemorySize="+tc.flag)) {
				t.Fatal(options, err)
			}
		})
	}
}

// Real private JVM and helper, deterministic driver; no database or credentials.
// This proves the two allocation paths, not compatibility with a vendor driver.
func TestJDBCDirectMemoryFailureAndExplicitOverride(t *testing.T) {
	runtimeDir := os.Getenv("OPSCOPILOT_TEST_JDBC_RUNTIME")
	if runtimeDir == "" {
		t.Skip("set JDBC runtime path for the real helper memory regression")
	}
	for _, name := range []string{"JAVA_TOOL_OPTIONS", "JDK_JAVA_OPTIONS", "_JAVA_OPTIONS"} {
		t.Setenv(name, "")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	classes := t.TempDir()
	if out, err := exec.CommandContext(ctx, "javac", "--release", "21", "-d", classes, filepath.Join("testdata", "DirectMemoryDriver.java")).CombinedOutput(); err != nil {
		t.Fatalf("compile memory fixture: %v\n%s", err, out)
	}
	jarPath := filepath.Join(t.TempDir(), "direct-memory-fixture.jar")
	if out, err := exec.CommandContext(ctx, "jar", "--create", "--file", jarPath, "-C", classes, ".").CombinedOutput(); err != nil {
		t.Fatalf("package memory fixture: %v\n%s", err, out)
	}
	s := New(filepath.Join(t.TempDir(), "profiles.json"), &memorySecrets{values: map[string]string{}})
	s.JDBCRuntimeDir = runtimeDir
	t.Cleanup(s.Close)
	t.Setenv(jdbcDirectMemoryEnv, "")
	d, err := s.ImportJDBC(ctx, JDBCImport{Name: "Direct memory fixture", ClassName: "fixtures.DirectMemoryDriver", URLTemplate: "jdbc:direct-memory://{host}:{port}/{database}", Dialect: "generic", Paths: []string{jarPath}})
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"raw", "wrapped"} {
		t.Run(mode, func(t *testing.T) {
			p, err := s.startJDBC(ctx, d)
			if err != nil {
				t.Fatal(err)
			}
			defer p.close()
			if err := p.load(ctx, s.driverDir(), d, nil); err != nil {
				t.Fatal(err)
			}
			err = p.call(ctx, map[string]any{"op": "open", "url": "jdbc:direct-memory://fixture/" + mode, "user": "fixture-private-user", "password": "fixture-private-password"}, nil)
			if err == nil {
				t.Fatal("128 MiB allocation unexpectedly fit in 96 MiB")
			}
			for _, text := range []string{"OutOfMemoryError", "直接内存", "128.0 MiB", "96.0 MiB", "fixtures.DirectMemoryDriver.connect"} {
				if !strings.Contains(err.Error(), text) {
					t.Fatalf("missing memory evidence %q: %v", text, err)
				}
			}
			if strings.Contains(err.Error(), "fixture-private") {
				t.Fatal("credentials leaked")
			}
			select {
			case <-p.done:
			default:
				t.Fatal("memory-failed helper was left reusable")
			}
		})
	}
	profile := Profile{Engine: "jdbc", DriverID: d.ID, Host: "127.0.0.1", Port: 1888, User: "memory-diagnostic-reader", Database: "wrapped"}
	_, failure := s.Test(ctx, ProfileInput{Profile: profile})
	if failure == nil {
		t.Fatal("expected a diagnostic allocation failure")
	}
	for _, evidence := range []string{"数据库连接诊断 v4", "失败或完成阶段：建立连接／握手认证", "本地 JDBC 直接内存分配失败", "本次JVM异常报告", "辅助程序SHA256", "JVM本体SHA256", "驱动JAR.1", "fixtures.DirectMemoryDriver.connect"} {
		if !strings.Contains(failure.Error(), evidence) {
			t.Fatalf("missing comparison evidence %q: %v", evidence, failure)
		}
	}
	t.Setenv(jdbcDirectMemoryEnv, "256")
	version, err := s.Test(ctx, ProfileInput{Profile: profile})
	if err != nil || !strings.Contains(version, "memory-fixture") {
		t.Fatalf("explicit 256 MiB direct-memory check failed: %s %v", version, err)
	}
	if destination := os.Getenv("OPSCOPILOT_TEST_JDBC_REPORT_DIR"); destination != "" {
		if err := os.MkdirAll(destination, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(destination, "failure.log"), []byte(failure.Error()), 0600); err != nil {
			t.Fatal(err)
		}
		files, _ := filepath.Glob(filepath.Join(filepath.Dir(s.path), "database-diagnostics", "*.log"))
		sort.Strings(files)
		if len(files) < 2 {
			t.Fatal("missing successful comparison report")
		}
		data, err := os.ReadFile(files[len(files)-1])
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(destination, "success.log"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	// An injected setting can differ even with the exact same application and
	// private Java. Observe it without printing arbitrary -D values or credentials.
	t.Setenv("_JAVA_OPTIONS", "-Xmx160m -XX:MaxDirectMemorySize=192m -Dpassword=private-jvm-value -Dtoken=private-jvm-token")
	observedCtx, trace := connectiondiag.New(ctx)
	p, err := s.startJDBC(observedCtx, d)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.load(observedCtx, s.driverDir(), d, nil); err != nil {
		p.close()
		t.Fatal(err)
	}
	p.close()
	facts := trace.Facts()
	if !strings.Contains(facts["JVM请求的内存参数"], "MaxDirectMemorySize=256m") || !strings.Contains(facts["JVM接收的内存参数"], "MaxDirectMemorySize=192m") || !strings.Contains(facts["直接内存限额证据"], "192.00 MiB") || !strings.Contains(facts["JVM参数环境变量"], "_JAVA_OPTIONS") {
		t.Fatal("effective JVM override not observed", facts)
	}
	log := diagnosticFacts(facts) + trace.Finish()
	if strings.Contains(log, "private-jvm") || strings.Contains(log, "-Dpassword") || strings.Contains(log, "-Dtoken") {
		t.Fatal("JVM environment credentials leaked")
	}
	t.Setenv("_JAVA_OPTIONS", "")
	t.Setenv("JAVA_TOOL_OPTIONS", "-XX:NoSuchOpsCopilotFlag=private-startup-token")
	_, startupErr := s.Test(ctx, ProfileInput{Profile: profile})
	if startupErr == nil || !strings.Contains(startupErr.Error(), "Java 拒绝启动参数") || !strings.Contains(startupErr.Error(), "JAVA_TOOL_OPTIONS") || strings.Contains(startupErr.Error(), "private-startup-token") || strings.Contains(startupErr.Error(), "NoSuchOpsCopilotFlag") {
		t.Fatalf("startup failure was lost or leaked a JVM parameter: %v", startupErr)
	}
	s.mu.Lock()
	remaining := s.jdbcProcesses
	s.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("memory comparison left %d helper processes reserved", remaining)
	}
}
