package servicecenter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConnectionIsReadOnly(t *testing.T) {
	var requests atomic.Int32
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != "GET" || r.URL.Path != "/api/v1/telemetry-policy" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(Policy{Version: PolicyVersion})
	}))
	defer s.Close()
	path := filepath.Join(t.TempDir(), "service.json")
	c := NewClient(path, "v1.2.3")
	c.http.Transport = s.Client().Transport
	before := c.Settings()
	if err := c.TestConnection(context.Background(), s.URL); err != nil {
		t.Fatal(err)
	}
	after := c.Settings()
	if before.BaseURL != after.BaseURL || before.Choice != after.Choice || after.Ready || requests.Load() != 1 {
		t.Fatal("test changed settings or collection state")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("test persisted configuration")
	}
}

func TestRefusalMakesNoTelemetryRequestsOrRecords(t *testing.T) {
	var requests atomic.Int32
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		json.NewEncoder(w).Encode(Policy{Version: PolicyVersion, Notice: Notice})
	}))
	defer s.Close()
	path := filepath.Join(t.TempDir(), "service.json")
	c := NewClient(path, "v1.2.3")
	c.http.Transport = s.Client().Transport
	if _, e := c.Configure(s.URL); e != nil {
		t.Fatal(e)
	}
	for _, choice := range []string{"", "disabled"} {
		if choice != "" {
			if _, e := c.Choose(choice); e != nil {
				t.Fatal(e)
			}
		}
		c.Count("connection")
		c.Count("ctrl_k")
		c.Count("quick_command")
		c.Upgrade("id", "v1.3.0", "check", "success", "none")
		c.Refresh(context.Background())
		if c.state.InstallationID != "" || len(c.state.Records) > 0 {
			t.Fatalf("collected before consent: %+v", c.state)
		}
		if requests.Load() != 0 {
			t.Fatal("telemetry request without consent")
		}
	}
	reloaded := NewClient(path, "v1.2.3")
	if reloaded.Settings().Choice != "disabled" {
		t.Fatal("refusal not persisted")
	}
	data, _ := os.ReadFile(path)
	var stored State
	json.Unmarshal(data, &stored)
	if stored.InstallationID != "" || len(stored.Records) > 0 {
		t.Fatal("persisted unauthorized data")
	}
}
func TestConsentDailyCountsRevokeAndPolicyChange(t *testing.T) {
	var mu sync.Mutex
	var batches [][]Record
	policy := PolicyVersion
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/telemetry-policy" {
			mu.Lock()
			v := policy
			mu.Unlock()
			json.NewEncoder(w).Encode(Policy{Version: v})
			return
		}
		var b struct {
			Records []Record `json:"records"`
		}
		json.NewDecoder(r.Body).Decode(&b)
		mu.Lock()
		batches = append(batches, b.Records)
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer s.Close()
	path := filepath.Join(t.TempDir(), "service.json")
	c := NewClient(path, "v1.2.3")
	c.http.Transport = s.Client().Transport
	c.Configure(s.URL)
	if _, e := c.Choose("standard"); e != nil {
		t.Fatal(e)
	}
	c.Count("ctrl_k")
	if len(c.state.Records) > 0 {
		t.Fatal("counted before policy confirmation")
	}
	c.Refresh(context.Background())
	c.Count("connection")
	c.Count("connected")
	c.Count("ctrl_k")
	c.Count("quick_command")
	c.Refresh(context.Background())
	if len(c.state.Records) != 1 {
		t.Fatalf("expected cumulative daily record, got %d", len(c.state.Records))
	}
	r := c.state.Records[0]
	if r.Active != 1 || r.Connections != 1 || r.Connected != 1 || r.CtrlK != 1 || r.QuickCommands != 1 {
		t.Fatalf("bad counters: %+v", r)
	}
	if e := r.Validate(time.Now()); e != nil {
		t.Fatal(e)
	}
	oldID := c.state.InstallationID
	c.Choose("disabled")
	if len(c.state.Records) != 0 || c.state.InstallationID != "" || c.Settings().Ready {
		t.Fatal("revoke did not clear")
	}
	c.Count("ctrl_k")
	mu.Lock()
	count := len(batches)
	mu.Unlock()
	c.Refresh(context.Background())
	mu.Lock()
	if len(batches) != count {
		t.Fatal("sent after revoke")
	}
	mu.Unlock()
	c.Choose("standard")
	if c.state.InstallationID == oldID {
		t.Fatal("identifier reused after revoke")
	}
	c.Refresh(context.Background())
	mu.Lock()
	policy = "future-policy"
	mu.Unlock()
	c.Refresh(context.Background())
	if !c.Settings().NeedsConsent || len(c.state.Records) > 0 {
		t.Fatal("policy expansion did not invalidate consent")
	}
}
func TestRevokeCancelsPendingRequest(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/telemetry-policy" {
			json.NewEncoder(w).Encode(Policy{Version: PolicyVersion})
			return
		}
		io.Copy(io.Discard, r.Body)
		close(started)
		select {
		case <-r.Context().Done():
			close(cancelled)
		case <-time.After(5 * time.Second):
		}
	}))
	defer s.Close()
	c := NewClient(filepath.Join(t.TempDir(), "service.json"), "v1.0.0")
	c.http.Transport = s.Client().Transport
	c.Configure(s.URL)
	c.Choose("standard")
	done := make(chan struct{})
	go func() { c.Refresh(context.Background()); close(done) }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("request not started")
	}
	if _, e := c.Choose("disabled"); e != nil {
		t.Fatal(e)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("request was not cancelled")
	}
	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("server did not observe cancellation")
	}
	if len(c.state.Records) > 0 {
		t.Fatal("queue survived revocation")
	}
}
func TestConsentSaveFailureAndRecipientChange(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "file")
	os.WriteFile(bad, []byte("x"), 0600)
	c := NewClient(filepath.Join(bad, "settings.json"), "v1.0.0")
	if _, e := c.Choose("standard"); e == nil {
		t.Fatal("expected save failure")
	}
	if c.state.InstallationID != "" || c.state.Choice != "" {
		t.Fatal("save failure authorized collection")
	}
	c = NewClient(filepath.Join(t.TempDir(), "settings.json"), "v1.0.0")
	c.Configure("https://one.internal")
	c.Choose("standard")
	c.Configure("https://two.internal")
	if !c.Settings().NeedsConsent || c.state.InstallationID != "" {
		t.Fatal("recipient change kept consent")
	}
	c.Choose("disabled")
	c.Configure("https://three.internal")
	if c.Settings().NeedsConsent {
		t.Fatal("disabled mode prompted again")
	}
}
func TestOtherWindowRevocationCannotBeOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	a := NewClient(path, "v1.0.0")
	a.Configure("https://ops.internal")
	a.Choose("standard")
	b := NewClient(path, "v1.0.0")
	a.readyUntil = time.Now().Add(time.Minute)
	b.Choose("disabled")
	a.Count("ctrl_k")
	if a.Settings().Choice != "disabled" || len(a.state.Records) > 0 {
		t.Fatal("other window kept collecting")
	}
	if NewClient(path, "v1.0.0").Settings().Choice != "disabled" {
		t.Fatal("refusal overwritten")
	}
}
func TestExternalRecipientEditInvalidatesConsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	c := NewClient(path, "v1.0.0")
	c.Configure("https://old.internal")
	c.Choose("standard")
	state := c.state
	state.BaseURL = "https://new.internal"
	data, _ := json.Marshal(state)
	os.WriteFile(path, data, 0600)
	if !c.Settings().NeedsConsent {
		t.Fatal("old consent adopted by new recipient")
	}
	c.Refresh(context.Background())
	if len(c.state.Records) > 0 || c.state.InstallationID != "" {
		t.Fatal("collected against edited recipient")
	}
}
func TestQueueBounds(t *testing.T) {
	c := NewClient(filepath.Join(t.TempDir(), "settings.json"), "v1.0.0")
	c.Configure("https://ops.internal")
	c.Choose("standard")
	c.readyUntil = time.Now().Add(time.Minute)
	for i := 0; i < 1002; i++ {
		c.state.Records = append(c.state.Records, Record{Date: time.Now().Format("2006-01-02")})
	}
	c.state.Records = append(c.state.Records, Record{Date: time.Now().AddDate(0, 0, -8).Format("2006-01-02")})
	c.pruneLocked()
	if len(c.state.Records) != 1000 {
		t.Fatal("queue not bounded")
	}
}

