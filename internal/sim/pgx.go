package sim

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/leohteixeira/advisor-radar/internal/outbox"
)

// PGXStore is the PostgreSQL Store over pov_account, pov_idempotency, and
// outbox in the account_sim database. Every writer takes a transaction-scoped
// advisory lock per customer, so commands for one customer run one at a time
// and a concurrent request with the same key becomes a replay.
type PGXStore struct {
	pool *pgxpool.Pool
}

var _ Store = (*PGXStore)(nil)

// NewPGXStore wraps a pgx pool as a Store.
func NewPGXStore(pool *pgxpool.Pool) *PGXStore {
	return &PGXStore{pool: pool}
}

// WithTx runs fn inside one database transaction. An error from fn, or from
// commit, rolls everything back; errors from fn are returned unwrapped so
// callers can match the sim sentinels.
func (s *PGXStore) WithTx(ctx context.Context, fn func(Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("sim pgx: begin: %w", err)
	}
	// Rollback after a successful commit is a no-op.
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	if err := fn(&pgxTx{tx: tx}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("sim pgx: commit: %w", err)
	}
	return nil
}

type pgxTx struct {
	tx pgx.Tx
}

var _ Tx = (*pgxTx)(nil)

// customerLockSpace is the first key of the two-key advisory lock, so the
// per-customer locks cannot collide with single-key locks or other spaces.
const customerLockSpace int32 = 0x504F5641 // "POVA"

// lock serializes writers for one customer until the transaction ends.
func (t *pgxTx) lock(ctx context.Context, customerID string) error {
	if _, err := t.tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock($1::int4, hashtext($2))`, customerLockSpace, customerID,
	); err != nil {
		return fmt.Errorf("sim pgx: lock customer: %w", err)
	}
	return nil
}

func (t *pgxTx) GetAccount(ctx context.Context, customerID string) (Account, bool, error) {
	const q = `
SELECT acoes, etfs, renda_fixa, caixa
FROM pov_account
WHERE customer_id = $1`
	account := Account{CustomerID: customerID}
	err := t.tx.QueryRow(ctx, q, customerID).Scan(
		&account.Acoes, &account.ETFs, &account.RendaFixa, &account.Caixa,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, false, nil
	}
	if err != nil {
		return Account{}, false, fmt.Errorf("sim pgx: get account: %w", err)
	}
	return account, true, nil
}

func (t *pgxTx) PutAccount(ctx context.Context, account Account) error {
	const q = `
UPDATE pov_account
SET acoes = $2, etfs = $3, renda_fixa = $4, caixa = $5
WHERE customer_id = $1`
	tag, err := t.tx.Exec(ctx, q,
		account.CustomerID, account.Acoes, account.ETFs, account.RendaFixa, account.Caixa,
	)
	if err != nil {
		return fmt.Errorf("sim pgx: put account: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("sim pgx: put account: %d rows updated, want 1", tag.RowsAffected())
	}
	return nil
}

// LookupKey runs first in Apply, so it takes the customer lock before reading
// the key: a concurrent same-key request waits here and then sees the key.
func (t *pgxTx) LookupKey(ctx context.Context, customerID, key string) (string, bool, error) {
	if err := t.lock(ctx, customerID); err != nil {
		return "", false, err
	}
	const q = `
SELECT event_id
FROM pov_idempotency
WHERE customer_id = $1 AND idem_key = $2`
	var eventID string
	err := t.tx.QueryRow(ctx, q, customerID, key).Scan(&eventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("sim pgx: lookup key: %w", err)
	}
	return eventID, true, nil
}

func (t *pgxTx) SaveKey(ctx context.Context, customerID, key, eventID string) error {
	const q = `
INSERT INTO pov_idempotency (customer_id, idem_key, event_id)
VALUES ($1, $2, $3)`
	if _, err := t.tx.Exec(ctx, q, customerID, key, eventID); err != nil {
		return fmt.Errorf("sim pgx: save key: %w", err)
	}
	return nil
}

// InsertOutbox stages one event. A duplicate event_id fails the transaction
// rather than silently dropping the event.
func (t *pgxTx) InsertOutbox(ctx context.Context, row outbox.Row) error {
	const q = `
INSERT INTO outbox (event_id, routing_key, payload)
VALUES ($1::uuid, $2, $3::jsonb)`
	if _, err := t.tx.Exec(ctx, q, row.EventID, row.RoutingKey, row.Payload); err != nil {
		return fmt.Errorf("sim pgx: insert outbox: %w", err)
	}
	return nil
}

// ResetPOV upserts the given balances. It sorts the accounts by customer_id
// (byte-wise string order, not by lock hash) and takes each customer lock in
// that order, so it serializes with Apply and two resets acquire their locks
// in the same order and cannot deadlock.
func (t *pgxTx) ResetPOV(ctx context.Context, accounts []Account) error {
	sorted := slices.Clone(accounts)
	slices.SortFunc(sorted, func(a, b Account) int {
		switch {
		case a.CustomerID < b.CustomerID:
			return -1
		case a.CustomerID > b.CustomerID:
			return 1
		default:
			return 0
		}
	})
	const q = `
INSERT INTO pov_account (customer_id, acoes, etfs, renda_fixa, caixa)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (customer_id) DO UPDATE SET
  acoes = EXCLUDED.acoes,
  etfs = EXCLUDED.etfs,
  renda_fixa = EXCLUDED.renda_fixa,
  caixa = EXCLUDED.caixa`
	for _, account := range sorted {
		if err := t.lock(ctx, account.CustomerID); err != nil {
			return err
		}
		if _, err := t.tx.Exec(ctx, q,
			account.CustomerID, account.Acoes, account.ETFs, account.RendaFixa, account.Caixa,
		); err != nil {
			return fmt.Errorf("sim pgx: reset account: %w", err)
		}
	}
	return nil
}
