//go:build integration

package database_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/felipedsvit/erreia/internal/database"
)

// dsn returns the DSN to use for integration tests. Each test package
// gets its own database to avoid contention when tests run in parallel.
func dsn() string {
	if v := os.Getenv("ERREIA_TEST_DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://erreia:erreia@127.0.0.1:5432/erreia_test_db?sslmode=disable"
}

// startPostgres is a no-op when an external Postgres is already
// available; it is a placeholder for future container orchestration.
// Tests should be skipped when no DB is reachable.
func startPostgres(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		db, err := sql.Open("pgx", dsn())
		if err == nil {
			if err := db.Ping(); err == nil {
				_ = db.Close()
				return
			}
			_ = db.Close()
		}
		if time.Now().After(deadline) {
			t.Skipf("skipping: cannot reach test database: %v", err)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func TestMigrationsApplyCleanly(t *testing.T) {
	startPostgres(t)
	// Reset the schema for a fresh run.
	db, err := sql.Open("pgx", dsn())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
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
	_ = db.Close()

	pool, err := database.Open(context.Background(), dsn())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// Calling Open again should be a no-op (ErrNoChange).
	if _, err := database.Open(context.Background(), dsn()); err != nil {
		t.Fatal(err)
	}

	// Verify the tables exist (filter to public schema to avoid surprises
	// in databases that have multiple user schemas).
	db2, err := sql.Open("pgx", dsn())
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	for _, table := range []string{"users", "sessions", "boards", "columns", "cards"} {
		var n int
		if err := db2.QueryRow(
			`SELECT count(*) FROM information_schema.tables
             WHERE table_schema = 'public' AND table_name = $1`, table,
		).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("expected table %s to exist in public schema", table)
		}
	}
}
