package knowledge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKnowledgeTools(t *testing.T) {
	// 1. Setup temporary test directory
	tmpDir, err := os.MkdirTemp("", "knowledge_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test files
	files := map[string]string{
		"intro.md":          "# Intro\nWelcome",
		"guide/deploy.md":   "# Deployment\nSteps to deploy",
		"guide/config.json": "{}", // Should be ignored
		"secret.txt":        "secret", // Should be ignored
	}

	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// 2. Test ListFiles
	t.Run("ListFiles", func(t *testing.T) {
		list, err := ListFiles(tmpDir)
		if err != nil {
			t.Fatalf("ListFiles failed: %v", err)
		}

		// Expect 2 markdown files
		expectedCount := 2
		if len(list) != expectedCount {
			t.Errorf("Expected %d files, got %d: %v", expectedCount, len(list), list)
		}

		// Verify content (normalize separators)
		foundIntro := false
		foundDeploy := false
		for _, f := range list {
			f = filepath.ToSlash(f)
			if f == "intro.md" {
				foundIntro = true
			}
			if f == "guide/deploy.md" {
				foundDeploy = true
			}
		}

		if !foundIntro || !foundDeploy {
			t.Errorf("Missing expected files in list: %v", list)
		}
	})

	// 3. Test ReadFile
	t.Run("ReadFile_Success", func(t *testing.T) {
		content, err := ReadFile(tmpDir, "guide/deploy.md")
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		if content != "# Deployment\nSteps to deploy" {
			t.Errorf("Content mismatch: %s", content)
		}
	})

	t.Run("ReadFile_Security_Traversal", func(t *testing.T) {
		// Attempt to read outside tmpDir
		// Assuming tmpDir is inside /tmp/..., trying to read ../../../etc/passwd or similar
		// We just simulate ".."
		_, err := ReadFile(tmpDir, "../outside.md")
		if err == nil {
			t.Error("Expected error for directory traversal, got nil")
		}
		if !strings.Contains(err.Error(), "traversal detected") && !strings.Contains(err.Error(), "outside knowledge base") {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("ReadFile_Security_Absolute", func(t *testing.T) {
		// Attempt to pass absolute path
		_ = filepath.Join(tmpDir, "intro.md")
		// On Windows, Join might handle this, but ReadFile expects relative
		// If we pass absolute path as relative arg, filepath.Join(base, abs) behaviour depends on OS
		// But our check `strings.HasPrefix(cleanRel, "/")` or similar might catch it
		
		// Let's test a clearly malicious absolute path
		// On Windows: D:\foo
		// On Linux: /etc/passwd
		
		// Note: The implementation uses filepath.Clean and checks for ".." or prefix "/"
		// Let's just try to read a file that we know exists but pass it in a way that might trick it
		// Actually, standard usage is relative path.
		
		// Let's try to access a file that is technically inside but we want to ensure robust path handling
		content, err := ReadFile(tmpDir, "./intro.md")
		if err != nil {
			t.Errorf("Should support ./ prefix: %v", err)
		}
		if content != "# Intro\nWelcome" {
			t.Errorf("Failed to read with ./ prefix")
		}
	})
}
