package knowledge

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// LoadAll reads all Markdown (.md) files from the specified directory
// and concatenates their content into a single string.
// It returns the concatenated content and any error encountered.
func LoadAll(dir string) (string, error) {
	var sb strings.Builder

	// Check if directory exists
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return "", fmt.Errorf("directory not found: %s", dir)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path is not a directory: %s", dir)
	}

	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-markdown files
		if d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", path, err)
		}

		// Add header and content
		// Use filepath.ToSlash to ensure consistent forward slashes in headers regardless of OS
		relPath, _ := filepath.Rel(dir, path)
		headerPath := filepath.ToSlash(relPath)
		sb.WriteString(fmt.Sprintf("\n--- Document: %s ---\n", headerPath))
		sb.Write(content)
		sb.WriteString("\n")

		return nil
	})

	if err != nil {
		return "", fmt.Errorf("failed to walk directory %s: %w", dir, err)
	}

	return sb.String(), nil
}
