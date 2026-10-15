package achievements

import (
	"encoding/json"
	social "opscopilot/pkg/companionsocial"
	"opscopilot/pkg/knowledge"
	"opscopilot/pkg/recorder"
	"opscopilot/pkg/script"
	"os"
	"path/filepath"
	"testing"
)

func TestArchiveAndCaseRecoveryReadActualBusinessFormats(t *testing.T) {
	s := enabledStore(t)
	root := t.TempDir()
	id := "archive-case"
	if _, err := s.Prepare(social.MedalKnowledgeArchive, id, true); err != nil {
		t.Fatal(err)
	}
	rel, err := knowledge.AppendRecord(root, &knowledge.ArchiveInput{Service: "service", Module: "module", Session: &recorder.RecordingSession{ID: id}, Conclusion: "## 根本原因\n根因\n\n## 排查路径\n定位过程"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Repair(&Sources{Knowledge: root}); err != nil {
		t.Fatal(err)
	}
	status, _ := s.Status()
	if status.Summary.Progress[0].Count != 1 || status.Repairs != 0 {
		t.Fatal("real archive marker not recognized", status)
	}
	if err = os.Remove(filepath.Join(root, rel)); err != nil {
		t.Fatal(err)
	}
	if err = s.Repair(&Sources{Knowledge: root}); err != nil {
		t.Fatal(err)
	}
	status, _ = s.Status()
	if status.Summary.Progress[0].Count != 0 {
		t.Fatal("archive deletion not retracted", status)
	}
	recRoot := filepath.Join(root, "recordings")
	rec := recorder.NewRecorder(recRoot)
	session := rec.StartSession("problem", nil)
	if _, err = s.Prepare(social.MedalCaseCompleted, session.ID, true); err != nil {
		t.Fatal(err)
	}
	if err = rec.StopSession("root cause", "conclusion"); err != nil {
		t.Fatal(err)
	}
	if err = s.Repair(&Sources{Knowledge: root, Recordings: recRoot}); err != nil {
		t.Fatal(err)
	}
	status, _ = s.Status()
	if status.Summary.Progress[3].Count != 1 || status.Repairs != 0 {
		t.Fatal("actual recorder format not recovered", status)
	}
}

func TestRepairDetectsCorruptionDeletionAndEligibility(t *testing.T) {
	s := enabledStore(t)
	dir := t.TempDir()
	id := "owned"
	sc := &script.Script{ID: id, Commands: []script.ScriptCommand{{Content: "echo hello", Enabled: true}}}
	path := filepath.Join(dir, "script_"+id+".json")
	data, _ := json.Marshal(sc)
	if err := s.Do(social.MedalScriptCreated, id, true, func() error { return os.WriteFile(path, data, 0600) }); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.Repair(&Sources{Scripts: dir}); err == nil {
		t.Fatal("corruption treated as deletion")
	}
	status, _ := s.Status()
	if status.Summary.Progress[1].Count != 1 {
		t.Fatal(status)
	}
	os.Remove(path)
	if err := s.Repair(&Sources{Scripts: dir}); err != nil {
		t.Fatal(err)
	}
	status, _ = s.Status()
	if status.Summary.Progress[1].Count != 0 {
		t.Fatal(status)
	}
	os.WriteFile(path, data, 0600)
	if err := s.Repair(&Sources{Scripts: dir}); err != nil {
		t.Fatal(err)
	}
	status, _ = s.Status()
	if status.Summary.Progress[1].Count != 1 {
		t.Fatal(status)
	}
}
