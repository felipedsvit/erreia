package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/alexedwards/scs/v2"

	"github.com/felipedsvit/erreia/internal/web/middleware"
)

type tokenStore struct{ sm *scs.SessionManager }

func (t tokenStore) GetToken(r *http.Request) (string, error) {
	v, _ := t.sm.Get(r.Context(), "csrf").(string)
	return v, nil
}

// primeSession issues a session, stamps a CSRF token in it, and returns
// a request that carries the resulting session cookie.
func primeSession(t *testing.T, sm *scs.SessionManager, tok string) *http.Request {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	sm.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sm.Put(r.Context(), "csrf", tok)
	})).ServeHTTP(rec, req)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	return req
}

// runWithSession runs fn inside the session middleware so the context
// the CSRF middleware receives has session data.
func runWithSession(sm *scs.SessionManager, req *http.Request, fn http.Handler) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	sm.LoadAndSave(fn).ServeHTTP(rec, req)
	return rec
}

func TestCSRFAcceptsValidFormToken(t *testing.T) {
	t.Parallel()
	sm := scs.New()
	const tok = "good-token"
	req := primeSession(t, sm, tok)

	form := url.Values{"csrf_token": {tok}, "x": {"1"}}
	r2 := httptest.NewRequest(http.MethodPost, "/submit", strings.NewReader(form.Encode()))
	r2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range req.Cookies() {
		r2.AddCookie(c)
	}

	csrf := &middleware.CSRF{FieldName: "csrf_token", Header: "X-CSRF-Token", Session: tokenStore{sm: sm}}
	called := false
	rec := runWithSession(sm, r2, csrf.Protect(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})))
	if !called {
		t.Fatalf("handler not called for valid token (status %d)", rec.Code)
	}
}

func TestCSRFRejectsMissingToken(t *testing.T) {
	t.Parallel()
	sm := scs.New()
	req := primeSession(t, sm, "good-token")

	r2 := httptest.NewRequest(http.MethodPost, "/submit", strings.NewReader("x=1"))
	r2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range req.Cookies() {
		r2.AddCookie(c)
	}

	csrf := &middleware.CSRF{Session: tokenStore{sm: sm}}
	rec := runWithSession(sm, r2, csrf.Protect(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler should not be called")
	})))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestCSRFRejectsMismatchedToken(t *testing.T) {
	t.Parallel()
	sm := scs.New()
	req := primeSession(t, sm, "good-token")

	form := url.Values{"csrf_token": {"bad-token"}}
	r2 := httptest.NewRequest(http.MethodPost, "/submit", strings.NewReader(form.Encode()))
	r2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range req.Cookies() {
		r2.AddCookie(c)
	}

	csrf := &middleware.CSRF{Session: tokenStore{sm: sm}}
	rec := runWithSession(sm, r2, csrf.Protect(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler should not be called")
	})))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestCSRFAllowsSafeMethods(t *testing.T) {
	t.Parallel()
	sm := scs.New()
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		req := httptest.NewRequest(method, "/", nil)
		rec := httptest.NewRecorder()
		called := false
		(&middleware.CSRF{Session: tokenStore{sm: sm}}).Protect(
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			}),
		).ServeHTTP(rec, req)
		if !called {
			t.Errorf("%s: handler not called", method)
		}
	}
}

func TestCSRFHeaderToken(t *testing.T) {
	t.Parallel()
	sm := scs.New()
	const tok = "header-token"
	req := primeSession(t, sm, tok)

	r2 := httptest.NewRequest(http.MethodPost, "/submit", strings.NewReader(""))
	r2.Header.Set("X-CSRF-Token", tok)
	for _, c := range req.Cookies() {
		r2.AddCookie(c)
	}

	csrf := &middleware.CSRF{Session: tokenStore{sm: sm}}
	called := false
	runWithSession(sm, r2, csrf.Protect(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})))
	if !called {
		t.Fatal("header token not accepted")
	}
}
