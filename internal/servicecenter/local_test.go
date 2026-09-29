package servicecenter

import (
	"strings"
	"testing"
)

func TestLocalHTTPRequiresExplicitLoopbackMode(t *testing.T) {
	for _, tc := range []struct {
		url            string
		enabled, valid bool
	}{
		{"http://127.0.0.1:19080", true, true},
		{"http://localhost:19080", true, true},
		{"http://[::1]:19080", true, true},
		{"http://127.0.0.1:19080", false, false},
		{"http://192.168.1.2:19080", true, false},
		{"http://localhost.evil.example:19080", true, false},
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
