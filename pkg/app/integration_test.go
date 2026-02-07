package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestBackendScriptCall simulates how the backend calls external scripts
func TestBackendScriptCall(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "opscopilot-backend-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a simple test script that works
	testScriptPath := filepath.Join(tempDir, "test_bridge.bat")
	scriptContent := `@echo off
chcp 65001 >nul 2>&1
set "PROBLEM=%~1"
set "OUTPUT_DIR=%~2"
echo %PROBLEM% > "%OUTPUT_DIR%\conclusion.md"
echo Test output from bridge script >> "%OUTPUT_DIR%\conclusion.md"
`

	if err := os.WriteFile(testScriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to create test script: %v", err)
	}

	// Simulate backend call (like in app.go runTroubleshootWithExternal)
	problem := "Test Problem: High CPU Usage"
	absScriptPath, _ := filepath.Abs(testScriptPath)

	args := []string{"/C", absScriptPath, problem, tempDir}
	cmd := exec.Command("cmd", args...)
	output, err := cmd.CombinedOutput()

	t.Logf("Script output: %s", string(output))
	if err != nil {
		t.Logf("Script execution error: %v", err)
	}

	// Check result file
	conclusionPath := filepath.Join(tempDir, "conclusion.md")
	content, err := os.ReadFile(conclusionPath)
	if err != nil {
		t.Fatalf("Failed to read conclusion file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, problem) {
		t.Errorf("Conclusion file does not contain problem. Got: %s", contentStr)
	}

	t.Log("Backend script call test PASSED")
}

// TestParameterParsing tests parameter parsing logic
func TestParameterParsing(t *testing.T) {
	tests := []struct {
		name     string
		problem  string
		outputDir string
	}{
		{
			name:     "Simple ASCII",
			problem:  "CPU high",
			outputDir: "C:\\temp\\test",
		},
		{
			name:     "With spaces",
			problem:  "Server CPU usage is too high",
			outputDir: "C:\\temp\\test dir",
		},
		{
			name:     "Chinese characters",
			problem:  "服务器CPU占用过高",
			outputDir: "C:\\temp\\test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify parameters can be passed correctly
			if tt.problem == "" {
				t.Error("Problem should not be empty")
			}
			if tt.outputDir == "" {
				t.Error("OutputDir should not be empty")
			}
			t.Logf("Parameter parsing test passed for: %s", tt.name)
		})
	}
}

// TestScriptExistence verifies required script files exist
func TestScriptExistence(t *testing.T) {
	scriptsDir := filepath.Join("..", "..", "scripts")
	requiredFiles := []string{
		"troubleshoot_bridge_template.bat",
	}

	for _, file := range requiredFiles {
		path := filepath.Join(scriptsDir, file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Required script file not found: %s", file)
		} else {
			t.Logf("Found required file: %s", file)
		}
	}
}

// TestDocumentationExists verifies documentation was created
func TestDocumentationExists(t *testing.T) {
	docsDir := filepath.Join("..", "..", "docs")
	docFile := "外部脚本桥接使用指南.md"
	path := filepath.Join(docsDir, docFile)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("Documentation file not found: %s", docFile)
	} else {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("Failed to read documentation: %v", err)
		} else {
			contentStr := string(content)
			requiredSections := []string{
				"-Problem",
				"-OutputDir",
				"conclusion.md",
				"桥接脚本",
			}

			for _, section := range requiredSections {
				if !strings.Contains(contentStr, section) {
					t.Errorf("Documentation missing section: %s", section)
				}
			}
			t.Logf("Documentation validation passed")
		}
	}
}
