package filetxn

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadBoundedRejectsOversizedFileAndDirectory(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "state")
	if err := os.WriteFile(p, []byte("valid"), 0600); err != nil {
		t.Fatal(err)
	}
	b, err := ReadBounded(p, 5)
	if err != nil || string(b) != "valid" {
		t.Fatalf("read: %q %v", b, err)
	}
	if _, err = ReadBounded(p, 4); err == nil {
		t.Fatal("accepted oversized file")
	}
	if _, err = ReadBounded(root, 256); err == nil {
		t.Fatal("accepted directory")
	}
}
