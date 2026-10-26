package dbworkspace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestJDBCRuntimeHostResolver(t *testing.T) {
	waiting := errors.New("runtime is still preparing")
	s := New(filepath.Join(t.TempDir(), "profiles.json"), nil)
	s.ResolveJDBCRuntime = func() (string, error) { return "", waiting }
	if _, _, err := s.jdbcRuntime(); !errors.Is(err, waiting) {
		t.Fatalf("host readiness error lost: %v", err)
	}
	s.ResolveJDBCRuntime = func() (string, error) { return "", nil }
	if _, _, err := s.jdbcRuntime(); err == nil {
		t.Fatal("empty host path must not fall back to an unmanaged runtime")
	}
	root := t.TempDir()
	java := filepath.Join(root, "runtime", "bin", "java")
	if os.PathSeparator == '\\' {
		java += ".exe"
	}
	if err := os.MkdirAll(filepath.Dir(java), 0755); err != nil {
		t.Fatal(err)
	}
	helper := filepath.Join(root, "jdbc-helper.jar")
	for _, path := range []string{java, helper} {
		if err := os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	s.ResolveJDBCRuntime = func() (string, error) { return root, nil }
	if gotJava, gotHelper, err := s.jdbcRuntime(); err != nil || gotJava != java || gotHelper != helper {
		t.Fatalf("resolved runtime = %q, %q, %v", gotJava, gotHelper, err)
	}
	s.JDBCRuntimeDir = root
	s.ResolveJDBCRuntime = func() (string, error) { t.Fatal("explicit host override ignored"); return "", waiting }
	if _, _, err := s.jdbcRuntime(); err != nil {
		t.Fatal(err)
	}
}
