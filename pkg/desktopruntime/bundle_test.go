package desktopruntime

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testBundle(t *testing.T, root, tag string, extra map[string]string) string {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	files := map[string]string{"jdbc-helper.jar": "helper-" + tag, "runtime/bin/java.exe": "java-" + tag, "runtime/lib/modules": "modules-" + tag}
	for k, v := range extra {
		files[k] = v
	}
	for k, v := range files {
		f, err := w.Create(k)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write([]byte(v)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, BundleName), b.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(b.Bytes()))
}

func TestInstallRetryPreservesUserAndLegacyFiles(t *testing.T) {
	root := t.TempDir()
	untouched := map[string]string{"sessions.json": "connections", "config.json": "preferences", "jdbc-drivers/vendor.jar": "user driver", "jdbc-runtime/jdbc-helper.jar": "legacy helper", "knowledge/runbook.md": "user notes", "secret.key": "key"}
	for name, data := range untouched {
		p := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(p), 0755)
		if err := os.WriteFile(p, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	digest := testBundle(t, root, "first", nil)
	failed := func(context.Context, string) error { return fmt.Errorf("simulated Java failure") }
	if err := ensure(context.Background(), root, digest, nil, failed); err == nil {
		t.Fatal("failed Java must not become active")
	}
	if _, err := os.Stat(filepath.Join(managedDir(root, digest), receipt)); !os.IsNotExist(err) {
		t.Fatal("failed install committed", err)
	}
	calls := 0
	probe := func(context.Context, string) error { calls++; return nil }
	if err := ensure(context.Background(), root, digest, nil, probe); err != nil {
		t.Fatal(err)
	}
	if err := ensure(context.Background(), root, digest, nil, probe); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("unchanged component was reinstalled")
	}
	for name, data := range untouched {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(b) != data {
			t.Fatalf("user file modified: %s", name)
		}
	}
	entries, _ := os.ReadDir(filepath.Join(root, "jdbc-runtime", "managed"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".prepare-") {
			t.Fatal("failed staging was not cleaned")
		}
	}
}

func TestExactVersionSelectionAndRepair(t *testing.T) {
	root := t.TempDir()
	probe := func(context.Context, string) error { return nil }
	first := testBundle(t, root, "first", nil)
	if err := ensure(context.Background(), root, first, nil, probe); err != nil {
		t.Fatal(err)
	}
	second := testBundle(t, root, "second", nil)
	previous := ExpectedSHA256
	ExpectedSHA256 = second
	t.Cleanup(func() { ExpectedSHA256 = previous })
	if _, err := Resolve(root); err == nil {
		t.Fatal("must not fall back to previous helper")
	}
	if err := ensure(context.Background(), root, second, nil, probe); err != nil {
		t.Fatal(err)
	}
	resolved, err := Resolve(root)
	if err != nil || resolved != managedDir(root, second) {
		t.Fatal(resolved, err)
	}
	if b, err := os.ReadFile(filepath.Join(managedDir(root, first), "jdbc-helper.jar")); err != nil || string(b) != "helper-first" {
		t.Fatal("previous version destroyed")
	}
	if err := os.WriteFile(filepath.Join(resolved, "jdbc-helper.jar"), []byte("damaged"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := ensure(context.Background(), root, second, nil, probe); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(resolved, "jdbc-helper.jar"))
	if string(b) != "helper-second" {
		t.Fatal("corrupt helper not repaired")
	}
}

func TestRejectCorruptOrUnsafeBundle(t *testing.T) {
	for _, name := range []string{"../outside", "/absolute", "runtime\\bad", "runtime/file:stream", "runtime/CON.txt", "runtime/file.", "JDBC-HELPER.JAR", ".bundle-ready"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			digest := testBundle(t, root, "x", map[string]string{name: "bad"})
			if err := ensure(context.Background(), root, digest, nil, func(context.Context, string) error { t.Fatal("unsafe bundle reached probe"); return nil }); err == nil {
				t.Fatal("unsafe path accepted")
			}
		})
	}
	root := t.TempDir()
	digest := testBundle(t, root, "x", nil)
	f, err := os.OpenFile(filepath.Join(root, BundleName), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("corruption")
	f.Close()
	if err := Ensure(context.Background(), root, digest, nil); err == nil {
		t.Fatal("corrupt bundle accepted")
	}
}

func TestCancellationDoesNotPublishComponent(t *testing.T) {
	root := t.TempDir()
	digest := testBundle(t, root, "cancel", nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Ensure(ctx, root, digest, nil); err == nil {
		t.Fatal("cancel ignored")
	}
	if _, ok := ready.Load(managedDir(root, digest)); ok {
		t.Fatal("cancelled install ready")
	}
}
