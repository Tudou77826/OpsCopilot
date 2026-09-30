package servicecenter

import (
	"context"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	protocol "opscopilot/pkg/servicecenter"
)

func TestHTTPRequiresExplicitServerOptIn(t *testing.T) {
	for _, tc := range []struct {
		url            string
		enabled, valid bool
	}{
		{"http://127.0.0.1:19080", true, true},
		{"http://localhost:19080", true, true},
		{"http://[::1]:19080", true, true},
		{"http://127.0.0.1:19080", false, false},
		{"http://192.168.1.2:19080", true, true},
		{"http://88.45.4.2:8090", true, true},
		{"http://88.45.4.2:8090", false, false},
		{"http://localhost.evil.example:19080", true, true},
		{"http://localhost:19080/path", true, false},
	} {
		t.Run(tc.url, func(t *testing.T) {
			s, e := New(Config{DataDir: t.TempDir(), PublicURL: tc.url, AdminToken: strings.Repeat("x", 32), LocalHTTP: tc.enabled})
			if tc.valid {
				if e != nil {
					t.Fatal(e)
				}
				s.Close()
			} else if e == nil {
				s.Close()
				t.Fatal("unsafe local configuration accepted")
			}
		})
	}
}

func TestHTTPClientConnectsToDirectService(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	base := "http://" + listener.Addr().String()
	service, err := New(Config{DataDir: t.TempDir(), PublicURL: base, AdminToken: strings.Repeat("x", 32), LocalHTTP: true})
	if err != nil {
		listener.Close()
		t.Fatal(err)
	}
	defer service.Close()
	server := &http.Server{Handler: service.Handler()}
	defer server.Close()
	go server.Serve(listener)

	client := protocol.NewClient(filepath.Join(t.TempDir(), "service-center.json"), "v1.11.2")
	settings, err := client.Configure(base)
	if err != nil || settings.BaseURL != base {
		t.Fatalf("Configure() = %+v, %v", settings, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.TestConnection(ctx, base); err != nil {
		t.Fatalf("TestConnection() = %v", err)
	}
}
