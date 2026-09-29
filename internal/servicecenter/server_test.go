package servicecenter

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	protocol "opscopilot/pkg/servicecenter"
)

func TestReportingModesDeduplicateByInstallationAndLatestConsent(t *testing.T) {
	at := time.Now().UTC()
	records := []protocol.Record{
		{Kind: "daily", Date: "2026-09-29", InstallationID: "a", ReportingMode: "standard", ConsentAt: at},
		{Kind: "daily", Date: "2026-09-29", InstallationID: "a", ReportingMode: "minimal", ConsentAt: at.Add(time.Minute)},
		{Kind: "daily", Date: "2026-09-29", InstallationID: "b", ReportingMode: "standard", ConsentAt: at},
		{Kind: "daily", Date: "2026-09-29", InstallationID: "b", ReportingMode: "standard", ConsentAt: at, Version: "v2"},
	}
	for _, rows := range [][]protocol.Record{records, {records[3], records[2], records[1], records[0]}} {
		counts := aggregate(rows)["2026-09-29"].Reporting
		if counts["standard"] != 1 || counts["minimal"] != 1 || len(counts) != 2 {
			t.Fatalf("unexpected authorization counts: %v", counts)
		}
	}
}

func testServer(t *testing.T) *Server {
	t.Helper()
	s, e := New(Config{DataDir: t.TempDir(), PublicURL: "https://ops.internal", AdminToken: strings.Repeat("secret", 6)})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func request(s *Server, method, path string, body []byte, admin bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, bytes.NewReader(body))
	if admin {
		r.Header.Set("Authorization", "Bearer "+s.cfg.AdminToken)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}
func record() protocol.Record {
	r := protocol.Record{ReportingMode: "standard", InstallationID: "install", Date: time.Now().Format("2006-01-02"), Version: "v1.0.0", OS: "windows", Arch: "amd64", PolicyVersion: protocol.PolicyVersion, ConsentAt: time.Now().Add(-time.Hour), Kind: "daily", Active: 1, Connections: 2, Connected: 1, CtrlK: 3, QuickCommands: 4}
	r.ID = r.InstallationID + ":" + r.Date + ":" + r.Version
	return r
}
func TestTelemetryWhitelistIdempotencyAndAtomicBatch(t *testing.T) {
	s := testServer(t)
	r := record()
	payload, _ := json.Marshal(map[string]any{"records": []protocol.Record{r}})
	for i := 0; i < 2; i++ {
		w := request(s, "POST", "/api/v1/events/batch", payload, false)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	r.Connections = 1
	r.CtrlK = 1
	payload, _ = json.Marshal(map[string]any{"records": []protocol.Record{r}})
	request(s, "POST", "/api/v1/events/batch", payload, false)
	var rows []protocol.Record
	s.list("events", &rows)
	if len(rows) != 1 || rows[0].Connections != 2 || rows[0].CtrlK != 3 {
		t.Fatalf("retries double-counted or regressed: %+v", rows)
	}
	invalid := []string{`{"records":[{"hostname":"secret"}]}`, strings.Replace(string(payload), `"policyVersion":"`+protocol.PolicyVersion+`"`, `"policyVersion":"invalid"`, 1), string(payload) + `{}`}
	for _, p := range invalid {
		if request(s, "POST", "/api/v1/events/batch", []byte(p), false).Code != 400 {
			t.Fatal("accepted disallowed data")
		}
	}
	r = record()
	r.ID = "other"
	r.InstallationID = "other"
	bad := record()
	bad.QuickCommands = -1
	payload, _ = json.Marshal(map[string]any{"records": []protocol.Record{r, bad}})
	if request(s, "POST", "/api/v1/events/batch", payload, false).Code != 400 {
		t.Fatal("invalid batch accepted")
	}
	s.list("events", &rows)
	if len(rows) != 1 {
		t.Fatal("partially stored invalid batch")
	}
}
func TestAnnouncementValidityAndAdminAuthentication(t *testing.T) {
	s := testServer(t)
	a := protocol.Announcement{Text: "内网公告", URL: "https://ops.internal/help", StartsAt: time.Now().Add(-time.Hour), EndsAt: time.Now().Add(time.Hour)}
	data, _ := json.Marshal(a)
	if request(s, "POST", "/api/admin/announcements", data, false).Code != 401 {
		t.Fatal("admin endpoint unprotected")
	}
	if request(s, "POST", "/api/admin/announcements", data, true).Code != 200 {
		t.Fatal("announcement save failed")
	}
	a.Text = "已过期"
	a.EndsAt = time.Now().Add(-time.Minute)
	data, _ = json.Marshal(a)
	request(s, "POST", "/api/admin/announcements", data, true)
	w := request(s, "GET", "/api/v1/announcements", nil, false)
	var all []protocol.Announcement
	json.Unmarshal(w.Body.Bytes(), &all)
	if len(all) != 1 || all[0].Text != "内网公告" {
		t.Fatal("expired announcements exposed")
	}
	a.URL = "https://evil.example"
	data, _ = json.Marshal(a)
	if request(s, "POST", "/api/admin/announcements", data, true).Code != 400 {
		t.Fatal("external announcement link accepted")
	}
	for _, path := range []string{"/", "/help", "/feedback", "/admin", "/app.js", "/style.css", "/healthz"} {
		if request(s, "GET", path, nil, false).Code != 200 {
			t.Fatal("page missing", path)
		}
	}
}
func TestFeedbackCredentialReplyAndAttachment(t *testing.T) {
	s := testServer(t)
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	form.WriteField("content", "用户主动提供的问题")
	form.WriteField("confirmed", "yes")
	f, _ := form.CreateFormFile("attachment", "private.txt")
	f.Write([]byte("selected attachment"))
	form.Close()
	r := httptest.NewRequest("POST", "/api/v1/feedback", &body)
	r.Header.Set("Content-Type", form.FormDataContentType())
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var created map[string]string
	json.Unmarshal(w.Body.Bytes(), &created)
	q, _ := json.Marshal(map[string]string{"id": created["id"], "token": "wrong"})
	if request(s, "POST", "/api/v1/feedback/lookup", q, false).Code != 404 {
		t.Fatal("feedback readable without credential")
	}
	q, _ = json.Marshal(map[string]string{"status": "已解决", "reply": "已经处理"})
	if request(s, "POST", "/api/admin/feedback/"+created["id"], q, true).Code != 200 {
		t.Fatal("reply failed")
	}
	q, _ = json.Marshal(map[string]string{"id": created["id"], "token": created["token"]})
	w = request(s, "POST", "/api/v1/feedback/lookup", q, false)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "已经处理") || strings.Contains(w.Body.String(), "tokenHash") {
		t.Fatal("bad lookup", w.Body.String())
	}
	w = request(s, "GET", "/api/admin/feedback/"+created["id"]+"/attachment", nil, true)
	if w.Code != 200 || w.Body.String() != "selected attachment" {
		t.Fatal("attachment unavailable")
	}
	if request(s, "GET", "/api/admin/feedback/"+created["id"]+"/attachment", nil, false).Code != 401 {
		t.Fatal("public attachment")
	}
	if strings.Contains(request(s, "GET", "/api/admin/feedback", nil, true).Body.String(), "tokenHash") {
		t.Fatal("credential hash leaked")
	}
}

type rewriteTransport struct {
	base      string
	transport http.RoundTripper
}

func (rt rewriteTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	copy := r.Clone(r.Context())
	u := *r.URL
	copy.URL = &u
	target := strings.TrimPrefix(rt.base, "https://")
	copy.URL.Host = target
	copy.URL.Scheme = "https"
	return rt.transport.RoundTrip(copy)
}
func TestMirrorCompletePublishRangeAndOfflineFallback(t *testing.T) {
	s := testServer(t)
	asset := []byte("complete installer archive")
	digest := sha256.Sum256(asset)
	upstream := protocol.ReleaseInfo{TagName: "v1.2.0", PublishedAt: time.Now(), Assets: []protocol.Asset{{Name: "ops.zip", Size: int64(len(asset)), BrowserDownloadURL: "https://github.com/asset", Digest: "sha256:" + hex.EncodeToString(digest[:])}}}
	broken := false
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/latest" {
			json.NewEncoder(w).Encode(upstream)
			return
		}
		if broken {
			w.Write(asset[:3])
		} else {
			w.Write(asset)
		}
	}))
	s.cfg.GitHubURL = origin.URL + "/latest"
	s.client = origin.Client()
	s.client.Transport = rewriteTransport{base: origin.URL, transport: origin.Client().Transport}
	if e := s.Sync(context.Background()); e != nil {
		t.Fatal(e)
	}
	w := request(s, "GET", "/api/v1/releases/latest", nil, false)
	var mirrored protocol.ReleaseInfo
	json.Unmarshal(w.Body.Bytes(), &mirrored)
	if !strings.HasPrefix(mirrored.Assets[0].BrowserDownloadURL, s.cfg.PublicURL) || len(mirrored.Assets[0].SHA256) != 64 {
		t.Fatal("mirror URL/checksum missing")
	}
	r := httptest.NewRequest("GET", "/downloads/v1.2.0/ops.zip", nil)
	r.Header.Set("Range", "bytes=2-5")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 206 || w.Body.String() != string(asset[2:6]) {
		t.Fatal("range not supported", w.Code)
	}
	broken = true
	upstream.TagName = "v1.3.0"
	if s.Sync(context.Background()) == nil {
		t.Fatal("incomplete release published")
	}
	origin.Close()
	if s.Sync(context.Background()) == nil {
		t.Fatal("expected offline failure")
	}
	w = request(s, "GET", "/api/v1/releases/latest", nil, false)
	if !strings.Contains(w.Body.String(), "v1.2.0") {
		t.Fatal("last completed version lost")
	}
	w = request(s, "GET", "/downloads/v1.2.0/ops.zip", nil, false)
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), asset) {
		t.Fatal("offline cache unavailable")
	}
	if _, e := os.Stat(filepath.Join(s.cfg.DataDir, "downloads", "v1.3.0")); !os.IsNotExist(e) {
		t.Fatal("incomplete version left published")
	}
}
func TestRetentionAnonymizesThenExpires(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	r := record()
	r.Date = now.AddDate(0, 0, -91).Format("2006-01-02")
	r.ID = r.InstallationID + ":" + r.Date + ":" + r.Version
	s.put("events", r.ID, r)
	if e := s.Maintenance(now); e != nil {
		t.Fatal(e)
	}
	var rows []protocol.Record
	var totals []Aggregate
	s.list("events", &rows)
	s.list("aggregates", &totals)
	if len(rows) != 0 || len(totals) != 1 || totals[0].Connections != 2 {
		t.Fatal("retention conversion failed")
	}
	data, _ := json.Marshal(totals)
	if bytes.Contains(data, []byte("install")) {
		t.Fatal("identifier in anonymous totals")
	}
	s.Maintenance(now.AddDate(1, 0, 0))
	s.list("aggregates", &totals)
	if len(totals) != 0 {
		t.Fatal("aggregate retained over a year")
	}
}

