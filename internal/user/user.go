package user

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type User struct {
	ID          string
	Email       string
	DisplayName string
	AvatarKey   string
	AvatarURL   string // populated by handlers, not persisted
	CSRFToken   string // populated by handlers, not persisted
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Board struct {
	ID        string
	OwnerID   string
	Title     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Repo struct {
	pool *pgxpool.Pool
}

func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

var ErrNotFound = errors.New("not found")
var ErrEmailTaken = errors.New("email already in use")

func (r *Repo) Create(ctx context.Context, email, displayName, passwordHash string) (*User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	var u User
	err := r.pool.QueryRow(ctx, `
        INSERT INTO users (email, display_name, password_hash)
        VALUES ($1, $2, $3)
        RETURNING id, email, display_name, avatar_key, created_at, updated_at
    `, email, displayName, passwordHash).Scan(
		&u.ID, &u.Email, &u.DisplayName, &u.AvatarKey, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrEmailTaken
		}
		return nil, fmt.Errorf("insert user: %w", err)
	}
	return &u, nil
}

func (r *Repo) GetByID(ctx context.Context, id string) (*User, error) {
	var u User
	err := r.pool.QueryRow(ctx, `
        SELECT id, email, display_name, avatar_key, created_at, updated_at
        FROM users WHERE id = $1
    `, id).Scan(&u.ID, &u.Email, &u.DisplayName, &u.AvatarKey, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *Repo) GetByEmail(ctx context.Context, email string) (*User, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	var u User
	var hash string
	err := r.pool.QueryRow(ctx, `
        SELECT id, email, display_name, avatar_key, password_hash, created_at, updated_at
        FROM users WHERE email = $1
    `, email).Scan(&u.ID, &u.Email, &u.DisplayName, &u.AvatarKey, &hash, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	return &u, hash, nil
}

func (r *Repo) SetAvatarKey(ctx context.Context, userID, key string) error {
	_, err := r.pool.Exec(ctx, `
        UPDATE users SET avatar_key = $1, updated_at = now() WHERE id = $2
    `, key, userID)
	return err
}

func (r *Repo) ListBoards(ctx context.Context, ownerID string) ([]Board, error) {
	rows, err := r.pool.Query(ctx, `
        SELECT id, owner_id, title, created_at, updated_at
        FROM boards WHERE owner_id = $1
        ORDER BY updated_at DESC
    `, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Board
	for rows.Next() {
		var b Board
		if err := rows.Scan(&b.ID, &b.OwnerID, &b.Title, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// isUniqueViolation keeps the auth handlers free of pgx-specific imports.
func isUniqueViolation(err error) bool {
	return strings.Contains(err.Error(), "SQLSTATE 23505")
}
