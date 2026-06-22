//go:build integration

package session_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/felipedsvit/erreia/internal/database"
	"github.com/felipedsvit/erreia/internal/session"
)

const dsn = "postgres://erreia:erreia@127.0.0.1:5432/erreia_test_session?sslmode=disable"

func newStore(t *testing.T) *session.PostgresStore {
	t.Helper()
	// Reset schema and rerun migrations.
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Skipf("skipping: cannot reach test database: %v", err)
	}
	for _, stmt := range []string{
		`DROP TABLE IF EXISTS cards CASCADE`,
		`DROP TABLE IF EXISTS columns CASCADE`,
		`DROP TABLE IF EXISTS boards CASCADE`,
		`DROP TABLE IF EXISTS users CASCADE`,
		`DROP TABLE IF EXISTS sessions CASCADE`,
		`DROP TABLE IF EXISTS schema_migrations CASCADE`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("reset: %v", err)
		}
	}
	pool, err := database.Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return session.NewPostgresStore(pool)
}

func TestPostgresStoreCommitFindDelete(t *testing.T) {
	store := newStore(t)
	expiry := time.Now().Add(time.Hour)
	if err := store.Commit("token-1", []byte("payload-1"), expiry); err != nil {
		t.Fatal(err)
	}
	data, expired, err := store.Find("token-1")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "payload-1" {
		t.Fatalf("unexpected data: %q", data)
	}
	if expired {
		t.Fatal("session should not be expired")
	}
	if err := store.Delete("token-1"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Find("token-1"); err != nil {
		t.Fatalf("expected not-found after delete, got %v", err)
	}
}

func TestPostgresStoreExpiredSession(t *testing.T) {
	store := newStore(t)
	if err := store.Commit("expired", []byte("x"), time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	_, expired, err := store.Find("expired")
	if err != nil {
		t.Fatal(err)
	}
	if !expired {
		t.Fatal("expected expired=true")
	}
}

func TestPostgresStoreMissingToken(t *testing.T) {
	store := newStore(t)
	_, _, err := store.Find("nope")
	if err != nil {
		t.Fatalf("Find on missing token should be silent, got %v", err)
	}
}
