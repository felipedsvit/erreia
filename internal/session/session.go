package session

import (
	"encoding/gob"
	"net/http"

	"github.com/alexedwards/scs/v2"
)

const (
	flashKey     = "flash"
	csrfKey      = "csrf_token"
	flashInfo    = "info"
	flashSuccess = "success"
	flashError   = "error"
)

// Flash is a one-shot message rendered on the next page load.
type Flash struct {
	Kind    string
	Message string
}

// SetFlash stores a flash to be shown on the next request.
func SetFlash(sm *scs.SessionManager, r *http.Request, kind, msg string) {
	sm.Put(r.Context(), flashKey, Flash{Kind: kind, Message: msg})
}

// PopFlash reads and removes the flash from the session in one step.
func PopFlash(sm *scs.SessionManager, r *http.Request) (Flash, bool) {
	v, ok := sm.Pop(r.Context(), flashKey).(Flash)
	return v, ok
}

// GetOrCreateCSRF returns a stable CSRF token, generating one on first use.
// The token rotates on logout.
func GetOrCreateCSRF(sm *scs.SessionManager, r *http.Request) (string, error) {
	if v, ok := sm.Get(r.Context(), csrfKey).(string); ok && v != "" {
		return v, nil
	}
	tok, err := NewToken()
	if err != nil {
		return "", err
	}
	sm.Put(r.Context(), csrfKey, tok)
	return tok, nil
}

// SetCSRF lets handlers force a specific token (used in tests).
func SetCSRF(sm *scs.SessionManager, r *http.Request, tok string) {
	sm.Put(r.Context(), csrfKey, tok)
}

// CSRFStore implements middleware.TokenStore by reading the session value.
type CSRFStore struct{ SM *scs.SessionManager }

func (c CSRFStore) GetToken(r *http.Request) (string, error) {
	if s, ok := c.SM.Get(r.Context(), csrfKey).(string); ok {
		return s, nil
	}
	return "", nil
}

// init registers the session payload types with encoding/gob so that
// scs can encode them transparently. Without this, putting a Flash in
// the session produces "type not registered for interface" at encode time.
func init() { gob.Register(Flash{}) }
