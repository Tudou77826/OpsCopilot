package script

import (
	"errors"
	"opscopilot/pkg/filetxn"
	"os"
	"path/filepath"
	"testing"
)

func TestSharedStaleScriptCannotOverwriteOrResurrect(t *testing.T) {
	dir := t.TempDir()
	a := NewManager(nil, dir, nil)
	b := NewManager(nil, dir, nil)
	created, err := a.CreateScript("original", "")
	if err != nil {
		t.Fatal(err)
	}
	stale, err := b.LoadScript(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	created.Name = "first"
	if err = a.UpdateScript(created); err != nil {
		t.Fatal(err)
	}
	stale.Name = "second"
	if !errors.Is(b.UpdateScript(stale), filetxn.ErrConflict) {
		t.Fatal("stale update allowed")
	}
	path := filepath.Join(dir, "script_"+created.ID+".json")
	if err = os.WriteFile(path, []byte("corrupted"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = b.UpdateScript(stale); err == nil {
		t.Fatal("corrupt file overwritten")
	}
	bytes, _ := os.ReadFile(path)
	if string(bytes) != "corrupted" {
		t.Fatal("corrupt file replaced")
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err = b.UpdateScript(stale); err == nil {
		t.Fatal("deleted script resurrected")
	}
}
