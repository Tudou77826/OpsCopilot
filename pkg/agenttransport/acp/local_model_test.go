package acp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"opscopilot/pkg/assistantcontrol"
)

func localModelFixture(t *testing.T, profile, settings, env string) string {
	t.Helper()
	directory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(directory, "models"), 0700); err != nil {
		t.Fatal(err)
	}
	for path, text := range map[string]string{"models/selected.yaml": profile, "settings.yaml": settings, ".env": env} {
		if err := os.WriteFile(filepath.Join(directory, filepath.FromSlash(path)), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return directory
}

func TestLocalModelReadsOnlySelectedModelAndReferencedCredentials(t *testing.T) {
	t.Setenv("CHRYS_MODEL_PROFILE", "")
	t.Setenv("TEST_MODEL_KEY", "process-key")
	directory := localModelFixture(t, `id: chosen
name: Intranet
model_id: business-model
provider: anthropic
api_key: '{{TEST_MODEL_KEY}}'
http_headers: '{"X-Token":"{{NATIVE_ONLY_KEY}}"}'
api_style: responses
verify_ssl: false
bypass_proxy: true
`, "model:\n  profile:\n    active: chosen\napproval:\n  mode: bypass\n", "TEST_MODEL_KEY=file-key\nNATIVE_ONLY_KEY='native-secret'\nCHRYS_APPROVAL=bypass\nHOME=wrong-home\n")
	before, _ := os.ReadFile(filepath.Join(directory, "models/selected.yaml"))
	model, err := LoadLocalModel(directory)
	if err != nil {
		t.Fatal(err)
	}
	if model.Name != "Intranet" || strings.Contains(model.Profile, "process-key") || strings.Contains(model.Profile, "native-secret") || strings.Contains(model.Profile, "approval") {
		t.Fatal("model metadata or credential isolation changed")
	}
	var profile map[string]any
	var env map[string]string
	_ = json.Unmarshal([]byte(model.Profile), &profile)
	_ = json.Unmarshal([]byte(model.Environment), &env)
	if profile["provider"] != "anthropic" || profile["api_style"] != "responses" || profile["verify_ssl"] != false || profile["bypass_proxy"] != true {
		t.Fatal("native transport configuration lost")
	}
	if env["OPSCOPILOT_IMPORTED_MODEL_0"] != "process-key" || env["OPSCOPILOT_IMPORTED_MODEL_1"] != "native-secret" || len(env) != 2 {
		t.Fatal("wrong credential source or unrelated environment imported")
	}
	after, _ := os.ReadFile(filepath.Join(directory, "models/selected.yaml"))
	if string(before) != string(after) {
		t.Fatal("native model was changed")
	}
}

func TestLocalModelRequiresExplicitUnambiguousActiveSelection(t *testing.T) {
	t.Setenv("CHRYS_MODEL_PROFILE", "")
	for _, selector := range []string{"", "missing", "duplicate"} {
		directory := localModelFixture(t, "id: chosen\nname: duplicate\nmodel_id: model\n", "model.profile.active: '"+selector+"'\n", "")
		_ = os.WriteFile(filepath.Join(directory, "models/other.yaml"), []byte("id: other\nname: duplicate\nmodel_id: other\n"), 0600)
		if _, err := LoadLocalModel(directory); err == nil {
			t.Fatalf("silently chose endpoint for selector %q", selector)
		}
	}
}

func TestLocalModelEnvFallbackAndSanitizedFailures(t *testing.T) {
	t.Setenv("CHRYS_MODEL_PROFILE", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	if err := os.Unsetenv("ANTHROPIC_API_KEY"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANTHROPIC_BASE_URL", "")
	if err := os.Unsetenv("ANTHROPIC_BASE_URL"); err != nil {
		t.Fatal(err)
	}
	directory := localModelFixture(t, "id: selected\nname: Native\nprovider: anthropic\nmodel_id: model\n", "model.profile.active: selected\n", "ANTHROPIC_API_KEY=fallback-secret\nANTHROPIC_BASE_URL=https://intranet.invalid\n")
	model, err := LoadLocalModel(directory)
	if err != nil || !strings.Contains(model.Profile, "https://intranet.invalid") {
		t.Fatal("native endpoint fallback lost", err)
	}
	var env map[string]string
	_ = json.Unmarshal([]byte(model.Environment), &env)
	if env["OPSCOPILOT_IMPORTED_MODEL_KEY"] != "fallback-secret" || strings.Contains(model.Profile, "fallback-secret") {
		t.Fatal("native provider credential fallback lost")
	}
	_ = os.WriteFile(filepath.Join(directory, "models/selected.yaml"), []byte("api_key: secret-do-not-leak\ninvalid: ["), 0600)
	if _, err = LoadLocalModel(directory); err == nil || strings.Contains(err.Error(), "secret-do-not-leak") {
		t.Fatal("malformed config was accepted or secret leaked")
	}
}

func TestLiveICodeInheritedModelAuthenticationAndIsolation(t *testing.T) {
	executable := os.Getenv("OPSCOPILOT_CHRYS_TEST_EXE")
	if executable == "" {
		t.Skip("opt-in official iCode")
	}
	t.Setenv("CHRYS_MODEL_PROFILE", "")
	t.Setenv("NATIVE_MODEL_TEST_KEY", "")
	if err := os.Unsetenv("NATIVE_MODEL_TEST_KEY"); err != nil {
		t.Fatal(err)
	}
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Authorization") != "Bearer inherited-fixture-key" || r.Header.Get("X-Native-Auth") != "inherited-header" {
			t.Error("inherited model authentication was not preserved")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "fixture", "object": "chat.completion", "model": "fixture", "choices": []any{map[string]any{"index": 0, "message": map[string]string{"role": "assistant", "content": "INHERITED_OK"}, "finish_reason": "stop"}}, "usage": map[string]int{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}})
	}))
	defer server.Close()
	profile := "id: chosen\nname: Native model\nprovider: openai\nmodel_id: fixture\nstream: false\nbase_url: " + server.URL + "/v1\napi_key: '{{NATIVE_MODEL_TEST_KEY}}'\nhttp_headers: '{\"X-Native-Auth\":\"{{NATIVE_HEADER}}\"}'\n"
	directory := localModelFixture(t, profile, "model.profile.active: chosen\napproval.mode: bypass\n", "NATIVE_MODEL_TEST_KEY=inherited-fixture-key\nNATIVE_HEADER=inherited-header\nCHRYS_APPROVAL=bypass\n")
	model, err := LoadLocalModel(directory)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()
	c := &Conversation{}
	defer c.Close()
	turn, _ := (&assistantcontrol.Coordinator{}).Begin(ctx, assistantcontrol.Target{}, nil)
	defer turn.Finish("completed", "")
	if err = c.Run(ctx, ManagedConfig{Executable: executable, LocalModel: model}, turn, &managedFixtureTools{}, "reply"); err != nil {
		t.Fatal(err)
	}
	if requests != 1 || turn.Snapshot().Output != "INHERITED_OK" {
		t.Fatal("inherited model did not complete real ACP prompt")
	}
	var agent struct {
		Profile map[string]any `json:"profile"`
	}
	if err = c.client.callExtension(ctx, "profiles/agents/read", map[string]string{"name": "OpsCopilot"}, &agent); err != nil {
		t.Fatal(err)
	}
	approval, _ := agent.Profile["approval"].(map[string]any)
	if approval["default"] != "require" || approval["user_can_override"] != false {
		t.Fatal("native user authorization leaked into managed agent")
	}
	before, _ := os.ReadFile(filepath.Join(directory, "models/selected.yaml"))
	if string(before) != profile {
		t.Fatal("native model changed during isolated conversation")
	}
}
