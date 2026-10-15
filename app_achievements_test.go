package main

import (
	"encoding/json"
	"opscopilot/pkg/achievements"
	"opscopilot/pkg/ai"
	"opscopilot/pkg/config"
	"opscopilot/pkg/recorder"
	"opscopilot/pkg/script"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopArchiveAlsoConfirmsSavedCase(t *testing.T) {
	root := t.TempDir()
	docs := filepath.Join(root, "docs")
	if err := os.MkdirAll(docs, 0700); err != nil {
		t.Fatal(err)
	}
	mgr := config.NewManagerWithDir(root)
	mgr.Config.Docs.Dir = docs
	store := achievements.NewDormant(filepath.Join(root, "achievements.json"))
	store.SetEnabled(true)
	app := &App{configMgr: mgr, achievements: store, coreRecorder: recorder.NewRecorder(filepath.Join(root, "recordings")), aiService: ai.NewAIService(nil, nil, mgr)}
	for _, conclusion := range []string{"first valid conclusion", "second valid conclusion"} {
		app.StartSession("local archive regression: " + conclusion)
		var response struct {
			Success bool `json:"success"`
		}
		if err := json.Unmarshal([]byte(app.ArchiveSession("confirmed cause", conclusion, "local", "module", "")), &response); err != nil || !response.Success {
			t.Fatalf("archive: %v %+v", err, response)
		}
	}
	state, err := store.Status()
	if err != nil {
		t.Fatal(err)
	}
	if state.Summary.Progress[0].Count != 2 || state.Summary.Progress[3].Count != 2 {
		t.Fatalf("archive must confirm both archive and successfully saved case: %+v", state.Summary.Progress)
	}
	content, err := os.ReadFile(filepath.Join(docs, "archive", "local_module.md"))
	if err != nil || !strings.Contains(string(content), "first valid conclusion") || !strings.Contains(string(content), "second valid conclusion") {
		t.Fatalf("archive lost content: %v %s", err, content)
	}
}

func TestDesktopScriptCollectionAndKnowledgeScope(t *testing.T) {
	root := t.TempDir()
	store := achievements.NewDormant(filepath.Join(root, "achievements.json"))
	store.SetEnabled(true)
	app := &App{scriptMgr: script.NewManager(nil, filepath.Join(root, "scripts"), nil), achievements: store}
	sc, err := app.CreateScript("own", "")
	if err != nil {
		t.Fatal(err)
	}
	sc.Commands = []script.ScriptCommand{{Content: "echo own", Enabled: true}}
	if err = app.UpdateScript(sc); err != nil {
		t.Fatal(err)
	}
	app.recordKnowledgeRead(filepath.Join(root, "library-a"), "doc.md", "content")
	app.recordKnowledgeRead(filepath.Join(root, "library-a"), "./doc.md", "same scenario")
	app.recordKnowledgeRead(filepath.Join(root, "library-b"), "doc.md", "different library")
	app.recordKnowledgeRead(filepath.Join(root, "library-b"), "empty.md", " ")
	status, _ := store.Status()
	if status.Summary.Progress[1].Count != 1 || status.Summary.Progress[4].Count != 2 {
		t.Fatal(status)
	}
	if err = app.DeleteScript(sc.ID); err != nil {
		t.Fatal(err)
	}
	status, _ = store.Status()
	if status.Summary.Progress[1].Count != 0 {
		t.Fatal(status)
	}
}
