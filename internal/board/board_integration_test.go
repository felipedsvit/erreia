//go:build integration

package board_test

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/felipedsvit/erreia/internal/board"
	"github.com/felipedsvit/erreia/internal/database"
)

const dsn = "postgres://erreia:erreia@127.0.0.1:5432/erreia_test_board?sslmode=disable"

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
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
}

func newRepo(t *testing.T) *board.Repo {
	t.Helper()
	reset(t)
	pool, err := database.Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return board.NewRepo(pool)
}

const ownerID = "11111111-1111-1111-1111-111111111111"

func seedOwner(t *testing.T, repo *board.Repo) {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(
		`INSERT INTO users (id, email, display_name, password_hash) VALUES ($1, $2, $3, $4)`,
		ownerID, "alice@example.com", "Alice", "x",
	); err != nil {
		t.Fatal(err)
	}
}

func TestCreateBoardHasDefaultColumns(t *testing.T) {
	repo := newRepo(t)
	seedOwner(t, repo)

	b, err := repo.CreateBoard(context.Background(), ownerID, "Inbox")
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Columns) != 3 {
		t.Fatalf("expected 3 default columns, got %d", len(b.Columns))
	}
	want := []string{"To do", "In progress", "Done"}
	for i, col := range b.Columns {
		if col.Title != want[i] {
			t.Errorf("column %d: got %q want %q", i, col.Title, want[i])
		}
	}
}

func TestCreateColumnAppendsToEnd(t *testing.T) {
	repo := newRepo(t)
	seedOwner(t, repo)
	b, _ := repo.CreateBoard(context.Background(), ownerID, "X")

	c1, err := repo.CreateColumn(context.Background(), b.ID, "Review")
	if err != nil {
		t.Fatal(err)
	}
	c2, err := repo.CreateColumn(context.Background(), b.ID, "Archive")
	if err != nil {
		t.Fatal(err)
	}
	if c1.Position >= c2.Position {
		t.Fatalf("expected c2 after c1, got %d and %d", c1.Position, c2.Position)
	}
}

func TestCreateCardAppendsToColumn(t *testing.T) {
	repo := newRepo(t)
	seedOwner(t, repo)
	b, _ := repo.CreateBoard(context.Background(), ownerID, "X")
	colID := b.Columns[0].ID

	c1, err := repo.CreateCard(context.Background(), colID, "first")
	if err != nil {
		t.Fatal(err)
	}
	c2, err := repo.CreateCard(context.Background(), colID, "second")
	if err != nil {
		t.Fatal(err)
	}
	if c1.Position >= c2.Position {
		t.Errorf("expected c2 after c1, got %d and %d", c1.Position, c2.Position)
	}
}

func TestGetBoardReturnsTree(t *testing.T) {
	repo := newRepo(t)
	seedOwner(t, repo)
	b, _ := repo.CreateBoard(context.Background(), ownerID, "X")
	colID := b.Columns[0].ID
	if _, err := repo.CreateCard(context.Background(), colID, "alpha"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateCard(context.Background(), colID, "beta"); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetBoard(context.Background(), b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "X" {
		t.Errorf("title mismatch: %q", got.Title)
	}
	if len(got.Columns) != 3 {
		t.Errorf("expected 3 columns, got %d", len(got.Columns))
	}
	if len(got.Columns[0].Cards) != 2 {
		t.Errorf("expected 2 cards in first column, got %d", len(got.Columns[0].Cards))
	}
}

func TestMoveCardBetweenColumns(t *testing.T) {
	repo := newRepo(t)
	seedOwner(t, repo)
	b, _ := repo.CreateBoard(context.Background(), ownerID, "X")
	todo, done := b.Columns[0].ID, b.Columns[2].ID

	c1, err := repo.CreateCard(context.Background(), todo, "task1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateCard(context.Background(), todo, "task2"); err != nil {
		t.Fatal(err)
	}

	moved, boardID, err := repo.MoveCard(context.Background(), c1.ID, done, 0)
	if err != nil {
		t.Fatal(err)
	}
	if boardID != b.ID {
		t.Errorf("board id: got %q want %q", boardID, b.ID)
	}
	if moved.ColumnID != done || moved.Position != 0 {
		t.Errorf("moved card in wrong spot: %+v", moved)
	}

	// Re-fetch and check positions are dense.
	got, _ := repo.GetBoard(context.Background(), b.ID)
	for _, col := range got.Columns {
		for i, card := range col.Cards {
			if card.Position != i {
				t.Errorf("position gap in %s: %s at %d, expected %d", col.ID, card.ID, card.Position, i)
			}
		}
	}
}

func TestMoveCardToInvalidColumnFails(t *testing.T) {
	repo := newRepo(t)
	seedOwner(t, repo)
	b, _ := repo.CreateBoard(context.Background(), ownerID, "X")
	colID := b.Columns[0].ID
	c, _ := repo.CreateCard(context.Background(), colID, "task")

	if _, _, err := repo.MoveCard(context.Background(), c.ID, "00000000-0000-0000-0000-000000000000", 0); err == nil {
		t.Fatal("expected error moving to nonexistent column")
	}
}

func TestUpdateCard(t *testing.T) {
	repo := newRepo(t)
	seedOwner(t, repo)
	b, _ := repo.CreateBoard(context.Background(), ownerID, "X")
	c, _ := repo.CreateCard(context.Background(), b.Columns[0].ID, "old")

	updated, err := repo.UpdateCard(context.Background(), c.ID, "new", "with description")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != "new" || updated.Description != "with description" {
		t.Errorf("update didn't take: %+v", updated)
	}
}

func TestDeleteCard(t *testing.T) {
	repo := newRepo(t)
	seedOwner(t, repo)
	b, _ := repo.CreateBoard(context.Background(), ownerID, "X")
	c, _ := repo.CreateCard(context.Background(), b.Columns[0].ID, "doomed")

	if err := repo.DeleteCard(context.Background(), c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetCard(context.Background(), c.ID); err != board.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestBoardOwnership(t *testing.T) {
	repo := newRepo(t)
	seedOwner(t, repo)
	b, _ := repo.CreateBoard(context.Background(), ownerID, "Mine")
	owner, err := repo.BoardOwnerOf(context.Background(), b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if owner != ownerID {
		t.Errorf("owner mismatch: %q", owner)
	}
	if _, err := repo.BoardOwnerOf(context.Background(), "00000000-0000-0000-0000-000000000000"); err != board.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestCardBoardIDResolution(t *testing.T) {
	repo := newRepo(t)
	seedOwner(t, repo)
	b, _ := repo.CreateBoard(context.Background(), ownerID, "X")
	c, _ := repo.CreateCard(context.Background(), b.Columns[0].ID, "task")
	got, err := repo.CardBoardID(context.Background(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != b.ID {
		t.Errorf("card board id: %q vs %q", got, b.ID)
	}
}
