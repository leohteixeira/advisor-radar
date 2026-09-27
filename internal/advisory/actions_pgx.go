package advisory

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Upsert writes or replaces one signal_action row.
func (s *PGXStore) Upsert(ctx context.Context, action SignalAction) error {
	const q = `
INSERT INTO signal_action (signal_id, contacted_at, snoozed_until)
VALUES ($1, $2, $3)
ON CONFLICT (signal_id) DO UPDATE SET
    contacted_at = EXCLUDED.contacted_at,
    snoozed_until = EXCLUDED.snoozed_until`
	_, err := s.pool.Exec(ctx, q, action.SignalID, action.ContactedAt, action.SnoozedUntil)
	if err != nil {
		return fmt.Errorf("advisory pgx: upsert action: %w", err)
	}
	return nil
}

// Delete removes one signal_action row. Missing ids are a no-op.
func (s *PGXStore) Delete(ctx context.Context, signalID string) error {
	const q = `DELETE FROM signal_action WHERE signal_id = $1`
	_, err := s.pool.Exec(ctx, q, signalID)
	if err != nil {
		return fmt.Errorf("advisory pgx: delete action: %w", err)
	}
	return nil
}

// List returns every signal_action row.
func (s *PGXStore) List(ctx context.Context) ([]SignalAction, error) {
	const q = `
SELECT signal_id, contacted_at, snoozed_until
FROM signal_action
ORDER BY signal_id`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("advisory pgx: list actions: %w", err)
	}
	defer rows.Close()

	out := make([]SignalAction, 0)
	for rows.Next() {
		var row SignalAction
		var contacted, snoozed *time.Time
		if err := rows.Scan(&row.SignalID, &contacted, &snoozed); err != nil {
			return nil, fmt.Errorf("advisory pgx: scan action: %w", err)
		}
		row.ContactedAt = contacted
		row.SnoozedUntil = snoozed
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("advisory pgx: action rows: %w", err)
	}
	return out, nil
}

// Get returns one signal_action row when present.
func (s *PGXStore) Get(ctx context.Context, signalID string) (SignalAction, bool, error) {
	const q = `
SELECT signal_id, contacted_at, snoozed_until
FROM signal_action
WHERE signal_id = $1`
	var row SignalAction
	var contacted, snoozed *time.Time
	err := s.pool.QueryRow(ctx, q, signalID).Scan(&row.SignalID, &contacted, &snoozed)
	if errors.Is(err, pgx.ErrNoRows) {
		return SignalAction{}, false, nil
	}
	if err != nil {
		return SignalAction{}, false, fmt.Errorf("advisory pgx: get action: %w", err)
	}
	row.ContactedAt = contacted
	row.SnoozedUntil = snoozed
	return row, true, nil
}