func TestConnectionCompletionDoesNotCrossConsentOrCountTwice(t *testing.T) {
	c := NewClient(filepath.Join(t.TempDir(), "settings.json"), "v1.0.0")
	c.Configure("https://ops.internal")
	c.Choose("standard")
	c.readyUntil = time.Now().Add(time.Minute)
	complete := c.BeginConnection()
	complete(true)
	complete(true)
	if c.state.Records[0].Connections != 1 || c.state.Records[0].Connected != 1 {
		t.Fatal("duplicate completion counted")
	}
	late := c.BeginConnection()
	c.Choose("disabled")
	c.Choose("standard")
	c.readyUntil = time.Now().Add(time.Minute)
	current := c.BeginConnection()
	late(true)
	if c.state.Records[0].Connected != 0 {
		t.Fatal("old connection crossed consent")
	}
	current(true)
	if c.state.Records[0].Connected != 1 {
		t.Fatal("current connection not counted")
	}
}

func TestStartupIsOnlySuccessfulAfterTargetVersionRuns(t *testing.T) {
	var mu sync.Mutex
	var received []Record
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/telemetry-policy" {
			json.NewEncoder(w).Encode(Policy{Version: PolicyVersion})
			return
		}
		var batch struct {
			Records []Record `json:"records"`
		}
		json.NewDecoder(r.Body).Decode(&batch)
		mu.Lock()
		received = append(received, batch.Records...)
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer s.Close()
	path := filepath.Join(t.TempDir(), "settings.json")
	c := NewClient(path, "v1.0.0")
	c.http.Transport = s.Client().Transport
	c.Configure(s.URL)
	c.Choose("standard")
	c.Refresh(context.Background())
	c.PendingUpgrade("upgrade", "v2.0.0")
	c.Refresh(context.Background())
	mu.Lock()
	for _, r := range received {
		if r.Phase == "startup" {
			t.Fatal("old process reported target startup")
		}
	}
	mu.Unlock()
	next := NewClient(path, "v2.0.0")
	next.http.Transport = s.Client().Transport
	next.Refresh(context.Background())
	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, r := range received {
		if r.Phase == "startup" && r.Version == "v2.0.0" && r.TargetVersion == "v2.0.0" {
			found = true
		}
	}
	if !found || next.state.Pending != nil {
		t.Fatal("new version startup not confirmed")
	}
	persisted := NewClient(path, "v2.0.0")
	if persisted.state.Pending != nil {
		t.Fatal("completed startup marker persisted after restart")
	}
	for _, r := range persisted.state.Records {
		if r.Kind == "upgrade" {
			t.Fatal("acknowledged upgrade remained in the disk queue")
		}
	}
}

