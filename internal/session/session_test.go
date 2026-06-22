package session_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alexedwards/scs/v2"

	"github.com/felipedsvit/erreia/internal/session"
)

func newManager() *scs.SessionManager {
	return scs.New()
}

func TestGetOrCreateCSRFStable(t *testing.T) {
	t.Parallel()
	sm := newManager()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	var first string
	sm.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok, err := session.GetOrCreateCSRF(sm, r)
		if err != nil {
			t.Fatal(err)
		}
		first = tok
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	if first == "" {
		t.Fatal("expected non-empty token")
	}

	// Second request with the same cookie should yield the same token.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rec.Result().Cookies() {
		req2.AddCookie(c)
	}
	rec2 := httptest.NewRecorder()
	var second string
	sm.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok, _ := session.GetOrCreateCSRF(sm, r)
		second = tok
	})).ServeHTTP(rec2, req2)
	if first != second {
		t.Fatalf("expected stable token, got %q then %q", first, second)
	}
}

func TestSetAndPopFlash(t *testing.T) {
	t.Parallel()
	sm := newManager()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	sm.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session.SetFlash(sm, r, "info", "hello")
	})).ServeHTTP(rec, req)

	// Carry the cookie into a follow-up request and pop the flash.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rec.Result().Cookies() {
		req2.AddCookie(c)
	}
	rec2 := httptest.NewRecorder()
	sm.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, ok := session.PopFlash(sm, r)
		if !ok || f.Kind != "info" || f.Message != "hello" {
			t.Fatalf("unexpected flash: %+v ok=%v", f, ok)
		}
	})).ServeHTTP(rec2, req2)

	// A second pop on a fresh request should return ok=false.
	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rec2.Result().Cookies() {
		req3.AddCookie(c)
	}
	rec3 := httptest.NewRecorder()
	sm.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := session.PopFlash(sm, r); ok {
			t.Fatal("flash should be gone after pop")
		}
	})).ServeHTTP(rec3, req3)
}

func TestSetAndGetCSRFManual(t *testing.T) {
	t.Parallel()
	sm := newManager()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	sm.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testToken := "test-csrf-token-12345"
		session.SetCSRF(sm, r, testToken)
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, req)

	// Verify the token was stored
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range rec.Result().Cookies() {
		req2.AddCookie(c)
	}
	rec2 := httptest.NewRecorder()
	sm.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		store := session.CSRFStore{SM: sm}
		tok, err := store.GetToken(r)
		if err != nil {
			t.Fatal(err)
		}
		if tok != "test-csrf-token-12345" {
			t.Fatalf("expected test-csrf-token-12345, got %q", tok)
		}
	})).ServeHTTP(rec2, req2)
}

func TestNewTokenUniqueness(t *testing.T) {
	t.Parallel()
	tok1, err1 := session.NewToken()
	tok2, err2 := session.NewToken()
	if err1 != nil || err2 != nil {
		t.Fatalf("NewToken failed: %v, %v", err1, err2)
	}
	if tok1 == tok2 {
		t.Fatal("NewToken should produce unique tokens")
	}
}
