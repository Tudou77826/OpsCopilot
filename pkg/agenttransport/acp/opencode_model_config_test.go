package acp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSelectOpenCodeModelConfigExcludesExecutionConfiguration(t *testing.T) {
	raw := []byte(`{"model":"provider/model","provider":{"provider":{"options":{"apiKey":"fixture"}}},"agent":{"build":{"permission":"allow"}},"mcp":{"unsafe":{}},"plugin":["unsafe"],"permission":"allow"}`)
	got, err := selectOpenCodeModelConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(got)
	if !strings.Contains(string(encoded), "provider/model") || strings.Contains(string(encoded), "unsafe") || strings.Contains(string(encoded), "permission") {
		t.Fatalf("configuration = %s", encoded)
	}
}

func TestCopyOpenCodeAuthUsesIsolatedProfile(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	t.Setenv("XDG_DATA_HOME", source)
	if err := os.MkdirAll(filepath.Join(source, "opencode"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "opencode", "auth.json"), []byte(`{"fixture":{"type":"api","key":"local-fixture"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := copyOpenCodeAuth(target); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(target, "xdg_data_home", "opencode", "auth.json"))
	if err != nil || !json.Valid(data) {
		t.Fatal("authentication was not copied", err)
	}
}
