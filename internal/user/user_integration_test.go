//go:build integration

package user_test

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/felipedsvit/erreia/internal/database"
	"github.com/felipedsvit/erreia/internal/user"
)

const dsn = "postgres://erreia:erreia@127.0.0.1:5432/erreia_test_user?sslmode=disable"

// reset drops the relevant tables and re-applies migrations.
// We share a single pool across tests for simplicity.
func reset(t *testing.T) {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
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
			db.Close()
			t.Fatalf("reset: %v", err)
		}
	}
	_ = db.Close()

	pool, err := database.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(pool.Close)
}

func newRepo(t *testing.T) *user.Repo {
	t.Helper()
	reset(t)
	pool, err := database.Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return user.NewRepo(pool)
}

func TestUserCreateAndFetch(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()

	u, err := repo.Create(ctx, "alice@example.com", "Alice", "argon2id$hash$here")
	if err != nil {
		t.Fatal(err)
	}
	if u.ID == "" || u.Email != "alice@example.com" {
		t.Fatalf("unexpected user: %+v", u)
	}
	if u.CreatedAt.IsZero() {
		t.Fatal("created_at not set")
	}

	got, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != u.ID {
		t.Fatalf("ids differ: %s vs %s", got.ID, u.ID)
	}
}

func TestUserDuplicateEmail(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	if _, err := repo.Create(ctx, "dup@example.com", "First", "h"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, "dup@example.com", "Second", "h"); err != user.ErrEmailTaken {
		t.Fatalf("expected ErrEmailTaken, got %v", err)
	}
}

func TestUserEmailLowercased(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	u, err := repo.Create(ctx, "MIXED@example.com", "M", "h")
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != "mixed@example.com" {
		t.Fatalf("email not lowercased: %q", u.Email)
	}
}

func TestSetAvatarKey(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	u, err := repo.Create(ctx, "avatar@example.com", "A", "h")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SetAvatarKey(ctx, u.ID, "avatars/u/x.jpg"); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AvatarKey != "avatars/u/x.jpg" {
		t.Fatalf("avatar key not stored: %q", got.AvatarKey)
	}
}

func TestGetByEmailReturnsHash(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	const hash = "argon2id$v=19$m=65536,t=2,p=2$..."
	if _, err := repo.Create(ctx, "hash@example.com", "H", hash); err != nil {
		t.Fatal(err)
	}
	u, gotHash, err := repo.GetByEmail(ctx, "hash@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if u == nil || gotHash != hash {
		t.Fatalf("hash mismatch: %q vs %q", gotHash, hash)
	}
}

func TestGetByEmailNotFound(t *testing.T) {
	repo := newRepo(t)
	_, _, err := repo.GetByEmail(context.Background(), "nobody@example.com")
	if err != user.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestListBoardsEmpty(t *testing.T) {
	repo := newRepo(t)
	bs, err := repo.ListBoards(context.Background(), "00000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 0 {
		t.Fatalf("expected no boards, got %d", len(bs))
	}
}
