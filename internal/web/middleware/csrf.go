package middleware

import (
	"crypto/subtle"
	"net/http"
)

// CSRF verifies a double-submit token on state-changing requests.
// The token is generated and stored in the session by the session package
// and rendered into every form as a hidden input.
type CSRF struct {
	FieldName string
	Header    string
	Session   TokenStore
}

type TokenStore interface {
	GetToken(r *http.Request) (string, error)
}

// Protect rejects POST/PUT/PATCH/DELETE requests whose token does not match
// the one in the session. Safe methods pass through.
func (c *CSRF) Protect(next http.Handler) http.Handler {
	field := c.FieldName
	if field == "" {
		field = "csrf_token"
	}
	header := c.Header
	if header == "" {
		header = "X-CSRF-Token"
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		expected, err := c.Session.GetToken(r)
		if err != nil || expected == "" {
			http.Error(w, "missing csrf token", http.StatusForbidden)
			return
		}
		got := r.FormValue(field)
		if got == "" {
			got = r.Header.Get(header)
		}
		if subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 0 {
			http.Error(w, "invalid csrf token", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
