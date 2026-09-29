package advisory

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PGXStore is a PostgreSQL Store for the advisory database.
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

// InvestorProfile reads the customer's book profile inside the transaction.
// A customer outside the book wraps ErrUnknownCustomer.
func (t *pgxTx) InvestorProfile(ctx context.Context, customerID string) (string, error) {
	const q = `
SELECT investor_profile
FROM book
WHERE customer_id = $1::uuid`
	var profile string
	err := t.tx.QueryRow(ctx, q, customerID).Scan(&profile)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("advisory pgx: investor profile: %w: %w", ErrUnknownCustomer, err)
	}
	if err != nil {
		return "", fmt.Errorf("advisory pgx: investor profile: %w", err)
	}
	return profile, nil
}

// UpdateBook sets aum and segment for one customer. aum is whole USD dollars.
func (t *pgxTx) UpdateBook(ctx context.Context, customerID string, aum float64, segment string) error {
	const q = `
UPDATE book
SET aum = $2, segment = $3
WHERE customer_id = $1::uuid`
	tag, err := t.tx.Exec(ctx, q, customerID, aum, segment)
	if err != nil {
		return fmt.Errorf("advisory pgx: update book: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("advisory pgx: update book %s: %w", customerID, ErrUnknownCustomer)
	}
	return nil
}

// InsertAlert stages one alert row. Conflicts on id are ignored. A zero
// SourceSchemaVersion is stored as NULL.
func (t *pgxTx) InsertAlert(ctx context.Context, row AlertRow) error {
	const q = `
INSERT INTO alerts (id, customer_id, kind, rule, source_event_id, raised_at, payload, source_schema_version)
VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, NULLIF($8::integer, 0))
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
		row.SourceSchemaVersion,
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
