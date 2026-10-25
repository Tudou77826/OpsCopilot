package acp

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedEnvironmentPreservesWindowsLauncherProfileButIsolatesConfig(t *testing.T) {
	t.Setenv("USERPROFILE", "C:\\Users\\runtime-owner")
	t.Setenv("APPDATA", "C:\\personal-config")
	t.Setenv("CHRYS_CONFIG", "personal-override")
	t.Setenv("OPENAI_API_KEY", "parent-secret")
	directory := t.TempDir()
	home := filepath.Join(directory, "home")
	for _, platform := range []string{"windows", "linux"} {
		values := map[string]string{}
		for _, entry := range managedEnvironmentForOS(directory, home, "managed-key", platform) {
			name, value, _ := strings.Cut(entry, "=")
			name = strings.ToUpper(name)
			if _, exists := values[name]; exists {
				t.Fatalf("duplicate environment entry: %s", name)
			}
			values[name] = value
		}
		profile := home
		if platform == "windows" {
			profile = "C:\\Users\\runtime-owner"
		}
		if values["USERPROFILE"] != profile || values["HOME"] != home || values["APPDATA"] != filepath.Join(directory, "appdata") || values["LOCALAPPDATA"] != filepath.Join(directory, "localappdata") {
			t.Fatal("launcher profile or config isolation changed", platform)
		}
		if values["OPENAI_API_KEY"] != "" || values["CHRYS_CONFIG"] != "" || values["OPSCOPILOT_MODEL_API_KEY"] != "managed-key" {
			t.Fatal("parent configuration leaked", platform)
		}
	}
}
