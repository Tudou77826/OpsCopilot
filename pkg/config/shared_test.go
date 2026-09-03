package config

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"opscopilot/pkg/sessionmanager"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSharedProcessHelper(t *testing.T) {
	dir := os.Getenv("OPS_SHARED_STORE_TEST_DIR")
	if dir == "" {
		return
	}
	id := os.Getenv("OPS_SHARED_STORE_TEST_ID")
	m := NewManagerWithDir(dir)
	m.SetReadOnly(true)
	if err := m.Load(); err != nil {
		t.Fatal(err)
	}
	sessions := sessionmanager.NewManagerWithPath(filepath.Join(dir, "sessions.json"))
	if err := sessions.Load(); err != nil {
		t.Fatal(err)
	}
	fmt.Println("shared-ready")
	_, _ = io.Copy(io.Discard, os.Stdin)
	if id == "0" {
		m.Config.Appearance.Theme = "light"
	}
	if id == "1" {
		m.Config.CompletionDelay = 222
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		key := fmt.Sprintf("%s-%d", id, i)
		if err := m.AddQuickCommand(QuickCommand{ID: key, Name: key, Content: "pwd"}); err != nil {
			t.Fatal(err)
		}
		if err := sessions.CreateFolder(key); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSharedProcessesPreserveSettingsAndAssetMutations(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"appearance":{"theme":"dark"},"completion_delay":150,"unknown_future_field":{"keep":true},"llm":{"APIKey":"fixture-key"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	type child struct {
		cmd    *exec.Cmd
		in     io.WriteCloser
		output *bytes.Buffer
	}
	var children []child
	for i := 0; i < 4; i++ {
		cmd := exec.Command(os.Args[0], "-test.run=^TestSharedProcessHelper$")
		cmd.Env = append(os.Environ(), "OPS_SHARED_STORE_TEST_DIR="+dir, fmt.Sprintf("OPS_SHARED_STORE_TEST_ID=%d", i))
		in, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		out, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		stderr := new(bytes.Buffer)
		cmd.Stderr = stderr
		if err = cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		scanner := bufio.NewScanner(out)
		ready := false
		for scanner.Scan() {
			if scanner.Text() == "shared-ready" {
				ready = true
				break
			}
		}
		if !ready {
			t.Fatal("helper failed", stderr.String())
		}
		children = append(children, child{cmd, in, stderr})
	}
	for _, c := range children {
		_ = c.in.Close()
	}
	for _, c := range children {
		if err := c.cmd.Wait(); err != nil {
			t.Fatal(err, c.output.String())
		}
	}
	m := NewManagerWithDir(dir)
	m.SetReadOnly(true)
	if err := m.Load(); err != nil {
		t.Fatal(err)
	}
	if m.Config.Appearance.Theme != "light" || m.Config.CompletionDelay != 222 || m.Config.LLM.APIKey != "fixture-key" {
		t.Fatal("lost unrelated settings")
	}
	if len(m.Config.QuickCommands) != 48 {
		t.Fatalf("lost commands: %d", len(m.Config.QuickCommands))
	}
	sessions := sessionmanager.NewManagerWithPath(filepath.Join(dir, "sessions.json"))
	if err := sessions.Load(); err != nil {
		t.Fatal(err)
	}
	if len(sessions.GetSessions()) != 48 {
		t.Fatal("lost session folders")
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "config.json"))
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	if value["unknown_future_field"] == nil {
		t.Fatal("lost unknown fields")
	}
}

func TestSharedManagersMergeAndQuickIntents(t *testing.T) {
	dir := t.TempDir()
	a := NewManagerWithDir(dir)
	if e := a.Load(); e != nil {
		t.Fatal(e)
	}
	b := NewManagerWithDir(dir)
	if e := b.Load(); e != nil {
		t.Fatal(e)
	}
	a.Config.Appearance.Theme = "light"
	if e := a.Save(); e != nil {
		t.Fatal(e)
	}
	b.Config.LLM.APIKey = "fixture-key"
	if e := b.Save(); e != nil {
		t.Fatal(e)
	}
	c := NewManagerWithDir(dir)
	c.SetReadOnly(true)
	if e := c.Load(); e != nil {
		t.Fatal(e)
	}
	if c.Config.Appearance.Theme != "light" || c.Config.LLM.APIKey != "fixture-key" {
		t.Fatal("unrelated settings lost")
	}
	if e := a.AddQuickCommand(QuickCommand{ID: "a", Name: "a", Content: "pwd"}); e != nil {
		t.Fatal(e)
	}
	if e := b.AddQuickCommand(QuickCommand{ID: "b", Name: "b", Content: "ls"}); e != nil {
		t.Fatal(e)
	}
	if !a.DeleteQuickCommand("a") {
		t.Fatal("delete")
	}
	if !b.UpdateQuickCommand("b", QuickCommand{Name: "updated", Content: "ls -l"}) {
		t.Fatal("update")
	}
	if e := c.Load(); e != nil {
		t.Fatal(e)
	}
	if len(c.Config.QuickCommands) != 1 || c.Config.QuickCommands[0].Name != "updated" {
		t.Fatal("lost quick command edits")
	}
}
func TestReadOnlyLoadDoesNotCreateAuxiliaryFiles(t *testing.T) {
	dir := t.TempDir()
	if e := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"appearance":{"theme":"light"}}`), 0600); e != nil {
		t.Fatal(e)
	}
	m := NewManagerWithDir(dir)
	m.SetReadOnly(true)
	if e := m.Load(); e != nil {
		t.Fatal(e)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("readonly created files: %v", entries)
	}
}

func TestSharedQuickCommandDeletedIDCannotBeRestoredByStaleEdit(t *testing.T) {
	dir := t.TempDir()
	a, b := NewManagerWithDir(dir), NewManagerWithDir(dir)
	if err := a.Load(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b"} {
		if err := a.AddQuickCommand(QuickCommand{ID: id, Name: id, Content: "pwd"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.Load(); err != nil {
		t.Fatal(err)
	}
	if !a.DeleteQuickCommand("a") {
		t.Fatal("delete failed")
	}
	if b.UpdateQuickCommand("a", QuickCommand{Name: "stale", Content: "ls"}) {
		t.Fatal("stale edit recreated deleted ID")
	}
	if b.ReorderQuickCommands([]string{"b", "a"}) {
		t.Fatal("stale order containing deleted ID was accepted")
	}
	if err := a.Load(); err != nil {
		t.Fatal(err)
	}
	if len(a.Config.QuickCommands) != 1 || a.Config.QuickCommands[0].ID != "b" {
		t.Fatal("failed mutation changed stored commands")
	}
}
