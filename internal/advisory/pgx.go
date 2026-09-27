package advisory

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/leohteixeira/advisor-radar/internal/book"
)

// PGXStore is a PostgreSQL Store for the advisory database.
type PGXStore struct {
	pool *pgxpool.Pool
}

// NewPGXStore wraps a pgx pool as a Store.
func NewPGXStore(pool *pgxpool.Pool) *PGXStore {
	return &PGXStore{pool: pool}
}

// EnsureSchema creates book, inbox, alerts, and outbox when missing.
func (s *PGXStore) EnsureSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS book (
    customer_id TEXT PRIMARY KEY,
    name        TEXT             NOT NULL,
    segment     TEXT             NOT NULL,
    aum         DOUBLE PRECISION NOT NULL,
    advisor     TEXT             NOT NULL,
    since       TEXT             NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS inbox (
    event_id     TEXT        PRIMARY KEY,
    received_at  TIMESTAMPTZ NOT NULL DEFAULT now()
)`,
		`CREATE TABLE IF NOT EXISTS alerts (
    id              TEXT        PRIMARY KEY,
    customer_id     TEXT        NOT NULL,
    kind            TEXT        NOT NULL,
    rule            TEXT        NOT NULL,
    source_event_id TEXT        NOT NULL,
    raised_at       TIMESTAMPTZ NOT NULL,
    payload         JSONB       NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS outbox (
    id           BIGSERIAL PRIMARY KEY,
    event_id     TEXT        NOT NULL,
    routing_key  TEXT        NOT NULL,
    payload      JSONB       NOT NULL,
    published_at TIMESTAMPTZ NULL,
    CONSTRAINT advisory_outbox_event_id_key UNIQUE (event_id)
)`,
		`CREATE INDEX IF NOT EXISTS advisory_outbox_unpublished_idx
    ON outbox (id)
    WHERE published_at IS NULL`,
	}
	for _, q := range statements {
		if _, err := s.pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("advisory pgx: ensure schema: %w", err)
		}
	}
	return nil
}

// SeedBook upserts the 22-client seed book.
func (s *PGXStore) SeedBook(ctx context.Context) error {
	const q = `
INSERT INTO book (customer_id, name, segment, aum, advisor, since)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (customer_id) DO UPDATE SET
    name = EXCLUDED.name,
    segment = EXCLUDED.segment,
    aum = EXCLUDED.aum,
    advisor = EXCLUDED.advisor,
    since = EXCLUDED.since`
	for _, c := range book.Clients {
		if _, err := s.pool.Exec(ctx, q, c.ID, c.Name, c.Segment, c.AUM, c.Advisor, c.Since); err != nil {
			return fmt.Errorf("advisory pgx: seed book %s: %w", c.ID, err)
		}
	}
	return nil
}

type pgxTx struct {
	tx pgx.Tx
}

// WithTx runs fn inside a database transaction.
func (s *PGXStore) WithTx(ctx context.Context, fn func(Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("advisory pgx: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(&pgxTx{tx: tx}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("advisory pgx: commit: %w", err)
	}
	return nil
}

// ClaimInbox inserts the source event_id. Returns false when already present.
func (t *pgxTx) ClaimInbox(ctx context.Context, eventID string) (bool, error) {
	const q = `
INSERT INTO inbox (event_id)
VALUES ($1)
ON CONFLICT (event_id) DO NOTHING`
	tag, err := t.tx.Exec(ctx, q, eventID)
	if err != nil {
		return false, fmt.Errorf("advisory pgx: claim inbox: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// InsertAlert stages one alert row. Conflicts on id are ignored.
func (t *pgxTx) InsertAlert(ctx context.Context, row AlertRow) error {
	const q = `
INSERT INTO alerts (id, customer_id, kind, rule, source_event_id, raised_at, payload)
VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)
ON CONFLICT (id) DO NOTHING`
	_, err := t.tx.Exec(
		ctx,
		q,
		row.ID,
		row.CustomerID,
		row.Kind,
		row.Rule,
		row.SourceEventID,
		row.RaisedAt,
		row.Payload,
	)
	if err != nil {
		return fmt.Errorf("advisory pgx: insert alert: %w", err)
	}
	return nil
}

// InsertOutbox stages one outbox row. Conflicts on event_id are ignored.
func (t *pgxTx) InsertOutbox(ctx context.Context, row OutboxRow) error {
	const q = `
INSERT INTO outbox (event_id, routing_key, payload)
VALUES ($1, $2, $3::jsonb)
ON CONFLICT (event_id) DO NOTHING`
	_, err := t.tx.Exec(ctx, q, row.EventID, row.RoutingKey, row.Payload)
	if err != nil {
		return fmt.Errorf("advisory pgx: insert outbox: %w", err)
	}
	return nil
}

// ListUnpublished returns rows whose published_at is still null.
func (s *PGXStore) ListUnpublished(ctx context.Context) ([]OutboxRow, error) {
	const q = `
SELECT event_id, routing_key, payload
FROM outbox
WHERE published_at IS NULL
ORDER BY id`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("advisory pgx: list unpublished: %w", err)
	}
	defer rows.Close()

	out := make([]OutboxRow, 0)
	for rows.Next() {
		var row OutboxRow
		if err := rows.Scan(&row.EventID, &row.RoutingKey, &row.Payload); err != nil {
			return nil, fmt.Errorf("advisory pgx: scan: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("advisory pgx: rows: %w", err)
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
		return fmt.Errorf("advisory pgx: mark published: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("advisory pgx: mark published: event_id %s not found or already published", eventID)
	}
	return nil
}
