package servicecenter

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminAssetsAndProtectedData(t *testing.T) {
	s := testServer(t)
	for _, path := range []string{"/admin", "/admin?demo=1", "/admin-assets/admin.css", "/admin-assets/admin.js"} {
		r := httptest.NewRecorder()
		s.Handler().ServeHTTP(r, httptest.NewRequest("GET", path, nil))
		if r.Code != 200 {
			t.Fatalf("%s: status %d", path, r.Code)
		}
		if !strings.Contains(r.Header().Get("Content-Security-Policy"), "script-src 'self'") {
			t.Fatalf("%s: missing strict script policy", path)
		}
	}
	r := httptest.NewRecorder()
	s.Handler().ServeHTTP(r, httptest.NewRequest("GET", "/api/admin/stats?demo=1", nil))
	if r.Code != 401 {
		t.Fatalf("demo query must not grant data access: %d", r.Code)
	}
}
