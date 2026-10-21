package assistantskills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLibraryImportUpdateDisableAndImmutableNativeSnapshot(t *testing.T) {
	source := t.TempDir()
	writeFixture(t, source, "SKILL.md", fixtureSkill)
	writeFixture(t, source, "scripts/lookup.py", "print('original')")
	writeFixture(t, source, "assets/client.jar", "binary\x00asset")
	l := Library{Directory: filepath.Join(t.TempDir(), "installed")}
	if err := l.Import(source, false); err != nil {
		t.Fatal(err)
	}
	if err := l.Import(source, false); err == nil {
		t.Fatal("silently replaced existing skill")
	}
	snapshot := t.TempDir()
	paths, rev, err := l.NativeSnapshot(snapshot)
	if err != nil || len(paths) != 1 {
		t.Fatal(paths, err)
	}
	if data, err := os.ReadFile(filepath.Join(paths[0], "scripts", "lookup.py")); err != nil || !strings.Contains(string(data), "original") {
		t.Fatal(string(data), err)
	}
	writeFixture(t, source, "scripts/lookup.py", "print('updated')")
	if err = l.ImportAs(source, true, "another-skill"); err == nil {
		t.Fatal("updated wrong named skill")
	}
	if err = l.ImportAs(source, true, "order-logs"); err != nil {
		t.Fatal(err)
	}
	next, newRev, err := l.NativeSnapshot(snapshot)
	if err != nil || newRev == rev || next[0] == paths[0] {
		t.Fatal("revision did not change", err)
	}
	old, _ := os.ReadFile(filepath.Join(paths[0], "scripts", "lookup.py"))
	if !strings.Contains(string(old), "original") {
		t.Fatal("mutated prior turn package")
	}
	if err = l.Enable("order-logs", false); err != nil {
		t.Fatal(err)
	}
	disabled, _, err := l.NativeSnapshot(snapshot)
	if err != nil || len(disabled) != 0 {
		t.Fatal(disabled, err)
	}
	state, err := l.Read()
	if err != nil || state.Skills[0].Enabled || len(state.Skills[0].Files) != 3 {
		t.Fatal(state, err)
	}
	if err = l.Enable("order-logs", true); err != nil {
		t.Fatal(err)
	}
	if err = l.Remove("../outside"); err == nil {
		t.Fatal("accepted traversal")
	}
	if err = l.Remove("order-logs"); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(source, "SKILL.md")); err != nil {
		t.Fatal("removed source package")
	}
}
func TestLibraryCommandValidationAndDependencies(t *testing.T) {
	source := t.TempDir()
	writeFixture(t, source, "SKILL.md", fixtureSkill)
	l := Library{Directory: t.TempDir()}
	if err := l.Import(source, false); err != nil {
		t.Fatal(err)
	}
	command := Command{Name: "sqlSessionInfoGetAndCreate", Description: "发现并打开数据库", Skill: "order-logs", Template: "请在 {{management}} 查看 {{business}}，再次核对 {{business}}", Fields: []Field{{"management", "管理面", "connection", true}, {"business", "业务", "text", true}}}
	if err := l.SaveCommand(command); err != nil {
		t.Fatal(err)
	}
	command.Description = "更新后的用途"
	if err := l.SaveCommand(command); err != nil {
		t.Fatal(err)
	}
	s, err := l.Read()
	if err != nil || len(s.Commands) != 1 || s.Commands[0].Description != command.Description {
		t.Fatal(s, err)
	}
	if err = l.Remove("order-logs"); err == nil {
		t.Fatal("removed a referenced skill")
	}
	for _, mutate := range []func(*Command){func(c *Command) { c.Name = "../bad" }, func(c *Command) { c.Template = "{{unknown}}" }, func(c *Command) { c.Fields = append(c.Fields, c.Fields[0]) }, func(c *Command) { c.Template += " {{bad-key}}" }, func(c *Command) { c.Fields[0].Kind = "password" }} {
		bad := command
		bad.Fields = append([]Field(nil), command.Fields...)
		mutate(&bad)
		if err = l.SaveCommand(bad); err == nil {
			t.Fatalf("invalid command accepted: %+v", bad)
		}
	}
	if err = l.RemoveCommand(command.Name); err != nil {
		t.Fatal(err)
	}
	if err = l.Remove("order-logs"); err != nil {
		t.Fatal(err)
	}
}
func TestLibraryRejectsSymlinksAndOversizedFiles(t *testing.T) {
	source := t.TempDir()
	writeFixture(t, source, "SKILL.md", fixtureSkill)
	f, err := os.Create(filepath.Join(source, "oversized.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate((8 << 20) + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	l := Library{Directory: t.TempDir()}
	if err = l.Import(source, false); err == nil {
		t.Fatal("oversized package accepted")
	}
	os.Remove(filepath.Join(source, "oversized.bin"))
	if err = os.Symlink(t.TempDir(), filepath.Join(source, "outside")); err != nil {
		t.Skip("symlink creation unavailable")
	}
	if err = l.Import(source, false); err == nil {
		t.Fatal("symlink accepted")
	}
}
