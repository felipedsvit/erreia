package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alexedwards/scs/v2"

	"github.com/felipedsvit/erreia/internal/web/middleware"
)

// wrap primes a session by routing the request through sm.LoadAndSave
// and returns the new request with all session cookies attached.
func wrap(t *testing.T, sm *scs.SessionManager, init func(http.ResponseWriter, *http.Request)) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	sm.LoadAndSave(http.HandlerFunc(init)).ServeHTTP(rec, req)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	return rec, req
}

func TestRequireAuthRedirectsUnauthenticated(t *testing.T) {
	t.Parallel()
	sm := scs.New()
	handler := middleware.RequireAuth(sm)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	// Wrap through LoadAndSave so the context carries a session.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/secret", nil)
	wrapped := sm.LoadAndSave(handler)
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Fatalf("expected redirect to /login, got %q", loc)
	}
}

func TestRequireAuthHTMXReturns401(t *testing.T) {
	t.Parallel()
	sm := scs.New()
	handler := middleware.RequireAuth(sm)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/secret", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	sm.LoadAndSave(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if h := rec.Header().Get("HX-Redirect"); h != "/login" {
		t.Fatalf("expected HX-Redirect, got %q", h)
	}
}

func TestRequireAuthPassesWithSession(t *testing.T) {
	t.Parallel()
	sm := scs.New()
	handler := middleware.RequireAuth(sm)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request: store the userID. Second request: pass the cookie.
	rec1, req1 := wrap(t, sm, func(w http.ResponseWriter, r *http.Request) {
		middleware.SetUserID(sm, r, "user-42")
		w.WriteHeader(http.StatusOK)
	})
	_ = rec1

	// Now wrap the protected handler with LoadAndSave and reuse the cookies.
	rec2 := httptest.NewRecorder()
	sm.LoadAndSave(handler).ServeHTTP(rec2, req1)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec2.Code)
	}
}

func TestUserIDRoundTrip(t *testing.T) {
	t.Parallel()
	sm := scs.New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	sm.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		middleware.SetUserID(sm, r, "u-1")
		uid := middleware.UserIDFromContext(sm, r)
		if uid != "u-1" {
			t.Fatalf("uid mismatch: got %q", uid)
		}
		middleware.ClearUserID(sm, r)
		uid = middleware.UserIDFromContext(sm, r)
		if uid != "" {
			t.Fatalf("uid should be empty after clear, got %q", uid)
		}
	})).ServeHTTP(rec, req)
	if strings.Contains(rec.Body.String(), "FAIL") {
		t.Fatal("subtest failed")
	}
}
