package sim

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/leohteixeira/advisor-radar/internal/outbox"
)

// PGXStore is the PostgreSQL Store over pov_account, pov_position,
// pov_product, pov_registration, pov_preferences, pov_idempotency, and outbox
// in the account_sim database. Every writer takes a transaction-scoped advisory lock
// per customer, so commands for one customer run one at a time and a
// concurrent request with the same key becomes a replay.
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

// GetAccount reads the cash and values the positions at the current day. It
// takes the customer lock first, so cash and positions come from one state
// that no command is changing.
func (t *pgxTx) GetAccount(ctx context.Context, customerID string) (Account, bool, error) {
	if err := t.lock(ctx, customerID); err != nil {
		return Account{}, false, err
	}
	const q = `
SELECT caixa
FROM pov_account
WHERE customer_id = $1`
	var cash int64
	err := t.tx.QueryRow(ctx, q, customerID).Scan(&cash)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, false, nil
	}
	if err != nil {
		return Account{}, false, fmt.Errorf("sim pgx: get account: %w", err)
	}
	positions, err := t.listPositions(ctx, customerID)
	if err != nil {
		return Account{}, false, err
	}
	account, err := aggregate(customerID, cash, positions)
	if err != nil {
		return Account{}, false, err
	}
	return account, true, nil
}

// PutAccount writes only the cash; the class fields of account are
// aggregates and positions are never written from them.
func (t *pgxTx) PutAccount(ctx context.Context, account Account) error {
	const q = `
UPDATE pov_account
SET caixa = $2
WHERE customer_id = $1`
	tag, err := t.tx.Exec(ctx, q, account.CustomerID, account.Caixa)
	if err != nil {
		return fmt.Errorf("sim pgx: put account: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("sim pgx: put account: %d rows updated, want 1", tag.RowsAffected())
	}
	return nil
}

// AddPosition adds delta's units and applied cents to the customer's
// position in delta.ProductID, inserting the row when absent. The customer
// lock is already held: Apply reads the key and the account first.
func (t *pgxTx) AddPosition(ctx context.Context, customerID string, delta Position) error {
	const q = `
INSERT INTO pov_position (customer_id, product_id, units_cents, applied_cents)
VALUES ($1, $2, $3, $4)
ON CONFLICT (customer_id, product_id) DO UPDATE SET
  units_cents = pov_position.units_cents + EXCLUDED.units_cents,
  applied_cents = pov_position.applied_cents + EXCLUDED.applied_cents`
	if _, err := t.tx.Exec(ctx, q, customerID, delta.ProductID, delta.UnitsCents, delta.AppliedCents); err != nil {
		return fmt.Errorf("sim pgx: add position: %w", err)
	}
	return nil
}

// listPositions returns the customer's positions valued at the current day.
func (t *pgxTx) listPositions(ctx context.Context, customerID string) ([]Position, error) {
	const q = `
SELECT p.product_id, pr.asset_class, p.units_cents, p.applied_cents
FROM pov_position p
JOIN pov_product pr ON pr.id = p.product_id
WHERE p.customer_id = $1`
	rows, err := t.tx.Query(ctx, q, customerID)
	if err != nil {
		return nil, fmt.Errorf("sim pgx: list positions: %w", err)
	}
	positions, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Position, error) {
		var position Position
		err := row.Scan(&position.ProductID, &position.AssetClass, &position.UnitsCents, &position.AppliedCents)
		return position, err
	})
	if err != nil {
		return nil, fmt.Errorf("sim pgx: scan positions: %w", err)
	}
	return valuePositions(positions, SeedDay), nil
}

// ListProducts returns the catalog ordered by risk and then id.
func (t *pgxTx) ListProducts(ctx context.Context) ([]Product, error) {
	const q = `
SELECT id, name, asset_class, risk, return_label, minimum_cents
FROM pov_product
ORDER BY risk, id`
	rows, err := t.tx.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("sim pgx: list products: %w", err)
	}
	products, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Product, error) {
		var (
			product Product
			risk    int16
		)
		err := row.Scan(&product.ID, &product.Name, &product.AssetClass, &risk, &product.ReturnLabel, &product.MinimumCents)
		product.Risk = int(risk)
		return product, err
	})
	if err != nil {
		return nil, fmt.Errorf("sim pgx: scan products: %w", err)
	}
	return products, nil
}

func (t *pgxTx) GetRegistration(ctx context.Context, customerID string) (Registration, bool, error) {
	const q = `
SELECT email, phone, city, account_number
FROM pov_registration
WHERE customer_id = $1`
	registration := Registration{CustomerID: customerID}
	err := t.tx.QueryRow(ctx, q, customerID).Scan(
		&registration.Email, &registration.Phone, &registration.City, &registration.AccountNumber,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Registration{}, false, nil
	}
	if err != nil {
		return Registration{}, false, fmt.Errorf("sim pgx: get registration: %w", err)
	}
	return registration, true, nil
}

