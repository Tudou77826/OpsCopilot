package shellsidecar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSettingsServiceLoadsLegacyTopLevelFieldsAndKeepsZeroDelay(t *testing.T) {
	dataDir := t.TempDir()
	path := filepath.Join(dataDir, "shell-settings.json")
	legacy := `{
  "theme": "light",
  "terminal": {
    "scrollback": 5000,
    "search_enabled": true,
    "highlight_enabled": true,
    "font_family": "JetBrains Mono",
    "font_size": 16
  },
  "completion_delay": 0,
  "highlight_rules": [{
    "id": "errors",
    "name": "Errors",
    "pattern": "ERROR",
    "is_enabled": true,
    "priority": 1,
    "style": {"color": "#ff0000"}
  }]
}`
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	service, err := NewSettingsService(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := service.Get()
	if err != nil {
		t.Fatal(err)
	}
	if settings.CompletionDelay != 0 {
		t.Fatalf("completion delay = %d, want 0", settings.CompletionDelay)
	}
	if len(settings.HighlightRules) != 1 || settings.HighlightRules[0].ID != "errors" {
		t.Fatalf("highlight rules = %#v", settings.HighlightRules)
	}

	if err := service.Save(settings); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(saved)
	if !strings.Contains(text, `"completionDelay": 0`) || !strings.Contains(text, `"highlightRules"`) {
		t.Fatalf("canonical fields missing from saved settings:\n%s", text)
	}
	if strings.Contains(text, `"completion_delay"`) || strings.Contains(text, `"highlight_rules"`) {
		t.Fatalf("legacy fields remained after save:\n%s", text)
	}
}
