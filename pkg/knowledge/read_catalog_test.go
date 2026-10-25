package knowledge

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReadCatalogUsesExistingParserWithoutChangingIndex(t *testing.T) {
	dir := t.TempDir()
	body, err := os.ReadFile(filepath.Join("testdata", "payment_sop.md"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "payments.md")
	if err = os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	read, err := ReadCatalog(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(dir, catalogFileName)); !os.IsNotExist(err) {
		t.Fatal("read persisted catalog", err)
	}
	built, err := BuildCatalog(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(read.Services, built.Services) {
		t.Fatal("read differs from existing catalog parser")
	}
	before, err := os.ReadFile(filepath.Join(dir, catalogFileName))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	read, err = ReadCatalog(dir)
	if err != nil || read.TotalScenarios() != 0 {
		t.Fatal("read used stale persisted catalog", err)
	}
	after, err := os.ReadFile(filepath.Join(dir, catalogFileName))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("read changed existing index")
	}
}

func TestReadCatalogDoesNotFollowLinkedMarkdown(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("---\nservice: outside\nmodule: private\n---\n\n## 场景：外部资料\nprivate\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "linked.md")); err != nil {
		t.Skip("symbolic link unavailable on this host")
	}
	cat, err := ReadCatalog(dir)
	if err != nil || cat.TotalScenarios() != 0 {
		t.Fatal("followed external knowledge symlink", err)
	}
}
