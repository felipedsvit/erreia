package board

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Board struct {
	ID        string
	OwnerID   string
	Title     string
	CreatedAt time.Time
	UpdatedAt time.Time
	Columns   []Column
}

type Column struct {
	ID        string
	BoardID   string
	Title     string
	Position  int
	CreatedAt time.Time
	UpdatedAt time.Time
	Cards     []Card
}

type Card struct {
	ID          string
	ColumnID    string
	Title       string
	Description string
	Position    int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

var ErrNotFound = errors.New("not found")

// CreateBoard inserts a board plus a small set of default columns so the
// UI is useful from the first render.
func (r *Repo) CreateBoard(ctx context.Context, ownerID, title string) (*Board, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var b Board
	if err := tx.QueryRow(ctx, `
        INSERT INTO boards (owner_id, title) VALUES ($1, $2)
        RETURNING id, owner_id, title, created_at, updated_at
    `, ownerID, title).Scan(&b.ID, &b.OwnerID, &b.Title, &b.CreatedAt, &b.UpdatedAt); err != nil {
		return nil, err
	}

	defaults := []string{"To do", "In progress", "Done"}
	b.Columns = make([]Column, 0, len(defaults))
	for i, t := range defaults {
		var c Column
		if err := tx.QueryRow(ctx, `
            INSERT INTO columns (board_id, title, position)
            VALUES ($1, $2, $3)
            RETURNING id, board_id, title, position, created_at, updated_at
        `, b.ID, t, i).Scan(&c.ID, &c.BoardID, &c.Title, &c.Position, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		b.Columns = append(b.Columns, c)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &b, nil
}

// GetBoard returns the full board tree (columns + cards) ordered by position.
func (r *Repo) GetBoard(ctx context.Context, boardID string) (*Board, error) {
	var b Board
	err := r.pool.QueryRow(ctx, `
        SELECT id, owner_id, title, created_at, updated_at
        FROM boards WHERE id = $1
    `, boardID).Scan(&b.ID, &b.OwnerID, &b.Title, &b.CreatedAt, &b.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	colRows, err := r.pool.Query(ctx, `
        SELECT id, board_id, title, position, created_at, updated_at
        FROM columns WHERE board_id = $1
        ORDER BY position ASC, created_at ASC
    `, boardID)
	if err != nil {
		return nil, err
	}
	defer colRows.Close()

	colIndex := map[string]int{}
	for colRows.Next() {
		var c Column
		if err := colRows.Scan(&c.ID, &c.BoardID, &c.Title, &c.Position, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		colIndex[c.ID] = len(b.Columns)
		b.Columns = append(b.Columns, c)
	}
	if err := colRows.Err(); err != nil {
		return nil, err
	}

	if len(b.Columns) == 0 {
		return &b, nil
	}

	cardRows, err := r.pool.Query(ctx, `
        SELECT id, column_id, title, description, position, created_at, updated_at
        FROM cards
        WHERE column_id = ANY($1)
        ORDER BY position ASC, created_at ASC
    `, columnIDs(b.Columns))
	if err != nil {
		return nil, err
	}
	defer cardRows.Close()
	for cardRows.Next() {
		var c Card
		if err := cardRows.Scan(&c.ID, &c.ColumnID, &c.Title, &c.Description, &c.Position, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		if idx, ok := colIndex[c.ColumnID]; ok {
			b.Columns[idx].Cards = append(b.Columns[idx].Cards, c)
		}
	}
	return &b, cardRows.Err()
}

// CreateColumn appends a new column to the end of the board.
func (r *Repo) CreateColumn(ctx context.Context, boardID, title string) (*Column, error) {
	var pos int
	if err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(MAX(position), -1) + 1 FROM columns WHERE board_id = $1`,
		boardID,
	).Scan(&pos); err != nil {
		return nil, err
	}
	var c Column
	if err := r.pool.QueryRow(ctx, `
        INSERT INTO columns (board_id, title, position)
        VALUES ($1, $2, $3)
        RETURNING id, board_id, title, position, created_at, updated_at
    `, boardID, title, pos).Scan(&c.ID, &c.BoardID, &c.Title, &c.Position, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}
	return &c, nil
}

// CreateCard appends a new card to the end of a column.
func (r *Repo) CreateCard(ctx context.Context, columnID, title string) (*Card, error) {
	var pos int
	if err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(MAX(position), -1) + 1 FROM cards WHERE column_id = $1`,
		columnID,
	).Scan(&pos); err != nil {
		return nil, err
	}
	var c Card
	if err := r.pool.QueryRow(ctx, `
        INSERT INTO cards (column_id, title, position)
        VALUES ($1, $2, $3)
        RETURNING id, column_id, title, description, position, created_at, updated_at
    `, columnID, title, pos).Scan(&c.ID, &c.ColumnID, &c.Title, &c.Description, &c.Position, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}
	return &c, nil
}

// GetColumn loads a single column with its cards in position order.
func (r *Repo) GetColumn(ctx context.Context, columnID string) (*Column, error) {
	var c Column
	err := r.pool.QueryRow(ctx, `
        SELECT id, board_id, title, position, created_at, updated_at
        FROM columns WHERE id = $1
    `, columnID).Scan(&c.ID, &c.BoardID, &c.Title, &c.Position, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, `
        SELECT id, column_id, title, description, position, created_at, updated_at
        FROM cards WHERE column_id = $1
        ORDER BY position ASC, created_at ASC
    `, columnID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var card Card
		if err := rows.Scan(&card.ID, &card.ColumnID, &card.Title, &card.Description, &card.Position, &card.CreatedAt, &card.UpdatedAt); err != nil {
			return nil, err
		}
		c.Cards = append(c.Cards, card)
	}
	return &c, rows.Err()
}

// GetCard loads a single card.
func (r *Repo) GetCard(ctx context.Context, cardID string) (*Card, error) {
	var c Card
	err := r.pool.QueryRow(ctx, `
        SELECT id, column_id, title, description, position, created_at, updated_at
        FROM cards WHERE id = $1
    `, cardID).Scan(&c.ID, &c.ColumnID, &c.Title, &c.Description, &c.Position, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// UpdateCard mutates the editable fields of a card.
func (r *Repo) UpdateCard(ctx context.Context, cardID, title, description string) (*Card, error) {
	var c Card
	err := r.pool.QueryRow(ctx, `
        UPDATE cards SET title = $1, description = $2, updated_at = now()
        WHERE id = $3
        RETURNING id, column_id, title, description, position, created_at, updated_at
    `, title, description, cardID).Scan(
		&c.ID, &c.ColumnID, &c.Title, &c.Description, &c.Position, &c.CreatedAt, &c.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// DeleteCard removes a card permanently.
func (r *Repo) DeleteCard(ctx context.Context, cardID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM cards WHERE id = $1`, cardID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// MoveCard relocates a card to (columnID, position) and shifts neighbours.
// It returns the card and the board ID it lives on (so the caller can NOTIFY).
func (r *Repo) MoveCard(ctx context.Context, cardID, destColumnID string, destPos int) (*Card, string, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var srcColumnID string
	var srcPos int
	if err := tx.QueryRow(ctx,
		`SELECT column_id, position FROM cards WHERE id = $1 FOR UPDATE`,
		cardID,
	).Scan(&srcColumnID, &srcPos); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, "", ErrNotFound
		}
		return nil, "", err
	}

	var boardID string
	if err := tx.QueryRow(ctx, `SELECT board_id FROM columns WHERE id = $1`, destColumnID).Scan(&boardID); err != nil {
		return nil, "", err
	}

	// Step 1: take the card out of the source column.
	if _, err := tx.Exec(ctx, `
        UPDATE cards SET position = position - 1, updated_at = now()
        WHERE column_id = $1 AND position > $2
    `, srcColumnID, srcPos); err != nil {
		return nil, "", err
	}

	// Step 2: make a gap in the destination column.
	if _, err := tx.Exec(ctx, `
        UPDATE cards SET position = position + 1, updated_at = now()
        WHERE column_id = $1 AND position >= $2
    `, destColumnID, destPos); err != nil {
		return nil, "", err
	}

	// Step 3: place the card.
	var c Card
	if err := tx.QueryRow(ctx, `
        UPDATE cards SET column_id = $1, position = $2, updated_at = now()
        WHERE id = $3
        RETURNING id, column_id, title, description, position, created_at, updated_at
    `, destColumnID, destPos, cardID).Scan(
		&c.ID, &c.ColumnID, &c.Title, &c.Description, &c.Position, &c.CreatedAt, &c.UpdatedAt,
	); err != nil {
		return nil, "", err
	}

	if _, err := tx.Exec(ctx, `UPDATE boards SET updated_at = now() WHERE id = $1`, boardID); err != nil {
		return nil, "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, "", err
	}
	return &c, boardID, nil
}

// BoardOwnerOf returns the owner of a board (used for authorization).
func (r *Repo) BoardOwnerOf(ctx context.Context, boardID string) (string, error) {
	var owner string
	err := r.pool.QueryRow(ctx, `SELECT owner_id FROM boards WHERE id = $1`, boardID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return owner, nil
}

// ColumnBoardID returns the board that owns a given column.
func (r *Repo) ColumnBoardID(ctx context.Context, columnID string) (string, error) {
	var boardID string
	err := r.pool.QueryRow(ctx, `SELECT board_id FROM columns WHERE id = $1`, columnID).Scan(&boardID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return boardID, err
}

// CardBoardID returns the board that owns a given card (via its column).
func (r *Repo) CardBoardID(ctx context.Context, cardID string) (string, error) {
	var boardID string
	err := r.pool.QueryRow(ctx, `
        SELECT c.board_id
        FROM columns c
        JOIN cards ca ON ca.column_id = c.id
        WHERE ca.id = $1
    `, cardID).Scan(&boardID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return boardID, err
}

func columnIDs(cs []Column) []string {
	ids := make([]string, len(cs))
	for i, c := range cs {
		ids[i] = c.ID
	}
	return ids
}
