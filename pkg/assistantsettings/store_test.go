package assistantsettings

import (
	"errors"
	"opscopilot/pkg/filetxn"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPersistenceResetAndConflictingEditors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "assistant.json")
	store := New(path)
	first, err := store.Read()
	if err != nil || first.EffectivePersona() != DefaultPersona {
		t.Fatalf("default: %+v %v", first, err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("reading created a preference file")
	}
	saved, err := store.Save("  你叫小圆，回答先给结论。  ", first.Revision)
	if err != nil {
		t.Fatal(err)
	}
	after, err := New(path).Read()
	if err != nil || after.PersonaPrompt != "你叫小圆，回答先给结论。" {
		t.Fatalf("restart: %+v %v", after, err)
	}
	if _, err = store.Save("stale", first.Revision); !errors.Is(err, filetxn.ErrConflict) {
		t.Fatalf("conflict: %v", err)
	}
	if _, err = store.Save(strings.Repeat("字", MaxPersonaLength+1), saved.Revision); err == nil {
		t.Fatal("accepted oversized prompt")
	}
	after, _ = store.Read()
	if after.Revision != saved.Revision {
		t.Fatal("rejected save modified preferences")
	}
	reset, err := store.Save("", saved.Revision)
	if err != nil || reset.EffectivePersona() != DefaultPersona {
		t.Fatalf("reset: %+v %v", reset, err)
	}
}
func TestCorruptionIsPreserved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "assistant.json")
	store := New(path)
	before, _ := store.Read()
	os.WriteFile(path, []byte("broken"), 0600)
	if _, err := store.Read(); err == nil {
		t.Fatal("silently defaulted corrupted preferences")
	}
	if _, err := store.Save("replacement", before.Revision); err == nil {
		t.Fatal("overwrote corruption")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "broken" {
		t.Fatal("damaged file was changed")
	}
}
