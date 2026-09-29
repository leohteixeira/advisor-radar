package cases

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PGXStore is a PostgreSQL Store for the cases database.
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

var _ Tx = (*pgxTx)(nil)

// WithTx runs fn inside a database transaction.
func (s *PGXStore) WithTx(ctx context.Context, fn func(Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("cases pgx: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(&pgxTx{tx: tx}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("cases pgx: commit: %w", err)
	}
	return nil
}

// GetCase loads one case by id.
func (s *PGXStore) GetCase(ctx context.Context, id string) (CaseRow, bool, error) {
	return getCase(ctx, s.pool, id)
}

func (t *pgxTx) GetCase(ctx context.Context, id string) (CaseRow, bool, error) {
	return getCase(ctx, t.tx, id)
}

type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func getCase(ctx context.Context, q rowQuerier, id string) (CaseRow, bool, error) {
	const query = `
SELECT id, customer_id, COALESCE(signal_id::text, ''), advisor_id, state, sla_total_minutes, escalated, opened_at
FROM cases
WHERE id = $1`
	var row CaseRow
	err := q.QueryRow(ctx, query, id).Scan(
		&row.ID,
		&row.CustomerID,
		&row.SignalID,
		&row.AdvisorID,
		&row.State,
		&row.SLATotalMinutes,
		&row.Escalated,
		&row.OpenedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CaseRow{}, false, nil
		}
		return CaseRow{}, false, fmt.Errorf("cases pgx: get case: %w", err)
	}
	return row, true, nil
}