func TestMinimalTierCollectsOnlyActiveVersionAndDowngradeClearsQueue(t *testing.T) {
	c := NewClient(filepath.Join(t.TempDir(), "settings.json"), "v1.2.3")
	c.Configure("https://ops.internal")
	c.Choose("standard")
	c.readyUntil = time.Now().Add(time.Minute)
	c.Count("connection")
	oldID := c.state.InstallationID
	c.Choose("minimal")
	if len(c.state.Records) != 0 || c.state.InstallationID == oldID {
		t.Fatal("downgrade retained old queue or identifier")
	}
	c.readyUntil = time.Now().Add(time.Minute)
	c.Count("active")
	c.Count("ctrl_k")
	c.Count("quick_command")
	c.BeginConnection()(true)
	c.Upgrade("id", "v1.3.0", "check", "success", "none")
	c.PendingUpgrade("id", "v1.3.0")
	if len(c.state.Records) != 1 || c.state.Pending != nil {
		t.Fatal("minimal collected extra records")
	}
	r := c.state.Records[0]
	if r.ReportingMode != "minimal" || r.Active != 1 || r.Validate(time.Now()) != nil {
		t.Fatalf("invalid minimal record: %+v", r)
	}
	data, _ := json.Marshal(r)
	var fields map[string]any
	json.Unmarshal(data, &fields)
	for _, key := range []string{"os", "arch", "connections", "connected", "ctrlK", "quickCommands", "phase", "targetVersion"} {
		if _, ok := fields[key]; ok {
			t.Fatalf("minimal serialized %s", key)
		}
	}
	next := NewClient(c.path, c.version)
	if next.Settings().Choice != "minimal" || next.Settings().NeedsConsent {
		t.Fatal("minimal consent not retained")
	}
}

