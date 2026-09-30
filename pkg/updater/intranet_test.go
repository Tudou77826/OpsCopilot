package updater

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
