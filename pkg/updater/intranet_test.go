package updater

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
