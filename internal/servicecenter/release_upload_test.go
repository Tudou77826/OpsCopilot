package servicecenter

import (
	"archive/zip"
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
	"opscopilot/pkg/updater"
)

func releaseZIP(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	f, err := z.Create("opscopilot.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Write([]byte("MZ\x00\x00test executable")); err != nil {
		t.Fatal(err)
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func uploadRequest(t *testing.T, s *Server, version string, artifact []byte, admin bool) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	m := multipart.NewWriter(&body)
	_ = m.WriteField("version", version)
	_ = m.WriteField("notes", "## 修复\n- 修复连接问题")
	f, err := m.CreateFormFile("windows", "download.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Write(artifact); err != nil {
		t.Fatal(err)
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/admin/releases", &body)
	r.Header.Set("Content-Type", m.FormDataContentType())
	if admin {
		r.Header.Set("Authorization", "Bearer "+s.cfg.AdminToken)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func TestManualReleaseUploadPublishesOnlyCompleteArtifact(t *testing.T) {
	s := testServer(t)
	good := releaseZIP(t)
	if w := uploadRequest(t, s, "v1.2.0", good, false); w.Code != 401 {
		t.Fatalf("unauthenticated upload: %d", w.Code)
	}
	if w := uploadRequest(t, s, "v1.2.0", []byte("not a zip"), true); w.Code != 400 {
		t.Fatalf("invalid zip: %d %s", w.Code, w.Body.String())
	}
	if w := request(s, "GET", "/api/v1/releases/latest", nil, false); w.Code != 503 {
		t.Fatal("failed upload became visible")
	}
	if w := uploadRequest(t, s, "v1.2.0", good, true); w.Code != 201 {
		t.Fatalf("upload: %d %s", w.Code, w.Body.String())
	}
	w := request(s, "GET", "/api/v1/releases/latest", nil, false)
	var release protocol.ReleaseInfo
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &release) != nil {
		t.Fatalf("latest: %d %s", w.Code, w.Body.String())
	}
	digest := sha256.Sum256(good)
	if release.TagName != "v1.2.0" || release.Source != "upload" || len(release.Assets) != 1 || release.Assets[0].SHA256 != hex.EncodeToString(digest[:]) || release.Assets[0].Size != int64(len(good)) || release.Assets[0].BrowserDownloadURL != "https://ops.internal/downloads/v1.2.0/opscopilot-windows.zip" {
		t.Fatalf("wrong release contract: %+v", release)
	}
	r := httptest.NewRequest("GET", "/downloads/v1.2.0/opscopilot-windows.zip", nil)
	r.Header.Set("Range", "bytes=0-3")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 206 || !bytes.Equal(w.Body.Bytes(), good[:4]) {
		t.Fatalf("range download: %d", w.Code)
	}
	if w = uploadRequest(t, s, "v1.2.0", good, true); w.Code != 409 {
		t.Fatalf("duplicate version: %d", w.Code)
	}
	if w = uploadRequest(t, s, "v1.1.9", good, true); w.Code != 409 {
		t.Fatalf("older version: %d", w.Code)
	}
	stored, err := os.ReadFile(filepath.Join(s.cfg.DataDir, "downloads", "v1.2.0", "opscopilot-windows.zip"))
	if err != nil || !bytes.Equal(stored, good) {
		t.Fatal("published artifact was replaced")
	}
}

func TestGitHubSyncCannotRollbackManualRelease(t *testing.T) {
	s := testServer(t)
	if w := uploadRequest(t, s, "v1.3.0", releaseZIP(t), true); w.Code != 201 {
		t.Fatalf("manual upload: %d %s", w.Code, w.Body.String())
	}
	asset := []byte("older GitHub asset")
	upstream := protocol.ReleaseInfo{TagName: "v1.2.0", PublishedAt: time.Now(), Assets: []protocol.Asset{{Name: "ops.zip", Size: int64(len(asset)), BrowserDownloadURL: "https://github.com/asset"}}}
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/latest" {
			_ = json.NewEncoder(w).Encode(upstream)
		} else {
			_, _ = w.Write(asset)
		}
	}))
	defer origin.Close()
	s.cfg.GitHubURL = origin.URL + "/latest"
	s.client = origin.Client()
	s.client.Transport = rewriteTransport{base: origin.URL, transport: origin.Client().Transport}
	if err := s.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	w := request(s, "GET", "/api/v1/releases/latest", nil, false)
	if !strings.Contains(w.Body.String(), `"tag_name":"v1.3.0"`) {
		t.Fatalf("manual latest regressed: %s", w.Body.String())
	}
	w = request(s, "GET", "/api/v1/releases", nil, false)
	if !strings.Contains(w.Body.String(), `"tag_name":"v1.2.0"`) {
		t.Fatal("older synchronized history missing")
	}
	upstream.TagName = "v1.3.0"
	if err := s.Sync(context.Background()); err != nil {
		t.Fatalf("same-tag manual release should remain valid: %v", err)
	}
	w = request(s, "GET", "/api/v1/releases/latest", nil, false)
	if !strings.Contains(w.Body.String(), `"source":"upload"`) {
		t.Fatal("same-tag GitHub sync replaced manual release")
	}
}

func TestUploadWorksWhileGitHubSyncIsWaiting(t *testing.T) {
	s := testServer(t)
	started := make(chan struct{})
	resume := make(chan struct{})
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-resume:
			w.WriteHeader(http.StatusServiceUnavailable)
		case <-r.Context().Done():
		}
	}))
	defer origin.Close()
	s.cfg.GitHubURL = origin.URL
	done := make(chan error, 1)
	go func() { done <- s.Sync(context.Background()) }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("sync did not start")
	}
	if w := uploadRequest(t, s, "v1.3.0", releaseZIP(t), true); w.Code != 201 {
		t.Fatalf("upload blocked by GitHub sync: %d %s", w.Code, w.Body.String())
	}
	close(resume)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("sync did not finish")
	}
	if w := request(s, "GET", "/api/v1/releases/latest", nil, false); w.Code != 200 || !strings.Contains(w.Body.String(), `"tag_name":"v1.3.0"`) {
		t.Fatal("manual version lost after sync failure")
	}
}

func TestManualReleaseClientCheckAndDownload(t *testing.T) {
	s := testServer(t)
	network := httptest.NewServer(s.Handler())
	defer network.Close()
	s.cfg.PublicURL = network.URL
	if w := uploadRequest(t, s, "v1.4.0", releaseZIP(t), true); w.Code != 201 {
		t.Fatalf("upload: %d %s", w.Code, w.Body.String())
	}
	status, err := updater.CheckIntranetContext(context.Background(), network.URL, "v1.3.0")
	if err != nil || !status.HasUpdate || status.DownloadURL != network.URL+"/downloads/v1.4.0/opscopilot-windows.zip" {
		t.Fatalf("client did not discover update: %+v %v", status, err)
	}
	dir, err := updater.DownloadVerified(status.DownloadURL, t.TempDir(), status.Release.Assets[0].SHA256, status.Release.Assets[0].Size, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "opscopilot.exe")); err != nil {
		t.Fatalf("client could not extract uploaded release: %v", err)
	}
}
