package realtime

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Listener owns a dedicated pgx.Conn that runs LISTEN on a single channel
// and forwards every NOTIFY to the Hub. It reconnects with exponential
// backoff and is safe to Start/Stop multiple times.
type Listener struct {
	dsn     string
	channel string
	hub     *Hub
	logger  *slog.Logger
}

func NewListener(dsn, channel string, hub *Hub, logger *slog.Logger) *Listener {
	return &Listener{dsn: dsn, channel: channel, hub: hub, logger: logger}
}

// Run blocks until ctx is cancelled, maintaining the LISTEN connection
// across transient errors with exponential backoff capped at 30s.
func (l *Listener) Run(ctx context.Context) {
	backoff := time.Second
	const maxBackoff = 30 * time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		err := l.runOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			backoff = time.Second
			continue
		}
		l.logger.Warn("listener disconnected", "err", err, "retry_in", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (l *Listener) runOnce(ctx context.Context) error {
	conn, err := pgx.Connect(ctx, l.dsn)
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())

	if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{l.channel}.Sanitize()); err != nil {
		return err
	}
	l.logger.Info("listener connected", "channel", l.channel)

	for {
		notif, err := conn.WaitForNotification(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			return err
		}
		ev, err := DecodeEvent(notif.Payload)
		if err != nil {
			l.logger.Warn("decode notify payload", "err", err, "raw", notif.Payload)
			continue
		}
		l.hub.Broadcast(ev)
	}
}

// EnsurePoolCompatibility keeps the pgxpool import used somewhere; the
// Listener deliberately avoids the pool so LISTEN owns its connection.
var _ = pgxpool.Pool{}
