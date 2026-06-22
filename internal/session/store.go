package session

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore implements scs.Store backed by the `sessions` table.
type PostgresStore struct {
	pool    *pgxpool.Pool
	timeout time.Duration
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool, timeout: 5 * time.Second}
}

// Find returns the session data for a token. The bool return is `found` per
// the scs.Store contract: false when the row is missing or expired; true
// when a valid, non-expired session exists.
func (s *PostgresStore) Find(token string) ([]byte, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	var data []byte
	var expiry time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT data, expiry FROM sessions WHERE token = $1`, token,
	).Scan(&data, &expiry)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if expiry.Before(time.Now()) {
		return nil, false, nil
	}
	return data, true, nil
}

// Commit upserts a session row.
func (s *PostgresStore) Commit(token string, data []byte, expiry time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	_, err := s.pool.Exec(ctx, `
        INSERT INTO sessions (token, data, expiry)
        VALUES ($1, $2, $3)
        ON CONFLICT (token) DO UPDATE
        SET data = EXCLUDED.data, expiry = EXCLUDED.expiry
    `, token, data, expiry)
	return err
}

// Delete removes a session row.
func (s *PostgresStore) Delete(token string) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token = $1`, token)
	return err
}

// NewToken returns a 32-byte URL-safe token used for CSRF.
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
