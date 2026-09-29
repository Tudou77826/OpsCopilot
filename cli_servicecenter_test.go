package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	server "opscopilot/internal/servicecenter"
	"opscopilot/pkg/script"
	"opscopilot/pkg/servicecenter"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestCLIActualKnowledgeTelemetryE2E(t *testing.T) {
	const token = "isolated-cli-e2e-admin-token-2026-test"
	s, err := server.New(server.Config{DataDir: t.TempDir(), PublicURL: "https://ops.internal", AdminToken: token})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var mu sync.Mutex
	var payloads []string
	network := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/events/batch" {
			b, _ := io.ReadAll(r.Body)
			mu.Lock()
			payloads = append(payloads, string(b))
			mu.Unlock()
			r.Body = io.NopCloser(strings.NewReader(string(b)))
		}
		s.Handler().ServeHTTP(w, r)
	}))
	defer network.Close()
	roots := x509.NewCertPool()
	roots.AddCert(network.Certificate())
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
	previous := http.DefaultTransport
	http.DefaultTransport = transport
	defer func() { http.DefaultTransport = previous; transport.CloseIdleConnections() }()
	// loadCLIEnv resolves beside the test executable, which Go places in its
	// temporary build directory. Preserve any file present before this test.
	path := filepath.Join(loadCLIEnv().binDir, "service-center.json")
	before, readErr := os.ReadFile(path)
	t.Cleanup(func() {
		if readErr == nil {
			os.WriteFile(path, before, 0600)
		} else {
			os.Remove(path)
		}
	})
	t.Setenv("OPSCOPILOT_KNOWLEDGE_DIR", t.TempDir())
	configDir := t.TempDir()
	t.Setenv("OPSCOPILOT_SESSIONS_FILE", filepath.Join(configDir, "sessions.json"))
	t.Setenv("OPSCOPILOT_WHITELIST_PATH", filepath.Join(configDir, "command_whitelist.json"))
	t.Setenv("OPSCOPILOT_FILE_ACCESS_PATH", filepath.Join(configDir, "file_access.json"))
	if err := os.WriteFile(filepath.Join(configDir, "sessions.json"), []byte(`[{"id":"e2e-host","name":"192.0.2.1","type":"session","config":{"host":"192.0.2.1","port":22,"user":"e2e","protocol":"ssh"}}]`), 0600); err != nil {
		t.Fatal(err)
	}
	c := servicecenter.NewClient(path, Version)
	if _, err = c.Configure(network.URL); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Choose("standard"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		args []string
		code int
	}{
		{[]string{"list"}, 0},
		{[]string{"search", "--query", "private-e2e-query-do-not-send"}, 0},
		{[]string{"read", "--id", "private-e2e-document-do-not-send"}, 1},
	} {
		if code := cmdKnowledge(tc.args); code != tc.code {
			t.Fatalf("CLI exit %d expected %d", code, tc.code)
		}
	}
	// Exercise real business methods with isolated local data and no remote host.
	if code := cmdExec([]string{"--server", "192.0.2.1", "--command", "private-e2e-command-do-not-send"}); code != 1 {
		t.Fatalf("policy rejection should fail: %d", code)
	}
	for _, direction := range []string{"upload", "download"} {
		code := cmdFile([]string{direction, "--server", "192.0.2.1", "--local", filepath.Join(configDir, "private-e2e-file-do-not-send"), "--remote", "/private-e2e-path-do-not-send"})
		if code == 0 {
			t.Fatalf("unexpected successful transfer without host: %s", direction)
		}
	}
	c.Refresh(context.Background())
	a := &App{ctx: context.Background(), serviceCenter: c, scriptMgr: script.NewManager(nil, t.TempDir(), nil)}
	if _, err := a.CreateScript("private-e2e-script-do-not-send", "private-e2e-description-do-not-send"); err != nil {
		t.Fatal(err)
	}
	if err := a.ReplayScript("nonexistent-script", "nonexistent-session"); err == nil {
		t.Fatal("missing script unexpectedly succeeded")
	}
	a.CountServiceUsage("ctrl_k")
	a.CountServiceUsage("quick_command")
	c.Refresh(context.Background())
	req, _ := http.NewRequest("GET", network.URL+"/api/admin/stats", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var stats struct {
		Daily map[string]server.Aggregate `json:"daily"`
	}
	if err = json.NewDecoder(res.Body).Decode(&stats); err != nil {
		t.Fatal(err)
	}
	var usage map[string]int
	for _, a := range stats.Daily {
		usage = a.Usage
	}
	for key, want := range map[string]int{"cli_knowledge_list_started": 1, "cli_knowledge_list_success": 1, "cli_knowledge_search_started": 1, "cli_knowledge_search_success": 1, "cli_knowledge_read_started": 1, "cli_knowledge_read_failure": 1, "cli_exec_started": 1, "cli_exec_failure": 1, "cli_policy_command": 1, "cli_upload_started": 1, "cli_upload_failure": 1, "cli_download_started": 1, "cli_download_failure": 1, "gui_script_created": 1, "gui_script_started": 1, "gui_script_failure": 1} {
		if usage[key] != want {
			t.Fatalf("actual CLI telemetry %s=%d expected %d", key, usage[key], want)
		}
	}
	mu.Lock()
	sent := strings.Join(payloads, "\n")
	mu.Unlock()
	if strings.Contains(sent, "private-e2e-") {
		t.Fatal("CLI argument leaked into telemetry")
	}
	for _, choice := range []string{"minimal", "disabled"} {
		if _, err = c.Choose(choice); err != nil {
			t.Fatal(err)
		}
		c.Refresh(context.Background())
		mu.Lock()
		n := len(payloads)
		mu.Unlock()
		if code := cmdKnowledge([]string{"list"}); code != 0 {
			t.Fatal(code)
		}
		mu.Lock()
		after := len(payloads)
		mu.Unlock()
		if after != n {
			t.Fatalf("actual CLI sent data under %s consent", choice)
		}
	}
	t.Log("PASS: actual CLI knowledge/exec/transfer and desktop script methods report fixed outcomes via HTTPS without argument/script/path leakage; CLI respects minimal/refused consent")
}

func TestCLIUsageDoesNotCreateStateWithoutConsent(t *testing.T) {
	dir := t.TempDir()
	done, observe := beginCLIUsage(cliEnv{binDir: dir}, "cli_exec")
	observe("cli_policy_command")
	done("success")
	if _, err := os.Stat(filepath.Join(dir, "service-center.json")); !os.IsNotExist(err) {
		t.Fatal("CLI created unauthorized telemetry state")
	}
}
func TestCLIUsageRespectsRefusalAndMinimalConsent(t *testing.T) {
	for _, choice := range []string{"disabled", "minimal"} {
		t.Run(choice, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "service-center.json")
			c := servicecenter.NewClient(path, "v1.2.3")
			c.Configure("https://ops.internal")
			if _, err := c.Choose(choice); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(path)
			done, observe := beginCLIUsage(cliEnv{binDir: dir}, "cli_exec")
			observe("cli_policy_path")
			done("failure")
			after, _ := os.ReadFile(path)
			if string(before) != string(after) {
				t.Fatal("CLI changed refused/minimal state")
			}
		})
	}
}