// GetPreferences takes the customer lock first, so it serializes with the
// customer's commands and a concurrent update, then reads the stored row. A
// known customer without a row answers DefaultPreferences.
func (t *pgxTx) GetPreferences(ctx context.Context, customerID string) (Preferences, bool, error) {
	if err := t.lock(ctx, customerID); err != nil {
		return Preferences{}, false, err
	}
	const q = `
SELECT p.channel, p.beta
FROM pov_account a
LEFT JOIN pov_preferences p ON p.customer_id = a.customer_id
WHERE a.customer_id = $1`
	var (
		channel *string
		beta    *bool
	)
	err := t.tx.QueryRow(ctx, q, customerID).Scan(&channel, &beta)
	if errors.Is(err, pgx.ErrNoRows) {
		return Preferences{}, false, nil
	}
	if err != nil {
		return Preferences{}, false, fmt.Errorf("sim pgx: get preferences: %w", err)
	}
	if channel == nil || beta == nil {
		return DefaultPreferences(), true, nil
	}
	return Preferences{Channel: *channel, Beta: *beta}, true, nil
}

// PutPreferences upserts the customer's preferences. The customer lock is
// already held: UpdatePreferences reads the preferences first.
func (t *pgxTx) PutPreferences(ctx context.Context, customerID string, prefs Preferences) error {
	const q = `
INSERT INTO pov_preferences (customer_id, channel, beta)
VALUES ($1, $2, $3)
ON CONFLICT (customer_id) DO UPDATE SET
  channel = EXCLUDED.channel,
  beta = EXCLUDED.beta`
	if _, err := t.tx.Exec(ctx, q, customerID, prefs.Channel, prefs.Beta); err != nil {
		return fmt.Errorf("sim pgx: put preferences: %w", err)
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

// ResetPOV upserts the catalog, then restores each seeded account: cash,
// exactly the seeded positions, registration, and preferences. It sorts the accounts by
// customer_id (byte-wise string order, not by lock hash) and takes each
// customer lock in that order, so it serializes with Apply and two resets
// acquire their locks in the same order and cannot deadlock.
func (t *pgxTx) ResetPOV(ctx context.Context, seed Seed) error {
	const upsertProduct = `
INSERT INTO pov_product (id, name, asset_class, risk, return_label, minimum_cents)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (id) DO UPDATE SET
  name = EXCLUDED.name,
  asset_class = EXCLUDED.asset_class,
  risk = EXCLUDED.risk,
  return_label = EXCLUDED.return_label,
  minimum_cents = EXCLUDED.minimum_cents`
	for _, product := range seed.Products {
		if _, err := t.tx.Exec(ctx, upsertProduct,
			product.ID, product.Name, product.AssetClass, product.Risk, product.ReturnLabel, product.MinimumCents,
		); err != nil {
			return fmt.Errorf("sim pgx: reset product: %w", err)
		}
	}

	accounts := slices.Clone(seed.Accounts)
	slices.SortFunc(accounts, func(a, b SeedAccount) int {
		return strings.Compare(a.CustomerID, b.CustomerID)
	})
	for _, account := range accounts {
		if err := t.lock(ctx, account.CustomerID); err != nil {
			return err
		}
		if err := t.resetAccount(ctx, account); err != nil {
			return err
		}
	}
	return nil
}

func (t *pgxTx) resetAccount(ctx context.Context, account SeedAccount) error {
	const upsertAccount = `
INSERT INTO pov_account (customer_id, caixa)
VALUES ($1, $2)
ON CONFLICT (customer_id) DO UPDATE SET caixa = EXCLUDED.caixa`
	if _, err := t.tx.Exec(ctx, upsertAccount, account.CustomerID, account.CashCents); err != nil {
		return fmt.Errorf("sim pgx: reset account: %w", err)
	}
	if _, err := t.tx.Exec(ctx,
		`DELETE FROM pov_position WHERE customer_id = $1`, account.CustomerID,
	); err != nil {
		return fmt.Errorf("sim pgx: clear positions: %w", err)
	}
	const insertPosition = `
INSERT INTO pov_position (customer_id, product_id, units_cents, applied_cents)
VALUES ($1, $2, $3, $4)`
	for _, position := range account.Positions {
		if _, err := t.tx.Exec(ctx, insertPosition,
			account.CustomerID, position.ProductID, position.UnitsCents, position.AppliedCents,
		); err != nil {
			return fmt.Errorf("sim pgx: reset position: %w", err)
		}
	}
	const upsertRegistration = `
INSERT INTO pov_registration (customer_id, email, phone, city, account_number)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (customer_id) DO UPDATE SET
  email = EXCLUDED.email,
  phone = EXCLUDED.phone,
  city = EXCLUDED.city,
  account_number = EXCLUDED.account_number`
	registration := account.Registration
	if _, err := t.tx.Exec(ctx, upsertRegistration,
		account.CustomerID, registration.Email, registration.Phone, registration.City, registration.AccountNumber,
	); err != nil {
		return fmt.Errorf("sim pgx: reset registration: %w", err)
	}
	if err := t.PutPreferences(ctx, account.CustomerID, seedPreferences(account.Preferences)); err != nil {
		return fmt.Errorf("sim pgx: reset preferences: %w", err)
	}
	return nil
}
