package shellsidecar

import (
	"opscopilot/pkg/achievements"
	"opscopilot/pkg/script"
	"path/filepath"
	"testing"
)

func TestSocialCollectorsTrackOwnScriptsCommandsAndWithdrawals(t *testing.T) {
	root := t.TempDir()
	store := achievements.NewDormant(filepath.Join(root, "achievements.json"))
	store.SetEnabled(true)
	svc := NewTerminalService("test")
	scripts, err := NewStructuredScriptService(svc, root)
	if err != nil {
		t.Fatal(err)
	}
	defer scripts.Close()
	scripts.achievements = store
	legacy, err := scripts.mgr.CreateScript("imported", "")
	if err != nil {
		t.Fatal(err)
	}
	legacy.Steps = []script.ScriptStep{{Command: "echo imported", Enabled: true}}
	if err = scripts.Update(legacy); err != nil {
		t.Fatal(err)
	}
	own, err := scripts.Create("own", "")
	if err != nil {
		t.Fatal(err)
	}
	status, _ := store.Status()
	if status.Summary.Progress[1].Count != 0 {
		t.Fatal("empty or imported script counted")
	}
	own.Steps = []script.ScriptStep{{Command: "echo own", Enabled: true}}
	if err = scripts.Update(own); err != nil {
		t.Fatal(err)
	}
	commands, err := NewQuickCmdService(root)
	if err != nil {
		t.Fatal(err)
	}
	commands.achievements = store
	cmdID, err := commands.Save(QuickCommand{Name: "command", Content: "echo command"})
	if err != nil {
		t.Fatal(err)
	}
	status, _ = store.Status()
	if status.Summary.Progress[1].Count != 1 || status.Summary.Progress[2].Count != 1 {
		t.Fatal(status)
	}
	if err = scripts.Delete(own.ID); err != nil {
		t.Fatal(err)
	}
	if err = commands.Delete(cmdID); err != nil {
		t.Fatal(err)
	}
	status, _ = store.Status()
	if status.Summary.Progress[1].Count != 0 || status.Summary.Progress[2].Count != 0 {
		t.Fatal("deletion did not retract", status)
	}
}
