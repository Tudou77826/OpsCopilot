package servicecenter

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	protocol "opscopilot/pkg/servicecenter"
)

// Exercises the production client and server over real HTTPS, with isolated disk
// state. OPS_E2E_PREVIEW=1 keeps the populated real admin available for UI review.
func TestTelemetryClientServerE2E(t *testing.T) {
	const token = "e2e-disposable-local-review-token-2026"
	dir := t.TempDir()
	cfg := Config{DataDir: filepath.Join(dir, "server"), PublicURL: "http://127.0.0.1:19081", LocalHTTP: true, AdminToken: token}
	s, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	var posts atomic.Int32
	var fail atomic.Bool
	var futurePolicy atomic.Bool
	var capturedMu sync.Mutex
	var captured [][]protocol.Record
	network := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/telemetry-policy" && futurePolicy.Load() {
			respond(w, 200, protocol.Policy{Version: "future-policy"})
			return
		}
		if r.URL.Path == "/api/v1/events/batch" {
			posts.Add(1)
			body, _ := io.ReadAll(r.Body)
			var batch struct {
				Records []protocol.Record `json:"records"`
			}
			if err := json.Unmarshal(body, &batch); err != nil {
				t.Error(err)
			}
			capturedMu.Lock()
			captured = append(captured, batch.Records)
			capturedMu.Unlock()
			r.Body = io.NopCloser(bytes.NewReader(body))
			if fail.Load() {
				w.WriteHeader(503)
				return
			}
		}
		s.Handler().ServeHTTP(w, r)
	}))
	defer network.Close()
	// Trust only this test certificate; retain HTTPS verification and restore the
	// process default afterward. This test must not run in parallel.
	roots := x509.NewCertPool()
	roots.AddCert(network.Certificate())
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
	previous := http.DefaultTransport
	http.DefaultTransport = transport
	defer func() { http.DefaultTransport = previous; transport.CloseIdleConnections() }()
	newClient := func(name, choice, version string) *protocol.Client {
		c := protocol.NewClient(filepath.Join(dir, name, "service-center.json"), version)
		if _, e := c.Configure(network.URL); e != nil {
			t.Fatal(e)
		}
		if choice != "" {
			if _, e := c.Choose(choice); e != nil {
				t.Fatal(e)
			}
		}
		return c
	}
	refresh := func(c *protocol.Client) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		c.Refresh(ctx)
	}
	state := func(name string) protocol.State {
		b, e := os.ReadFile(filepath.Join(dir, name, "service-center.json"))
		if e != nil {
			t.Fatal(e)
		}
		var v protocol.State
		if e = json.Unmarshal(b, &v); e != nil {
			t.Fatal(e)
		}
		return v
	}
	stats := func() map[string]Aggregate {
		req, _ := http.NewRequest("GET", network.URL+"/api/admin/stats", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			t.Fatal(res.StatusCode)
		}
		var v struct {
			Daily map[string]Aggregate `json:"daily"`
		}
		if e = json.NewDecoder(res.Body).Decode(&v); e != nil {
			t.Fatal(e)
		}
		return v.Daily
	}
	day := time.Now().Format("2006-01-02")
	for _, choice := range []string{"", "disabled"} {
		c := newClient("refused", choice, "v1.2.3")
		c.Count("ctrl_k")
		c.BeginUsage("gui_script")("success")
		refresh(c)
		if posts.Load() != 0 || len(state("refused").Records) != 0 || state("refused").InstallationID != "" {
			t.Fatal("unconsented collection")
		}
	}
	t.Log("PASS: pending/refused consent produces no telemetry or installation identity")
	c := newClient("standard", "standard", "v1.2.3")
	refresh(c)
	if !c.Settings().Ready {
		t.Fatal("real policy did not authorize client")
	}
	c.BeginConnection()(true)
	c.BeginConnection()(false)
	for i := 0; i < 3; i++ {
		c.Count("ctrl_k")
	}
	for i := 0; i < 4; i++ {
		c.Count("quick_command")
	}
	want := map[string]int{}
	for _, prefix := range []string{"gui_script", "gui_diagnose", "gui_archive", "gui_upload", "gui_download", "gui_generate", "cli_exec", "cli_upload", "cli_download", "cli_diagnose", "cli_knowledge_list", "cli_knowledge_search", "cli_knowledge_read"} {
		outcomes := []string{"success", "failure"}
		if strings.HasPrefix(prefix, "gui_") {
			outcomes = append(outcomes, "cancelled")
		}
		for _, outcome := range outcomes {
			done := c.BeginUsage(prefix)
			done(outcome)
			done(outcome)
			want[prefix+"_started"]++
			want[prefix+"_"+outcome]++
		}
	}
	for _, key := range []string{"gui_script_created", "gui_knowledge_opened", "gui_knowledge_search", "gui_knowledge_found", "gui_knowledge_empty", "gui_command_typed", "cli_policy_command", "cli_policy_target", "cli_policy_path", "cli_policy_size", "cli_recovery_started", "cli_recovery_success", "cli_recovery_failure"} {
		c.Count(key)
		want[key]++
	}
	for _, key := range []string{"gui_knowledge_search", "cli_recovery_started"} {
		c.Count(key)
		want[key]++
	}
	c.Upgrade("e2e-upgrade", "v1.2.4", "download", "success", "none")
	c.Upgrade("e2e-upgrade", "v1.2.4", "install", "failure", "install")
	c.Upgrade("e2e-check", "v1.2.4", "check", "no_update", "none")
	c.Upgrade("e2e-startup", "v1.2.4", "startup", "success", "none")
	refresh(c)
	a := stats()[day]
	if a.Active != 1 || a.Connections != 2 || a.Connected != 1 || a.CtrlK != 3 || a.QuickCommands != 4 || !reflect.DeepEqual(a.Usage, want) || a.Upgrades["download:success"] != 1 || a.Upgrades["install:failure"] != 1 || a.Upgrades["check:no_update"] != 1 || a.Upgrades["startup:success"] != 1 {
		t.Fatalf("incorrect initial aggregate: %+v", a)
	}
	refresh(c)
	refresh(c)
	if !reflect.DeepEqual(a, stats()[day]) {
		t.Fatal("repeated cumulative snapshots double counted")
	}
	t.Log("PASS: all GUI/CLI fixed metrics, outcomes and upgrades persist; duplicate completions and uploads are idempotent")
	second := protocol.NewClient(filepath.Join(dir, "standard", "service-center.json"), "v1.2.3")
	refresh(second)
	var wg sync.WaitGroup
	for _, client := range []*protocol.Client{c, second} {
		wg.Add(1)
		go func(client *protocol.Client) {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				client.Count("ctrl_k")
			}
		}(client)
	}
	wg.Wait()
	refresh(c)
	if stats()[day].CtrlK != 23 {
		t.Fatalf("lost shared-file increments: %+v", stats()[day])
	}
	fail.Store(true)
	c.Count("quick_command")
	refresh(c)
	if stats()[day].QuickCommands != 4 || len(state("standard").Records) == 0 {
		t.Fatal("failure did not retain pending snapshot")
	}
	fail.Store(false)
	refresh(c)
	refresh(c)
	if stats()[day].QuickCommands != 5 {
		t.Fatal("retry lost or duplicated data")
	}
	t.Log("PASS: concurrent desktop/CLI-style clients lose no increments; HTTP 503 recovery retries exactly once")
	minimal := newClient("minimal", "minimal", "v1.2.3")
	refresh(minimal)
	minimal.Count("connection")
	minimal.Count("ctrl_k")
	minimal.BeginUsage("cli_exec")("success")
	minimal.Upgrade("minimal-upgrade", "v1.2.4", "download", "success", "none")
	refresh(minimal)
	m := state("minimal")
	if len(m.Records) != 1 || m.Records[0].OS != "" || m.Records[0].Arch != "" || len(m.Records[0].Usage) != 0 || m.Records[0].Connections != 0 || m.Records[0].CtrlK != 0 {
		t.Fatalf("minimal exceeded scope: %+v", m)
	}
	if stats()[day].Active != 2 || stats()[day].CtrlK != 23 || !reflect.DeepEqual(stats()[day].Usage, want) {
		t.Fatal("minimal contaminated feature totals")
	}
	// Reject out-of-scope data through the actual HTTP endpoint.
	bad := m.Records[0]
	bad.Usage = map[string]int{"cli_exec_started": 1}
	b, _ := json.Marshal(map[string]any{"records": []protocol.Record{bad}})
	res, e := http.Post(network.URL+"/api/v1/events/batch", "application/json", bytes.NewReader(b))
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatal("server accepted minimal extra fields")
	}
	t.Log("PASS: minimal sends only activity/version/consent metadata; server rejects extra feature data")
	pending := c.BeginUsage("gui_script")
	if _, e := second.Choose("disabled"); e != nil {
		t.Fatal(e)
	}
	before := posts.Load()
	pending("success")
	c.Count("ctrl_k")
	refresh(c)
	if posts.Load() != before || state("standard").InstallationID != "" || len(state("standard").Records) != 0 {
		t.Fatal("cross-process revocation failed")
	}
	t.Log("PASS: revoke clears queue/identity and prevents outstanding completions and subsequent uploads")
	if _, e := c.Choose("standard"); e != nil {
		t.Fatal(e)
	}
	futurePolicy.Store(true)
	before = posts.Load()
	refresh(c)
	if !c.Settings().NeedsConsent || posts.Load() != before || len(state("standard").Records) != 0 || state("standard").InstallationID != "" {
		t.Fatalf("expanded policy did not require reconsent before upload: settings=%+v posts=%d/%d state=%+v",c.Settings(),posts.Load(),before,state("standard"))
	}
	futurePolicy.Store(false)
	t.Log("PASS: changed collection policy requires renewed consent without uploading under old consent")
	res, e = http.Get(network.URL + "/api/admin/stats")
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatal("statistics exposed without auth")
	}
	baseline := stats()
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	s, e = New(cfg)
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(baseline, stats()) {
		t.Fatal("server restart lost aggregate")
	}
	t.Log("PASS: admin authentication and database restart persistence")
	if os.Getenv("OPS_E2E_PREVIEW") == "1" {
		listener, e := net.Listen("tcp", "127.0.0.1:19081")
		if e != nil {
			t.Fatal(e)
		}
		ui := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second}
		go ui.Serve(listener)
		defer ui.Close()
		t.Log("Real E2E admin review: http://127.0.0.1:19081/admin (disposable test token in source). Create build/telemetry-e2e-stop to finish.")
		deadline := time.Now().Add(10 * time.Minute)
		for time.Now().Before(deadline) {
			if _, e := os.Stat(filepath.Join("..", "..", "build", "telemetry-e2e-stop")); e == nil {
				break
			}
			time.Sleep(time.Second)
		}
	}
}
