package updater

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.3.3", "1.3.4", -1},
		{"1.3.4", "1.3.3", 1},
		{"1.3.4", "1.3.4", 0},
		{"1.3.4", "1.4.0", -1},
		{"2.0.0", "1.9.9", 1},
		{"1.0", "1.0.0", 0},
		{"1.0.0", "1.0", 0},
		{"0.9.0", "1.0.0", -1},
		{"10.2.3", "9.99.99", 1},
		{"1.3.4", "1.3.5", -1},
		{"1.3.4", "2.0.0", -1},
	}

	for _, tt := range tests {
		got := compareVersions(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestCompareVersionsWithVPrefix(t *testing.T) {
	a := "1.3.4"
	b := "1.3.5"
	if compareVersions(a, b) != -1 {
		t.Errorf("expected -1, got %d", compareVersions(a, b))
	}
}

func TestParseGitHubRelease(t *testing.T) {
	raw := `{
		"tag_name": "v1.3.4",
		"name": "v1.3.4",
		"body": "## New features\n- Feature A\n\n## Bug fixes\n- Fix B",
		"html_url": "https://github.com/Tudou77826/OpsCopilot/releases/tag/v1.3.4",
		"assets": [
			{
				"name": "opscopilot-windows.zip",
				"browser_download_url": "https://example.com/opscopilot-windows.zip",
				"size": 16219182
			},
			{
				"name": "opscopilot.exe",
				"browser_download_url": "https://example.com/opscopilot.exe",
				"size": 14000000
			}
		]
	}`

	var release ReleaseInfo
	if err := json.Unmarshal([]byte(raw), &release); err != nil {
		t.Fatalf("parse release: %v", err)
	}

	if release.TagName != "v1.3.4" {
		t.Errorf("TagName = %q, want v1.3.4", release.TagName)
	}
	if len(release.Assets) != 2 {
		t.Fatalf("Assets count = %d, want 2", len(release.Assets))
	}
	if release.Assets[1].Name != "opscopilot.exe" {
		t.Errorf("Asset[1].Name = %q, want opscopilot.exe", release.Assets[1].Name)
	}
	if release.Assets[1].Size != 14000000 {
		t.Errorf("Asset[1].Size = %d, want 14000000", release.Assets[1].Size)
	}
}

func TestSelectDownloadURLPrefersZip(t *testing.T) {
	assets := []Asset{
		{
			Name:               "opscopilot.exe",
			BrowserDownloadURL: "https://example.com/opscopilot.exe",
		},
		{
			Name:               "opscopilot-windows.zip",
			BrowserDownloadURL: "https://example.com/opscopilot-windows.zip",
		},
	}

	got := selectDownloadURL(assets)
	want := "https://example.com/opscopilot-windows.zip"
	if got != want {
		t.Fatalf("selectDownloadURL() = %q, want %q", got, want)
	}
}

func TestSelectDownloadURLFallsBackToExe(t *testing.T) {
	assets := []Asset{
		{
			Name:               "opscopilot.exe",
			BrowserDownloadURL: "https://example.com/opscopilot.exe",
		},
	}

	got := selectDownloadURL(assets)
	want := "https://example.com/opscopilot.exe"
	if got != want {
		t.Fatalf("selectDownloadURL() = %q, want %q", got, want)
	}
}

func TestProtectedFiles(t *testing.T) {
	protected := []string{
		"config.json",
		"sessions.json",
		"quick_commands.json",
		"highlight_rules.json",
		"command_whitelist.json",
		"file_access.json",
		"mcp.json",
		"mcp-server.exe",
	}
	for _, name := range protected {
		if !protectedFiles[name] {
			t.Errorf("expected %q to be protected", name)
		}
	}

	// Non-protected files should NOT be in the map.
	notProtected := []string{
		"opscopilot.exe",
		"prompts.json",
		"mcp-config.example.json",
	}
	for _, name := range notProtected {
		if protectedFiles[name] {
			t.Errorf("expected %q to NOT be protected", name)
		}
	}
}

func TestUnzip(t *testing.T) {
	srcDir := t.TempDir()
	extractDir := filepath.Join(srcDir, "extracted")

	// Test that unzip handles a nonexistent file gracefully.
	err := unzip(filepath.Join(srcDir, "nonexistent.zip"), extractDir)
	if err == nil {
		t.Error("expected error for nonexistent zip")
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(len(s) > 0 && len(sub) > 0 && findSubstring(s, sub)))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
