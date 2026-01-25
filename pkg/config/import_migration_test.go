package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestManager(t *testing.T) (*Manager, string) {
	t.Helper()

	dir := t.TempDir()

	mgr := NewManager()
	mgr.configPath = filepath.Join(dir, "config.json")
	mgr.promptsPath = filepath.Join(dir, "prompts.json")
	mgr.quickCommandsPath = filepath.Join(dir, "quick_commands.json")
	mgr.highlightRulesPath = filepath.Join(dir, "highlight_rules.json")

	mgr.Config.LLM = LLMConfig{
		APIKey:       "sk-new",
		BaseURL:      "https://new.example/v1",
		FastModel:    "new-fast",
		ComplexModel: "new-complex",
	}
	mgr.Config.CompletionDelay = 150
	mgr.Config.Terminal = TerminalConfig{Scrollback: 5000, SearchEnabled: true, HighlightEnabled: true}
	mgr.Config.HighlightRules = []HighlightRule{}
	mgr.Config.QuickCommands = []QuickCommand{}

	if err := mgr.Save(); err != nil {
		t.Fatalf("Save baseline config: %v", err)
	}

	return mgr, dir
}

func TestImportFromDirectory_BrokenMainConfig_SkipsAndImportsOthers(t *testing.T) {
	mgr, baseDir := newTestManager(t)

	oldDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(oldDir, "config.json"), []byte("{broken"), 0644); err != nil {
		t.Fatalf("write broken config.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "prompts.json"), []byte(`{"p":"v"}`), 0644); err != nil {
		t.Fatalf("write prompts.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "quick_commands.json"), []byte(`[{"id":"1","name":"n","content":"c"}]`), 0644); err != nil {
		t.Fatalf("write quick_commands.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "highlight_rules.json"), []byte(`[{"id":"r1","name":"r","pattern":"x","is_enabled":true,"priority":1,"style":{"color":"#fff"}}]`), 0644); err != nil {
		t.Fatalf("write highlight_rules.json: %v", err)
	}

	beforeLLM := mgr.Config.LLM

	if err := mgr.ImportFromDirectory(oldDir); err != nil {
		t.Fatalf("ImportFromDirectory error: %v", err)
	}

	if mgr.Config.LLM != beforeLLM {
		t.Fatalf("LLM should not change when config.json is broken")
	}
	if mgr.Config.Prompts["p"] != "v" {
		t.Fatalf("prompts not imported")
	}
	if len(mgr.Config.QuickCommands) != 1 || mgr.Config.QuickCommands[0].ID != "1" {
		t.Fatalf("quick commands not imported")
	}
	if len(mgr.Config.HighlightRules) != 1 || mgr.Config.HighlightRules[0].ID != "r1" {
		t.Fatalf("highlight rules not imported")
	}
	if msg := mgr.LastImportMessage(); msg == "" || !strings.Contains(msg, "config.json 格式错误") {
		t.Fatalf("LastImportMessage = %q, want it to mention broken config.json", msg)
	}

	for _, p := range []string{"config.json.bak", "prompts.json.bak", "quick_commands.json.bak", "highlight_rules.json.bak"} {
		if _, err := os.Stat(filepath.Join(baseDir, p)); err != nil {
			t.Fatalf("expected backup file %s: %v", p, err)
		}
	}
}

func TestImportFromDirectory_Idempotent(t *testing.T) {
	mgr, baseDir := newTestManager(t)

	oldDir := t.TempDir()
	oldCfg := map[string]any{
		"llm": map[string]any{
			"APIKey":       "sk-old",
			"BaseURL":      "https://old.example/v1",
			"FastModel":    "old-fast",
			"ComplexModel": "old-complex",
		},
	}
	b, err := json.MarshalIndent(oldCfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal old config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "config.json"), b, 0644); err != nil {
		t.Fatalf("write old config.json: %v", err)
	}

	if err := mgr.ImportFromDirectory(oldDir); err != nil {
		t.Fatalf("first import error: %v", err)
	}
	firstConfig, err := os.ReadFile(filepath.Join(baseDir, "config.json"))
	if err != nil {
		t.Fatalf("read config.json after first import: %v", err)
	}

	if err := mgr.ImportFromDirectory(oldDir); err != nil {
		t.Fatalf("second import error: %v", err)
	}
	secondConfig, err := os.ReadFile(filepath.Join(baseDir, "config.json"))
	if err != nil {
		t.Fatalf("read config.json after second import: %v", err)
	}

	if string(firstConfig) != string(secondConfig) {
		t.Fatalf("import is not idempotent: config.json differs after second import")
	}
	if mgr.Config.Terminal.SearchEnabled != true {
		t.Fatalf("new field Terminal.SearchEnabled should remain true")
	}
}

func TestImportFromDirectory_DirectoryNotExists(t *testing.T) {
	mgr, _ := newTestManager(t)

	oldLLM := mgr.Config.LLM
	err := mgr.ImportFromDirectory(filepath.Join(t.TempDir(), "nonexistent"))
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if mgr.Config.LLM != oldLLM {
		t.Fatalf("LLM config changed on error")
	}
}

func TestImportFromDirectory_EmptyDirectory_NoChanges(t *testing.T) {
	mgr, baseDir := newTestManager(t)

	beforeConfig, err := os.ReadFile(filepath.Join(baseDir, "config.json"))
	if err != nil {
		t.Fatalf("read before config.json: %v", err)
	}
	beforePrompts, err := os.ReadFile(filepath.Join(baseDir, "prompts.json"))
	if err != nil {
		t.Fatalf("read before prompts.json: %v", err)
	}

	emptyOldDir := t.TempDir()
	if err := mgr.ImportFromDirectory(emptyOldDir); err != nil {
		t.Fatalf("ImportFromDirectory empty dir error: %v", err)
	}

	afterConfig, err := os.ReadFile(filepath.Join(baseDir, "config.json"))
	if err != nil {
		t.Fatalf("read after config.json: %v", err)
	}
	afterPrompts, err := os.ReadFile(filepath.Join(baseDir, "prompts.json"))
	if err != nil {
		t.Fatalf("read after prompts.json: %v", err)
	}

	if string(beforeConfig) != string(afterConfig) {
		t.Fatalf("config.json changed after importing from empty directory")
	}
	if string(beforePrompts) != string(afterPrompts) {
		t.Fatalf("prompts.json changed after importing from empty directory")
	}
}

func TestImportFromDirectory_PartialFiles_ImportsLLMAndPromptsOnly(t *testing.T) {
	mgr, _ := newTestManager(t)

	oldDir := t.TempDir()

	oldCfg := map[string]any{
		"llm": map[string]any{
			"APIKey":  "sk-old",
			"BaseURL": "https://old.example/v1",
			"Model":   "old-model",
		},
		"completion_delay": 999,
	}
	b, err := json.MarshalIndent(oldCfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal old config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "config.json"), b, 0644); err != nil {
		t.Fatalf("write old config.json: %v", err)
	}

	oldPrompts := map[string]string{
		"custom_prompt": "hello",
	}
	pb, err := json.MarshalIndent(oldPrompts, "", "  ")
	if err != nil {
		t.Fatalf("marshal old prompts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "prompts.json"), pb, 0644); err != nil {
		t.Fatalf("write old prompts.json: %v", err)
	}

	if err := mgr.ImportFromDirectory(oldDir); err != nil {
		t.Fatalf("ImportFromDirectory error: %v", err)
	}

	if mgr.Config.LLM.APIKey != "sk-old" {
		t.Fatalf("APIKey = %q, want %q", mgr.Config.LLM.APIKey, "sk-old")
	}
	if mgr.Config.LLM.BaseURL != "https://old.example/v1" {
		t.Fatalf("BaseURL = %q, want %q", mgr.Config.LLM.BaseURL, "https://old.example/v1")
	}
	if mgr.Config.LLM.FastModel != "old-model" {
		t.Fatalf("FastModel = %q, want %q", mgr.Config.LLM.FastModel, "old-model")
	}
	if mgr.Config.LLM.ComplexModel != "new-complex" {
		t.Fatalf("ComplexModel = %q, want %q (keep new default)", mgr.Config.LLM.ComplexModel, "new-complex")
	}
	if mgr.Config.CompletionDelay != 150 {
		t.Fatalf("CompletionDelay = %d, want %d (keep new default)", mgr.Config.CompletionDelay, 150)
	}
	if mgr.Config.Prompts["custom_prompt"] != "hello" {
		t.Fatalf("custom prompt not imported")
	}
	if len(mgr.Config.QuickCommands) != 0 {
		t.Fatalf("QuickCommands should remain default")
	}
	if len(mgr.Config.HighlightRules) != 0 {
		t.Fatalf("HighlightRules should remain default")
	}
}
