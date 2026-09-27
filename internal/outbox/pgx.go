package outbox

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PGXStore is a PostgreSQL outbox Store for the account_sim database.
type PGXStore struct {
	pool *pgxpool.Pool
}

// NewPGXStore wraps a pgx pool as a Store.
func NewPGXStore(pool *pgxpool.Pool) *PGXStore {
	return &PGXStore{pool: pool}
}

type pgxTx struct {
	tx pgx.Tx
}

// WithTx runs fn inside a database transaction.
func (s *PGXStore) WithTx(ctx context.Context, fn func(Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("outbox pgx: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(&pgxTx{tx: tx}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("outbox pgx: commit: %w", err)
	}
	return nil
}

// Insert stages one outbox row. Conflicts on event_id are ignored.
func (t *pgxTx) Insert(ctx context.Context, row Row) error {
	const q = `
INSERT INTO outbox (event_id, routing_key, payload)
VALUES ($1, $2, $3::jsonb)
ON CONFLICT (event_id) DO NOTHING`
	_, err := t.tx.Exec(ctx, q, row.EventID, row.RoutingKey, row.Payload)
	if err != nil {
		return fmt.Errorf("outbox pgx: insert: %w", err)
	}
	return nil
}

// ListUnpublished returns rows whose published_at is still null.
func (s *PGXStore) ListUnpublished(ctx context.Context) ([]Row, error) {
	const q = `
SELECT event_id, routing_key, payload
FROM outbox
WHERE published_at IS NULL
ORDER BY id`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("outbox pgx: list unpublished: %w", err)
	}
	defer rows.Close()

	out := make([]Row, 0)
	for rows.Next() {
		var row Row
		if err := rows.Scan(&row.EventID, &row.RoutingKey, &row.Payload); err != nil {
			return nil, fmt.Errorf("outbox pgx: scan: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("outbox pgx: rows: %w", err)
	}
	return out, nil
}

// MarkPublished stamps published_at after the broker accepts the message.
func (s *PGXStore) MarkPublished(ctx context.Context, eventID string, at time.Time) error {
	const q = `
UPDATE outbox
SET published_at = $2
WHERE event_id = $1 AND published_at IS NULL`
	tag, err := s.pool.Exec(ctx, q, eventID, at)
	if err != nil {
		return fmt.Errorf("outbox pgx: mark published: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("outbox pgx: mark published: event_id %s not found or already published", eventID)
	}
	return nil
}
