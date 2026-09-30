package updater

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifiedDownloadRejectsTamperingBeforeExtraction(t *testing.T) {
	var data bytes.Buffer
	archive := zip.NewWriter(&data)
	f, _ := archive.Create("opscopilot.exe")
	f.Write([]byte("test binary"))
	archive.Close()
	sum := sha256.Sum256(data.Bytes())
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(data.Bytes()) }))
	defer s.Close()
	dir := t.TempDir()
	if _, e := DownloadVerified(s.URL+"/ops.zip", dir, hex.EncodeToString(sum[:]), int64(data.Len()), nil); e != nil {
		t.Fatal(e)
	}
	dir = t.TempDir()
	if _, e := DownloadVerified(s.URL+"/ops.zip", dir, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", int64(data.Len()), nil); e == nil {
		t.Fatal("tampered artifact accepted")
	}
	if _, e := os.Stat(filepath.Join(dir, "extracted", "opscopilot.exe")); !os.IsNotExist(e) {
		t.Fatal("untrusted artifact extracted")
	}
}

func TestCheckIntranetOverHTTP(t *testing.T) {
	var network *httptest.Server
	network = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/releases/latest":
			_ = json.NewEncoder(w).Encode(ReleaseInfo{TagName: "v1.11.2", Assets: []Asset{{Name: "opscopilot-windows.zip", BrowserDownloadURL: network.URL + "/downloads/v1.11.2/opscopilot-windows.zip", SHA256: strings.Repeat("a", 64), Size: 4}}})
		case "/api/v1/releases":
			_ = json.NewEncoder(w).Encode([]ReleaseInfo{})
		default:
			http.NotFound(w, r)
		}
	}))
	defer network.Close()

	status, err := CheckIntranet(network.URL, "v1.11.1")
	if err != nil {
		t.Fatal(err)
	}
	if !status.HasUpdate || status.DownloadURL != network.URL+"/downloads/v1.11.2/opscopilot-windows.zip" {
		t.Fatalf("unexpected HTTP update status: %+v", status)
	}
}
