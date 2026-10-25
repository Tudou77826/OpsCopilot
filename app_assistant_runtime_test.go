package main

import (
	"encoding/json"
	"errors"
	"opscopilot/pkg/config"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"opscopilot/pkg/agenttransport/acp"
)

func TestAssistantAlwaysUsesICodeAndStatusOmitsSecrets(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("CHRYS_MODEL_PROFILE", "")
	directory := acp.LocalModelDirectory()
	if err := os.MkdirAll(filepath.Join(directory, "models"), 0700); err != nil {
		t.Fatal(err)
	}
	for file, data := range map[string]string{
		"settings.yaml":      "model.profile.active: native\n",
		"models/native.yaml": "id: native\nname: Local model\nprovider: anthropic\nmodel_id: native-model\napi_key: native-secret\nbase_url: https://native.invalid\n",
	} {
		if err := os.WriteFile(filepath.Join(directory, filepath.FromSlash(file)), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	mgr := config.NewManagerWithDir(t.TempDir())
	if err := mgr.Load(); err != nil {
		t.Fatal(err)
	}
	mgr.Config.LLM = config.LLMConfig{APIKey: "app-secret", BaseURL: "https://app.invalid", ComplexModel: "app-model"}
	a := &App{configMgr: mgr}
	native, err := a.assistantModelConfig("")
	if err != nil || native.LocalModel.Name != "Local model" || native.APIKey != "" || native.BaseURL != "" {
		t.Fatal("default model source did not stay native", err)
	}
	app, err := a.assistantModelConfig("opscopilot")
	if err != nil || app.LocalModel != native.LocalModel || app.APIKey != "" || app.BaseURL != "" {
		t.Fatal("legacy source changed the iCode-only model", err)
	}
	status, _ := json.Marshal(a.InspectAssistantModel("icode"))
	if strings.Contains(string(status), "secret") || strings.Contains(string(status), "native.invalid") || !strings.Contains(string(status), "Local model") {
		t.Fatal("model status exposed configuration or omitted model name")
	}
	if current, err := a.assistantModelConfig("unknown"); err != nil || current.LocalModel != native.LocalModel {
		t.Fatal("saved source must never select another model")
	}
}

func TestAssistantExecutableDiscovery(t *testing.T) {
	root := t.TempDir()
	application := filepath.Join(root, "opscopilot.exe")
	installed := filepath.Join(root, "installed", "icode.exe")
	var looked []string
	lookup := func(name string) (string, error) {
		looked = append(looked, name)
		if name == "icode" {
			return installed, nil
		}
		return "", errors.New("missing")
	}
	if got, err := resolveAssistantExecutable("", application, lookup); err != nil || got != installed || len(looked) != 1 || looked[0] != "icode" {
		t.Fatal(got, err, looked)
	}
	write := func(relative string) string {
		t.Helper()
		file := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, nil, 0600); err != nil {
			t.Fatal(err)
		}
		return file
	}
	legacy := write("chrys/chrys.exe")
	if got, err := resolveAssistantExecutable("", application, lookup); err != nil || got != legacy {
		t.Fatal(got, err)
	}
	bundled := write("icode/icode.exe")
	if got, err := resolveAssistantExecutable("", application, lookup); err != nil || got != bundled {
		t.Fatal(got, err)
	}
	t.Setenv("OPSCOPILOT_CHRYS_EXECUTABLE", legacy)
	t.Setenv("OPSCOPILOT_ICODE_EXECUTABLE", bundled)
	if got, err := findChrysExecutable(); err != nil || got != bundled {
		t.Fatal("new override must take precedence", got, err)
	}
	t.Setenv("OPSCOPILOT_ICODE_EXECUTABLE", "")
	if got, err := findChrysExecutable(); err != nil || got != legacy {
		t.Fatal("legacy override lost", got, err)
	}
	for _, configured := range []string{"relative.exe", root, filepath.Join(root, "missing.exe")} {
		if got, err := resolveAssistantExecutable(configured, application, lookup); err == nil || got != "" {
			t.Fatal("invalid override silently fell back", configured, got, err)
		}
	}
}

func TestSavedAssistantRuntimeOverridesEnvironmentAndPersists(t *testing.T) {
	file := filepath.Join(t.TempDir(), "icode.exe")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	mgr := config.NewManagerWithDir(t.TempDir())
	if err := mgr.Load(); err != nil {
		t.Fatal(err)
	}
	mgr.Config.AssistantRuntime.Executable = file
	mgr.Config.AssistantRuntime.ModelSource = "opscopilot"
	if err := mgr.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded := config.NewManagerWithDir(mgr.Directory())
	if err := reloaded.Load(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPSCOPILOT_ICODE_EXECUTABLE", filepath.Join(t.TempDir(), "not-installed.exe"))
	a := &App{configMgr: reloaded}
	if reloaded.Config.AssistantRuntime.ModelSource != "opscopilot" {
		t.Fatal("model source was not preserved")
	}
	if got, err := a.assistantExecutable(); err != nil || got != file {
		t.Fatal(got, err)
	}
	bad := a.CheckAssistantRuntime("relative.exe")
	if bad.Ready || len(bad.Checks) != 1 || bad.Checks[0].Status != "failed" {
		t.Fatal(bad)
	}
}

func TestAssistantExecutableLegacyPathFallback(t *testing.T) {
	legacy := filepath.Join(t.TempDir(), "chrys.exe")
	got, err := resolveAssistantExecutable("", "", func(name string) (string, error) {
		if name == "chrys" {
			return legacy, nil
		}
		return "", errors.New("not installed")
	})
	if err != nil || got != legacy {
		t.Fatal(got, err)
	}
}
