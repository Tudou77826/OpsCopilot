package assistantlocal

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestProjectGroupsPersistAndDoNotMoveDirectories(t *testing.T) {
	file := filepath.Join(t.TempDir(), "assistant-projects.json")
	store := &Projects{File: file}
	a, err := store.Create("A项目")
	if err != nil {
		t.Fatal(err)
	}
	aID := a.ActiveID
	aPaths := []string{t.TempDir(), t.TempDir(), t.TempDir()}
	if _, err = store.Add(aID, aPaths); err != nil {
		t.Fatal(err)
	}
	b, err := store.Create("B项目")
	if err != nil {
		t.Fatal(err)
	}
	bID := b.ActiveID
	bPath := t.TempDir()
	if _, err = store.Add(bID, []string{bPath}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Select(aID); err != nil {
		t.Fatal(err)
	}
	reloaded := &Projects{File: file}
	state, err := reloaded.Snapshot()
	if err != nil || state.ActiveID != aID || len(state.Projects) != 2 || len(state.Projects[0].Directories) != 3 || len(state.Projects[1].Directories) != 1 {
		t.Fatal(state, err)
	}
	state.Projects[0].Directories[0] = "tampered"
	check, _ := reloaded.Snapshot()
	if check.Projects[0].Directories[0] == "tampered" {
		t.Fatal("snapshot aliases state")
	}
	if _, err = reloaded.Rename(bID, "B改名"); err != nil {
		t.Fatal(err)
	}
	if _, err = reloaded.Delete(aID); err != nil {
		t.Fatal(err)
	}
	state, _ = store.Snapshot()
	if state.ActiveID != "" || len(state.Projects) != 1 || state.Projects[0].ID != bID || state.Projects[0].Name != "B改名" {
		t.Fatal(state)
	}
	for _, p := range aPaths {
		if _, err = os.Stat(p); err != nil {
			t.Fatal("deleting project removed a real directory", err)
		}
	}
	if _, err = store.Add(aID, []string{t.TempDir()}); err == nil {
		t.Fatal("deleted project silently redirected")
	}
}

func TestProjectValidationIsAtomicAndMissingDirectoriesAreRetained(t *testing.T) {
	store := &Projects{File: filepath.Join(t.TempDir(), "projects.json")}
	s, _ := store.Create("A")
	id := s.ActiveID
	root := t.TempDir()
	file := filepath.Join(root, "file.txt")
	if err := os.WriteFile(file, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"relative", file, filepath.Join(root, "missing"), root + "\n"} {
		if _, err := store.Add(id, []string{root, bad}); err == nil {
			t.Fatal("accepted invalid batch", bad)
		}
	}
	s, _ = store.Snapshot()
	if len(s.Projects[0].Directories) != 0 {
		t.Fatal("partial batch persisted")
	}
	if _, err := store.Add(id, []string{root, filepath.Join(root, ".")}); err != nil {
		t.Fatal(err)
	}
	s, _ = store.Snapshot()
	if len(s.Projects[0].Directories) != 1 {
		t.Fatal("duplicate canonical directory")
	}
	link := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, link); err == nil {
		if _, err = store.Add(id, []string{link}); err != nil {
			t.Fatal(err)
		}
		s, _ = store.Snapshot()
		if len(s.Projects[0].Directories) != 1 {
			t.Fatal("symlink not deduped")
		}
	}
	missing := filepath.Join(t.TempDir(), "temporary")
	if err := os.Mkdir(missing, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add(id, []string{missing}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(missing); err != nil {
		t.Fatal(err)
	}
	s, err := store.Snapshot()
	if err != nil || len(s.Projects[0].Directories) != 2 {
		t.Fatal("missing mount discarded saved config", s, err)
	}
	if _, err = store.Create("a"); err == nil {
		t.Fatal("duplicate name accepted")
	}
	if _, err = store.Create("\n"); err == nil {
		t.Fatal("invalid name accepted")
	}
}

func TestProjectStoresSerializeEditsAndDoNotOverwriteCorruptData(t *testing.T) {
	file := filepath.Join(t.TempDir(), "projects.json")
	a, b := &Projects{File: file}, &Projects{File: file}
	state, err := a.Create("A")
	if err != nil {
		t.Fatal(err)
	}
	id := state.ActiveID
	var wg sync.WaitGroup
	for _, store := range []*Projects{a, b} {
		wg.Add(1)
		go func(s *Projects) {
			defer wg.Done()
			if _, err := s.Add(id, []string{t.TempDir()}); err != nil {
				t.Error(err)
			}
		}(store)
	}
	wg.Wait()
	state, err = a.Snapshot()
	if err != nil || len(state.Projects[0].Directories) != 2 {
		t.Fatal("lost concurrent edit", state, err)
	}
	for _, corrupt := range []string{"broken", "null", `{"projects":[]}`, `{"version":2,"projects":[]}`} {
		if err = os.WriteFile(file, []byte(corrupt), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = b.Create("new"); err == nil {
			t.Fatal("corrupt config overwritten", corrupt)
		}
		data, _ := os.ReadFile(file)
		if string(data) != corrupt {
			t.Fatal("corrupt file changed")
		}
	}
}

func TestProjectAndDirectoryLimitsAreAtomic(t *testing.T) {
	store := &Projects{}
	state, _ := store.Create("A")
	id := state.ActiveID
	paths := []string{}
	for i := 0; i < MaxDirectories; i++ {
		paths = append(paths, t.TempDir())
	}
	if _, err := store.Add(id, paths); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add(id, paths); err != nil {
		t.Fatal("duplicate batch rejected", err)
	}
	if _, err := store.Add(id, []string{t.TempDir()}); err == nil {
		t.Fatal("directory limit bypassed")
	}
	state, _ = store.Snapshot()
	if len(state.Projects[0].Directories) != MaxDirectories {
		t.Fatal("failed mutation partially stored")
	}
	for i := 1; i < MaxProjects; i++ {
		if _, err := store.Create(fmt.Sprintf("P%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.Create("overflow"); err == nil {
		t.Fatal("project limit bypassed")
	}
}
