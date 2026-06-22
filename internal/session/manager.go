package session

import (
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewManager configures a scs.SessionManager with our Postgres store and
// the supplied cookie/lifetime settings.
func NewManager(pool *pgxpool.Pool, cookieName string, lifetime time.Duration, secure bool) *scs.SessionManager {
	sm := scs.New()
	sm.Store = NewPostgresStore(pool)
	sm.Lifetime = lifetime
	sm.IdleTimeout = 1 * time.Hour
	sm.Cookie.Name = cookieName
	sm.Cookie.HttpOnly = true
	sm.Cookie.SameSite = http.SameSiteLaxMode
	sm.Cookie.Secure = secure
	sm.Cookie.Path = "/"
	return sm
}