func TestMinimalModeRejectsExtraFields(t *testing.T) {
	s := testServer(t)
	r := record()
	r.ReportingMode = "minimal"
	r.PolicyVersion = protocol.MinimalPolicyVersion
	r.OS = ""
	r.Arch = ""
	r.Connections = 0
	r.Connected = 0
	r.CtrlK = 0
	r.QuickCommands = 0
	payload, _ := json.Marshal(map[string]any{"records": []protocol.Record{r}})
	if w := request(s, "POST", "/api/v1/events/batch", payload, false); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, alter := range []func(*protocol.Record){func(r *protocol.Record) { r.CtrlK = 1 }, func(r *protocol.Record) { r.OS = "windows" }, func(r *protocol.Record) { r.ReportingMode = "" }} {
		bad := r
		alter(&bad)
		payload, _ = json.Marshal(map[string]any{"records": []protocol.Record{bad}})
		if request(s, "POST", "/api/v1/events/batch", payload, false).Code != 400 {
			t.Fatal("accepted fields outside minimal tier")
		}
	}
}

func TestUsageAllowlistCumulativeSnapshotsAndRetention(t *testing.T) {
	s := testServer(t)
	r := record()
	post := func(record protocol.Record) int {
		payload, _ := json.Marshal(map[string]any{"records": []protocol.Record{record}})
		return request(s, "POST", "/api/v1/events/batch", payload, false).Code
	}
	r.Usage = map[string]int{"gui_script_created": 3, "cli_exec_started": 7, "cli_exec_failure": 2}
	if post(r) != 200 || post(r) != 200 {
		t.Fatal("valid usage rejected")
	}
	r.Usage = map[string]int{"cli_exec_started": 4, "cli_exec_success": 5}
	if post(r) != 200 {
		t.Fatal("out of order snapshot rejected")
	}
	var rows []protocol.Record
	s.list("events", &rows)
	totals := aggregate(rows)[r.Date]
	if totals.Usage["cli_exec_started"] != 7 || totals.Usage["gui_script_created"] != 3 || totals.Usage["cli_exec_success"] != 5 {
		t.Fatalf("invalid totals: %+v", totals)
	}
	for _, usage := range []map[string]int{{"command_text": 1}, {"cli_exec_success": -1}, {"gui_script_created": 1000001}} {
		bad := r
		bad.Usage = usage
		if post(bad) != 400 {
			t.Fatal("invalid usage accepted")
		}
	}
	minimal := r
	minimal.ReportingMode = "minimal"
	minimal.PolicyVersion = protocol.MinimalPolicyVersion
	minimal.OS = ""
	minimal.Arch = ""
	minimal.Connections = 0
	minimal.Connected = 0
	minimal.CtrlK = 0
	minimal.QuickCommands = 0
	if post(minimal) != 400 {
		t.Fatal("minimal usage accepted")
	}
	day, err := time.Parse("2006-01-02", r.Date)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Maintenance(day.AddDate(0, 0, 91).Add(12 * time.Hour)); err != nil {
		t.Fatal(err)
	}
	var stored []Aggregate
	s.list("aggregates", &stored)
	if len(stored) != 1 || stored[0].Usage["cli_exec_started"] != 7 {
		t.Fatal("usage lost during anonymous retention")
	}
}
