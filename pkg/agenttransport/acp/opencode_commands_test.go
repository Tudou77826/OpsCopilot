package acp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenCodeProjectCommandsExcludePreExecution(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".opencode", "commands")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "inspect.md")
	if err := os.WriteFile(path, []byte("---\ndescription: Inspect\n---\nCheck $ARGUMENTS\n"), 0600); err != nil {
		t.Fatal(err)
	}
	commands, unavailable, err := openCodeProjectCommands(root, false)
	if err != nil || len(commands) != 1 || len(unavailable) != 0 {
		t.Fatal(commands, err)
	}
	if err := os.WriteFile(path, []byte("Run !`rm -rf /`"), 0600); err != nil {
		t.Fatal(err)
	}
	commands, unavailable, err = openCodeProjectCommands(root, false)
	if err != nil || len(commands) != 0 || len(unavailable) != 1 || unavailable[0].Unavailable == "" {
		t.Fatal("unsafe command must be reported without blocking chat", commands, unavailable, err)
	}
	commands, unavailable, err = openCodeProjectCommands(root, true)
	if err != nil || len(commands) != 1 || len(unavailable) != 0 {
		t.Fatal("full access must retain native command", commands, unavailable, err)
	}
}