// ClaimInbox inserts the source event_id. Returns false when already present.
func (t *pgxTx) ClaimInbox(ctx context.Context, eventID string) (bool, error) {
	const q = `
INSERT INTO inbox (event_id)
VALUES ($1)
ON CONFLICT (event_id) DO NOTHING`
	tag, err := t.tx.Exec(ctx, q, eventID)
	if err != nil {
		return false, fmt.Errorf("cases pgx: claim inbox: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// customerLockSpace is the first key of the two-key advisory lock, so the
// per-customer case locks cannot collide with single-key locks or other spaces.
const customerLockSpace int32 = 0x43415345 // "CASE"

// LockCustomer takes a transaction-scoped advisory lock for one customer.
func (t *pgxTx) LockCustomer(ctx context.Context, customerID string) error {
	if _, err := t.tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock($1::int4, hashtext($2))`, customerLockSpace, customerID,
	); err != nil {
		return fmt.Errorf("cases pgx: lock customer: %w", err)
	}
	return nil
}

// OpenCaseFor returns the customer's case that is not Resolvido, if any. It
// locks the row, so a concurrent Advance to Resolvido waits for this
// transaction and a message cannot join a case resolved under it.
func (t *pgxTx) OpenCaseFor(ctx context.Context, customerID string) (CaseRow, bool, error) {
	return openCaseFor(ctx, t.tx, customerID, true)
}

// OpenCaseFor reads the customer's case that is not Resolvido, if any,
// outside any transaction.
func (s *PGXStore) OpenCaseFor(ctx context.Context, customerID string) (CaseRow, bool, error) {
	return openCaseFor(ctx, s.pool, customerID, false)
}

// InboxSeen reports whether eventID is already claimed.
func (s *PGXStore) InboxSeen(ctx context.Context, eventID string) (bool, error) {
	var seen bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM inbox WHERE event_id = $1)`, eventID).Scan(&seen)
	if err != nil {
		return false, fmt.Errorf("cases pgx: inbox seen: %w", err)
	}
	return seen, nil
}

func openCaseFor(ctx context.Context, q rowQuerier, customerID string, forUpdate bool) (CaseRow, bool, error) {
	query := `
SELECT id, customer_id, COALESCE(signal_id::text, ''), advisor_id, state, sla_total_minutes, escalated, opened_at
FROM cases
WHERE customer_id = $1 AND state <> $2
ORDER BY opened_at DESC
LIMIT 1`
	if forUpdate {
		query += `
FOR UPDATE`
	}
	var row CaseRow
	err := q.QueryRow(ctx, query, customerID, StateResolvido).Scan(
		&row.ID,
		&row.CustomerID,
		&row.SignalID,
		&row.AdvisorID,
		&row.State,
		&row.SLATotalMinutes,
		&row.Escalated,
		&row.OpenedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CaseRow{}, false, nil
		}
		return CaseRow{}, false, fmt.Errorf("cases pgx: open case for customer: %w", err)
	}
	return row, true, nil
}

// InsertHistory stages one case_history row.
func (t *pgxTx) InsertHistory(ctx context.Context, row HistoryRow) error {
	const q = `
INSERT INTO case_history (id, case_id, kind, text, occurred_at)
VALUES ($1, $2, $3, $4, $5)`
	if _, err := t.tx.Exec(ctx, q, row.ID, row.CaseID, row.Kind, row.Text, row.OccurredAt); err != nil {
		return fmt.Errorf("cases pgx: insert history: %w", err)
	}
	return nil
}

// InsertCase stages one case row. Conflicts on id are ignored.
func (t *pgxTx) InsertCase(ctx context.Context, row CaseRow) error {
	const q = `
INSERT INTO cases (id, customer_id, signal_id, advisor_id, state, sla_total_minutes, escalated, opened_at)
VALUES ($1, $2, NULLIF($3, '')::uuid, $4, $5, $6, $7, $8)
ON CONFLICT (id) DO NOTHING`
	_, err := t.tx.Exec(
		ctx,
		q,
		row.ID,
		row.CustomerID,
		row.SignalID,
		row.AdvisorID,
		row.State,
		row.SLATotalMinutes,
		row.Escalated,
		row.OpenedAt,
	)
	if err != nil {
		return fmt.Errorf("cases pgx: insert case: %w", err)
	}
	return nil
}

// UpdateCase writes state and escalated flag for an existing case.
func (t *pgxTx) UpdateCase(ctx context.Context, row CaseRow) error {
	const q = `
UPDATE cases
SET state = $2, escalated = $3, sla_total_minutes = $4
WHERE id = $1`
	tag, err := t.tx.Exec(ctx, q, row.ID, row.State, row.Escalated, row.SLATotalMinutes)
	if err != nil {
		return fmt.Errorf("cases pgx: update case: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("cases pgx: update case: %s not found", row.ID)
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
		return fmt.Errorf("cases pgx: insert outbox: %w", err)
	}
	return nil
}

// ArmDelay records one unpublished SLA delay. A second arm of the same case is a no-op.
func (t *pgxTx) ArmDelay(ctx context.Context, caseID string, ttlMs int) error {
	const q = `
INSERT INTO sla_delay (case_id, ttl_ms)
VALUES ($1, $2)
ON CONFLICT (case_id) DO NOTHING`
	if _, err := t.tx.Exec(ctx, q, caseID, ttlMs); err != nil {
		return fmt.Errorf("cases pgx: arm delay: %w", err)
	}
	return nil
}

// ListUnpublishedDelays returns delays the broker has not accepted yet.
func (s *PGXStore) ListUnpublishedDelays(ctx context.Context) ([]DelayArm, error) {
	const q = `
SELECT case_id, ttl_ms
FROM sla_delay
WHERE published_at IS NULL
ORDER BY case_id`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("cases pgx: list delays: %w", err)
	}
	defer rows.Close()
	out := make([]DelayArm, 0)
	for rows.Next() {
		var row DelayArm
		if err := rows.Scan(&row.CaseID, &row.TTLMs); err != nil {
			return nil, fmt.Errorf("cases pgx: scan delay: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cases pgx: list delays: %w", err)
	}
	return out, nil
}

// MarkDelayPublished stamps a delay after the broker accepts it.
func (s *PGXStore) MarkDelayPublished(ctx context.Context, caseID string, at time.Time) error {
	const q = `
UPDATE sla_delay
SET published_at = $2
WHERE case_id = $1 AND published_at IS NULL`
	if _, err := s.pool.Exec(ctx, q, caseID, at); err != nil {
		return fmt.Errorf("cases pgx: mark delay: %w", err)
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
		return nil, fmt.Errorf("cases pgx: list unpublished: %w", err)
	}
	defer rows.Close()

	out := make([]OutboxRow, 0)
	for rows.Next() {
		var row OutboxRow
		if err := rows.Scan(&row.EventID, &row.RoutingKey, &row.Payload); err != nil {
			return nil, fmt.Errorf("cases pgx: scan: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cases pgx: rows: %w", err)
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
		return fmt.Errorf("cases pgx: mark published: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("cases pgx: mark published: event_id %s not found or already published", eventID)
	}
	return nil
}
