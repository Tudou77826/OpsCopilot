package knowledge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAll(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "knowledge_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create dummy MD files
	files := map[string]string{
		"doc1.md":     "# Doc 1\nContent 1",
		"sub/doc2.md": "# Doc 2\nContent 2",
		"ignore.txt":  "Should be ignored",
	}

	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		err := os.MkdirAll(filepath.Dir(fullPath), 0755)
		if err != nil {
			t.Fatalf("Failed to create dir for %s: %v", path, err)
		}
		err = os.WriteFile(fullPath, []byte(content), 0644)
		if err != nil {
			t.Fatalf("Failed to write file %s: %v", path, err)
		}
	}

	// Test LoadAll
	result, err := LoadAll(tmpDir)
	if err != nil {
		t.Fatalf("LoadAll returned error: %v", err)
	}

	// Verify content
	expectedSubstrings := []string{
		"--- Document: doc1.md ---",
		"# Doc 1",
		"Content 1",
		"--- Document: sub/doc2.md ---",
		"# Doc 2",
		"Content 2",
	}

	for _, sub := range expectedSubstrings {
		if !strings.Contains(result, sub) {
			t.Errorf("Result missing substring: %s", sub)
		}
	}

	if strings.Contains(result, "Should be ignored") {
		t.Error("Result contains content from non-md file")
	}
}
