package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/felipedsvit/erreia/internal/web/middleware"
)

func TestSecurityHeadersSetsAllHeaders(t *testing.T) {
	t.Parallel()
	called := false
	h := middleware.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if !called {
		t.Fatal("downstream handler not called")
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options: got %q, want nosniff", got)
	}
	if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options: got %q, want DENY", got)
	}
	if got := rec.Header().Get("Referrer-Policy"); got == "" {
		t.Error("Referrer-Policy not set")
	}
	if got := rec.Header().Get("Content-Security-Policy"); got == "" {
		t.Error("Content-Security-Policy not set")
	}
	if got := rec.Header().Get("Permissions-Policy"); got == "" {
		t.Error("Permissions-Policy not set")
	}
}
