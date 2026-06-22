package realtime

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Publisher emits NOTIFY messages on the channel that Listener is LISTENing
// to. It uses a short-lived connection acquired from the pool to avoid
// holding a connection for the lifetime of the request.
type Publisher struct {
	pool    *pgxpool.Pool
	channel string
}

func NewPublisher(pool *pgxpool.Pool, channel string) *Publisher {
	return &Publisher{pool: pool, channel: channel}
}

// Notify runs pg_notify on a pooled connection.
func (p *Publisher) Notify(ctx context.Context, payload string) error {
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire conn: %w", err)
	}
	defer conn.Release()
	_, err = conn.Exec(ctx, "SELECT pg_notify($1, $2)", p.channel, payload)
	if err != nil {
		return fmt.Errorf("pg_notify: %w", err)
	}
	return nil
}

var _ = pgx.ErrNoRows
