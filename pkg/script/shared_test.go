package script

import (
	"errors"
	"opscopilot/pkg/filetxn"
	"os"
	"path/filepath"
	"testing"
)

type emptyReplaySender struct{ calls int }

func (s *emptyReplaySender) SendCommand(string, string) error { s.calls++; return nil }
func TestEmptyReplayCannotReportDispatchedOrOverwriteIdentity(t *testing.T) {
	sender := &emptyReplaySender{}
	m := NewManager(nil, t.TempDir(), sender)
	sc, err := m.CreateScript("empty", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = m.ReplayScript(sc.ID, "terminal"); err == nil {
		t.Fatal("empty replay succeeded")
	}
	sc.Steps = []ScriptStep{{Command: "${EMPTY}", Enabled: true}}
	if err = m.UpdateScript(sc); err != nil {
		t.Fatal(err)
	}
	if err = m.ReplayScriptWithVars(sc.ID, "terminal", map[string]string{"EMPTY": " "}); err == nil {
		t.Fatal("blank replay succeeded")
	}
	if sender.calls != 0 {
		t.Fatal("blank commands sent")
	}
	if _, err = m.CreateScriptWithID(sc.ID, "overwritten", ""); err == nil {
		t.Fatal("identity overwritten")
	}
}

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
