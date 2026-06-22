package middleware

import (
	"net/http"

	"github.com/alexedwards/scs/v2"
)

const userIDKey = "userID"

// RequireAuth rejects any request that is not backed by a session carrying
// a user ID. It expects scs.SessionManager.LoadAndSave to have already run.
func RequireAuth(sm *scs.SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			uid, ok := sm.Get(r.Context(), userIDKey).(string)
			if !ok || uid == "" {
				if isHTMX(r) {
					w.Header().Set("HX-Redirect", "/login")
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func isHTMX(r *http.Request) bool { return r.Header.Get("HX-Request") == "true" }

// SetUserID stores the authenticated user's ID in the session.
func SetUserID(sm *scs.SessionManager, r *http.Request, userID string) {
	sm.Put(r.Context(), userIDKey, userID)
}

// ClearUserID removes the user ID from the session (used on logout).
func ClearUserID(sm *scs.SessionManager, r *http.Request) {
	sm.Remove(r.Context(), userIDKey)
}

// UserIDFromContext extracts the user ID previously set by RequireAuth.
// Returns empty string when not present.
func UserIDFromContext(sm *scs.SessionManager, r *http.Request) string {
	if v, ok := sm.Get(r.Context(), userIDKey).(string); ok {
		return v
	}
	return ""
}
