package triagepipe

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PGXStore is a PostgreSQL Store for the triage database.
type PGXStore struct {
	pool *pgxpool.Pool
}

// NewPGXStore wraps a pgx pool as a Store.
func NewPGXStore(pool *pgxpool.Pool) *PGXStore {
	return &PGXStore{pool: pool}
}

// EnsureSchema creates inbox, results, and outbox when missing.
func (s *PGXStore) EnsureSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS inbox (
    event_id    TEXT        PRIMARY KEY,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`,
		`CREATE TABLE IF NOT EXISTS results (
    id              TEXT             PRIMARY KEY,
    source_event_id TEXT             NOT NULL,
    customer_id     TEXT             NOT NULL,
    intent          TEXT             NOT NULL,
    intent_prob     DOUBLE PRECISION NOT NULL,
    frustration     DOUBLE PRECISION NOT NULL,
    churn_risk      DOUBLE PRECISION NOT NULL,
    wants_human     DOUBLE PRECISION NOT NULL,
    classifier      TEXT             NOT NULL,
    model_version   TEXT             NOT NULL,
    degraded        BOOLEAN          NOT NULL,
    needs_review    BOOLEAN          NOT NULL,
    created_at      TIMESTAMPTZ      NOT NULL,
    payload         JSONB            NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS outbox (
    id           BIGSERIAL PRIMARY KEY,
    event_id     TEXT        NOT NULL,
    routing_key  TEXT        NOT NULL,
    payload      JSONB       NOT NULL,
    published_at TIMESTAMPTZ NULL,
    CONSTRAINT triage_outbox_event_id_key UNIQUE (event_id)
)`,
		`CREATE INDEX IF NOT EXISTS triage_outbox_unpublished_idx
    ON outbox (id)
    WHERE published_at IS NULL`,
	}
	for _, q := range statements {
		if _, err := s.pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("triagepipe pgx: ensure schema: %w", err)
		}
	}
	return nil
}

type pgxTx struct {
	tx pgx.Tx
}

// HasInbox reports whether the source event_id was already claimed.
func (s *PGXStore) HasInbox(ctx context.Context, eventID string) (bool, error) {
	const q = `SELECT EXISTS (SELECT 1 FROM inbox WHERE event_id = $1)`
	var exists bool
	if err := s.pool.QueryRow(ctx, q, eventID).Scan(&exists); err != nil {
		return false, fmt.Errorf("triagepipe pgx: has inbox: %w", err)
	}
	return exists, nil
}

// WithTx runs fn inside a database transaction.
func (s *PGXStore) WithTx(ctx context.Context, fn func(Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("triagepipe pgx: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(&pgxTx{tx: tx}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("triagepipe pgx: commit: %w", err)
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
		return false, fmt.Errorf("triagepipe pgx: claim inbox: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// InsertResult stages one result row. Conflicts on id are ignored.
func (t *pgxTx) InsertResult(ctx context.Context, row ResultRow) error {
	const q = `
INSERT INTO results (
    id, source_event_id, customer_id, intent, intent_prob, frustration,
    churn_risk, wants_human, classifier, model_version, degraded, needs_review,
    created_at, payload
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14::jsonb)
ON CONFLICT (id) DO NOTHING`
	_, err := t.tx.Exec(
		ctx,
		q,
		row.ID,
		row.SourceEventID,
		row.CustomerID,
		row.Intent,
		row.IntentProb,
		row.Frustration,
		row.ChurnRisk,
		row.WantsHuman,
		row.Classifier,
		row.ModelVersion,
		row.Degraded,
		row.NeedsReview,
		row.CreatedAt,
		row.Payload,
	)
	if err != nil {
		return fmt.Errorf("triagepipe pgx: insert result: %w", err)
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
		return fmt.Errorf("triagepipe pgx: insert outbox: %w", err)
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
		return nil, fmt.Errorf("triagepipe pgx: list unpublished: %w", err)
	}
	defer rows.Close()

	out := make([]OutboxRow, 0)
	for rows.Next() {
		var row OutboxRow
		if err := rows.Scan(&row.EventID, &row.RoutingKey, &row.Payload); err != nil {
			return nil, fmt.Errorf("triagepipe pgx: scan: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("triagepipe pgx: rows: %w", err)
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
		return fmt.Errorf("triagepipe pgx: mark published: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("triagepipe pgx: mark published: event_id %s not found or already published", eventID)
	}
	return nil
}
