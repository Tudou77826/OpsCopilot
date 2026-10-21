package assistantskills

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeFixture(t *testing.T, root, name, content string) {
	t.Helper()
	filename := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

const fixtureSkill = "---\nname: order-logs\ndescription: Locate order detail failures.\n---\nRead references/deployment.md before inspecting the service.\n"

func TestCatalogSnapshotsOnlyExplicitText(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "order-logs/SKILL.md", fixtureSkill)
	writeFixture(t, root, "order-logs/references/deployment.md", "service: orders-reader")
	writeFixture(t, root, "order-logs/scripts/escape.py", "PRIVATE_SCRIPT")
	writeFixture(t, root, "order-logs/references/data.bin", "PRIVATE_BINARY")
	got, err := Load(root)
	if err != nil || len(got.Skills) != 1 || len(got.Skills[0].Resources) != 1 {
		t.Fatal(got, err)
	}
	if got.Skills[0].Resources[0].Name != "references/deployment.md" {
		t.Fatal(got)
	}
	data, _ := json.Marshal(got.Skills)
	for _, forbidden := range []string{"PRIVATE_SCRIPT", "PRIVATE_BINARY", root, `"path"`, `"scripts"`} {
		if strings.Contains(string(data), forbidden) {
			t.Fatal("catalog exposed a path or executable", forbidden)
		}
	}
	writeFixture(t, root, "order-logs/references/deployment.md", "changed")
	next, err := Load(root)
	if err != nil || next.Revision == got.Revision || got.Skills[0].Resources[0].Content != "service: orders-reader" {
		t.Fatal("snapshot not independent", err)
	}
}

func TestCatalogRejectsInvalidAndOversizeText(t *testing.T) {
	for name, content := range map[string]string{
		"frontmatter":     "# no metadata",
		"mismatched-name": strings.Replace(fixtureSkill, "order-logs", "other", 1),
		"empty-body":      "---\nname: order-logs\ndescription: test\n---\n",
		"invalid-utf8":    fixtureSkill + string([]byte{0xff}),
		"too-large":       fixtureSkill + strings.Repeat("x", maxFileBytes),
		"duplicate-field": "---\nname: order-logs\nname: other\ndescription: test\n---\nbody",
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeFixture(t, root, "order-logs/SKILL.md", content)
			if _, err := Load(root); err == nil {
				t.Fatal("invalid catalog accepted")
			}
		})
	}
}
func TestCatalogRejectsLinkedReferences(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeFixture(t, root, "order-logs/SKILL.md", fixtureSkill)
	link := filepath.Join(root, "order-logs", "references")
	if err := os.Symlink(outside, link); err != nil {
		if runtime.GOOS != "windows" {
			t.Skip("symlink creation unavailable")
		}
		// Windows junctions need no symlink privilege. Both targets belong to
		// this test's temporary directories; cleanup removes only the link.
		if err := exec.Command("cmd", "/c", "mklink", "/J", link, outside).Run(); err != nil {
			t.Fatal("cannot exercise Windows linked-directory boundary", err)
		}
	}
	if _, err := Load(root); err == nil {
		t.Fatal("linked reference accepted")
	}
}
func TestMissingCatalogDoesNotPreventChat(t *testing.T) {
	for _, directory := range []string{"", filepath.Join(t.TempDir(), "not-created")} {
		got, err := Load(directory)
		if err != nil || len(got.Skills) != 0 {
			t.Fatal(got, err)
		}
	}
}

func TestCatalogEnforcesResourceAndTotalBudgets(t *testing.T) {
	for _, oversizedFiles := range []bool{false, true} {
		root := t.TempDir()
		writeFixture(t, root, "order-logs/SKILL.md", fixtureSkill)
		count, content := maxResources+1, "reference"
		if oversizedFiles {
			count, content = 9, strings.Repeat("x", maxFileBytes)
		}
		for i := 0; i < count; i++ {
			writeFixture(t, root, "order-logs/references/"+string(rune('a'+i))+".md", content)
		}
		if _, err := Load(root); err == nil {
			t.Fatal("oversized catalog accepted", oversizedFiles)
		}
	}
}
