package servicecenter

import "testing"

func TestHTTPServiceAddressAndDownloadURL(t *testing.T) {
	base, err := ValidateBase("http://88.45.4.2:8090/")
	if err != nil || base != "http://88.45.4.2:8090" {
		t.Fatalf("ValidateBase() = %q, %v", base, err)
	}
	if !DownloadURL(base, base+"/downloads/v1.2.3/opscopilot-windows.zip") {
		t.Fatal("same-origin HTTP download rejected")
	}
	for _, raw := range []string{
		"http://88.45.4.2:8091/downloads/v1.2.3/opscopilot-windows.zip",
		"https://88.45.4.2:8090/downloads/v1.2.3/opscopilot-windows.zip",
		"http://88.45.4.2:8090/downloads/v1.2.3/opscopilot-windows.zip?token=x",
	} {
		if DownloadURL(base, raw) {
			t.Fatalf("unexpectedly accepted download URL %q", raw)
		}
	}
	for _, raw := range []string{"http:88.45.4.2:8090", "http://88.45.4.2:8090/path", "http://user@88.45.4.2:8090"} {
		if _, err := ValidateBase(raw); err == nil {
			t.Fatalf("unexpectedly accepted service address %q", raw)
		}
	}
}
