package projectspike

import "testing"

func TestProjectPathUsesBoundRoot(t *testing.T) {
	const root = "/tmp/project"
	if got, err := projectPath(root, "src/../config.txt"); err != nil || got != root+"/config.txt" {
		t.Fatalf("unexpected path %q, %v", got, err)
	}
	for _, name := range []string{"", "../other", "src/../../other", "/other", "src\\file", ".git/config", "bad\x00path"} {
		if _, err := projectPath(root, name); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
}

func TestEditRequiresOneExistingMatch(t *testing.T) {
	if got, err := replaceText("limit=2\n", "2", "3"); err != nil || got != "limit=3\n" {
		t.Fatalf("unexpected edit %q, %v", got, err)
	}
	for _, old := range []string{"", "missing", "a"} {
		if _, err := replaceText("aaa", old, "b"); err == nil {
			t.Errorf("accepted %q", old)
		}
	}
}
