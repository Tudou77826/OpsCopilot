package knowledge

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Tool Definitions
const (
	ToolListFiles = "list_knowledge_files"
	ToolReadFile  = "read_knowledge_file"
	ToolSearch    = "search_knowledge"
)

func GetToolDefinitions() map[string]json.RawMessage {
	return map[string]json.RawMessage{
		ToolListFiles: json.RawMessage(`{
			"type": "object",
			"properties": {},
			"additionalProperties": false
		}`),
		ToolReadFile: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": {
					"type": "string",
					"description": "The relative path of the file to read, e.g., 'deploy/ssh.md'"
				}
			},
			"required": ["path"],
			"additionalProperties": false
		}`),
		ToolSearch: json.RawMessage(`{
			"type": "object",
			"properties": {
				"query": {
					"type": "string",
					"description": "Search query. Prefer short phrases or keywords."
				},
				"top_k": {
					"type": "integer",
					"description": "Number of results to return (1-20).",
					"minimum": 1,
					"maximum": 20
				}
			},
			"required": ["query"],
			"additionalProperties": false
		}`),
	}
}

// ListFiles returns a list of relative paths of all markdown files in the directory
func ListFiles(dir string) ([]string, error) {
	var files []string
	
	// Check if directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return []string{}, fmt.Errorf("directory not found: %s", dir)
	}

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		// Case insensitive check for .md
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return nil
		}
		files = append(files, filepath.ToSlash(relPath))
		return nil
	})
	return files, err
}

// ReadFile returns the content of a specific file
func ReadFile(baseDir, relPath string) (string, error) {
	// Security check: prevent directory traversal
	// filepath.Clean removes .. and resolves paths
	// We want to ensure the target file is inside baseDir
	
	cleanRel := filepath.ToSlash(filepath.Clean(relPath))
	// Prevent absolute paths or climbing up
	if strings.Contains(cleanRel, "..") || strings.HasPrefix(cleanRel, "/") {
		return "", fmt.Errorf("invalid file path: traversal detected")
	}

	fullPath := filepath.Join(baseDir, cleanRel)
	
	// Double check absolute paths
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", err
	}
	absFull, err := filepath.Abs(fullPath)
	if err != nil {
		return "", err
	}

	// Case-insensitive check on Windows is tricky, but HasPrefix is a good enough guard for now
	// Ensure absFull starts with absBase
	if !strings.HasPrefix(strings.ToLower(absFull), strings.ToLower(absBase)) {
		 return "", fmt.Errorf("access denied: path %s is outside knowledge base %s", absFull, absBase)
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}
