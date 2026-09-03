package shellsidecar

import (
	"opscopilot/pkg/config"
	"opscopilot/pkg/remote"
	"opscopilot/pkg/sessionmanager"
	"os"
	"path/filepath"
	"testing"
)

func TestDesktopPathsAndSettingsShareExistingFiles(t *testing.T) {
	root := t.TempDir()
	m := config.NewManagerWithDir(root)
	if e := m.Load(); e != nil {
		t.Fatal(e)
	}
	m.Config.Log.Dir = "custom-logs"
	m.Config.Scripts.Dir = "custom-scripts"
	m.Config.Appearance.Theme = "light"
	m.Config.LLM.APIKey = "fixture-secret"
	if e := m.Save(); e != nil {
		t.Fatal(e)
	}
	paths, e := ResolveDesktopData(root)
	if e != nil {
		t.Fatal(e)
	}
	if paths.Scripts != filepath.Join(root, "custom-scripts") || paths.Recordings != filepath.Join(root, "custom-logs", "recordings") {
		t.Fatalf("bad paths: %+v", paths)
	}
	d := &desktopSettings{root: root, snapshots: map[string]*config.Manager{}}
	settings, e := d.get()
	if e != nil {
		t.Fatal(e)
	}
	if settings.Theme != "light" {
		t.Fatal("did not reuse theme")
	}
	m.Config.LLM.ComplexModel = "other-window"
	if e = m.Save(); e != nil {
		t.Fatal(e)
	}
	settings.Theme = "dark"
	if e = d.save(settings); e != nil {
		t.Fatal(e)
	}
	verify := config.NewManagerWithDir(root)
	verify.SetReadOnly(true)
	if e = verify.Load(); e != nil {
		t.Fatal(e)
	}
	if verify.Config.LLM.APIKey != "fixture-secret" || verify.Config.LLM.ComplexModel != "other-window" || verify.Config.Appearance.Theme != "dark" {
		t.Fatal("local config fields overwritten")
	}
	if _, e = os.Stat(filepath.Join(root, "shell-settings.json")); !os.IsNotExist(e) {
		t.Fatal("created plugin settings copy")
	}
	ai := &AIConfigService{desktopRoot: root}
	if !ai.Status().Configured || ai.load().APIKey != "fixture-secret" {
		t.Fatal("AI does not read desktop config")
	}
}
func TestSharedSessionMutationPreservesCredentialsAndExternalEdits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	desktop := sessionmanager.NewManagerWithPath(path)
	if e := desktop.Upsert(remote.ConnectConfig{Host: "fixture", Port: 22, User: "test", Password: "private", RootPassword: "root"}, ""); e != nil {
		t.Fatal(e)
	}
	plugin, e := NewConfigServiceWithPath(path)
	if e != nil {
		t.Fatal(e)
	}
	plugin.mgr.PreserveCredentials = true
	if e = desktop.CreateFolder("outside"); e != nil {
		t.Fatal(e)
	}
	nodes, e := plugin.List()
	if e != nil {
		t.Fatal(e)
	}
	if len(nodes) != 2 {
		t.Fatal("not refreshed")
	}
	if e = plugin.Update(nodes[0].ID, remote.ConnectConfig{Host: "fixture", Port: 22, User: "test", HostKey: "pin"}, ""); e != nil {
		t.Fatal(e)
	}
	desktop.Load()
	nodes2 := desktop.GetSessions()
	if len(nodes2) != 2 {
		t.Fatal("external folder lost")
	}
	for _, n := range nodes2 {
		if n.Config != nil && (n.Config.Password != "private" || n.Config.RootPassword != "root") {
			t.Fatal("saved secrets erased")
		}
	}
}