func TestOldConsentRequiresNewTierSelection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	old := State{BaseURL: "https://ops.internal", Choice: "minimal", PolicyVersion: "1", InstallationID: "old", ConsentAt: time.Now(), ConsentRecipient: "https://ops.internal"}
	data, _ := json.Marshal(old)
	os.WriteFile(path, data, 0600)
	c := NewClient(path, "v1.2.3")
	if !c.Settings().NeedsConsent || c.state.InstallationID != "" || len(c.state.Records) != 0 {
		t.Fatal("old consent silently adopted new tier")
	}
}

func TestUsageConsentConcurrentClientsAndRevocation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service.json")
	a := NewClient(path, "v1.2.3")
	if _, err := a.Configure("https://ops.internal"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Choose("standard"); err != nil {
		t.Fatal(err)
	}
	a.readyUntil = time.Now().Add(time.Minute)
	b := NewClient(path, "v1.2.3")
	b.readyUntil = time.Now().Add(time.Minute)
	var wg sync.WaitGroup
	for _, c := range []*Client{a, b} {
		wg.Add(1)
		go func(c *Client) {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				finish := c.BeginUsage("cli_exec")
				finish("success")
				finish("failure")
			}
		}(c)
	}
	wg.Wait()
	a.Settings()
	if len(a.state.Records) != 1 || a.state.Records[0].Usage["cli_exec_started"] != 40 || a.state.Records[0].Usage["cli_exec_success"] != 40 || a.state.Records[0].Usage["cli_exec_failure"] != 0 {
		t.Fatalf("lost or duplicate counters: %+v", a.state.Records)
	}
	a.Count("cli_exec_--secret=password")
	if len(a.state.Records[0].Usage) != 2 {
		t.Fatal("accepted arbitrary keys")
	}
	snapshot := cloneRecords(a.state.Records)
	a.Count("cli_exec_started")
	if snapshot[0].Usage["cli_exec_started"] != 40 {
		t.Fatal("snapshot shares mutable map")
	}
	finish := a.BeginUsage("gui_script")
	if _, err := b.Choose("disabled"); err != nil {
		t.Fatal(err)
	}
	finish("success")
	a.Settings()
	if len(a.state.Records) != 0 || a.state.InstallationID != "" {
		t.Fatal("old completion survived revocation")
	}
	if _, err := b.Choose("standard"); err != nil {
		t.Fatal(err)
	}
	b.readyUntil = time.Now().Add(time.Minute)
	finish("failure")
	a.Settings()
	if len(a.state.Records) != 0 {
		t.Fatal("old completion adopted new consent")
	}
	if _, err := b.Choose("minimal"); err != nil {
		t.Fatal(err)
	}
	b.readyUntil = time.Now().Add(time.Minute)
	b.Count("gui_script_created")
	b.BeginUsage("cli_exec")("success")
	b.Count("active")
	if len(b.state.Records) != 1 || len(b.state.Records[0].Usage) != 0 {
		t.Fatal("minimal scope expanded")
	}
	reloaded := NewClient(path, "v1.2.3")
	if reloaded.Settings().NeedsConsent || reloaded.state.PolicyVersion != MinimalPolicyVersion {
		t.Fatal("unchanged minimal consent not preserved")
	}
}

func TestPreviousStandardConsentMustBeRenewed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service.json")
	state := State{BaseURL: "https://ops.internal", ConsentRecipient: "https://ops.internal", Choice: "standard", PolicyVersion: "2", ConsentAt: time.Now(), InstallationID: "old"}
	data, _ := json.Marshal(state)
	os.WriteFile(path, data, 0600)
	c := NewClient(path, "v1.2.3")
	if !c.Settings().NeedsConsent || c.Identity() != "" {
		t.Fatal("expanded standard scope inherited old consent")
	}
	c.Count("gui_script_created")
	if len(c.state.Records) != 0 {
		t.Fatal("collected before renewed consent")
	}
}
