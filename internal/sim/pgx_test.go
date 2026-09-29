package sim_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	accountv1 "github.com/leohteixeira/advisor-radar/gen/account/v1"
	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/outbox"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

// testDatabaseEnv names a PostgreSQL DSN for the integration tests. Unset, the
// tests skip. Each test migrates its own throwaway schema and drops it after.
const testDatabaseEnv = "ACCOUNT_SIM_TEST_DATABASE_URL"

// newTestPool returns a pool whose search_path is a fresh schema migrated with
// migrations/account_sim and seeded with the POV accounts through Reseed.
func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := newMigratedPool(t)
	if err := sim.Reseed(t.Context(), sim.NewPGXStore(pool)); err != nil {
		t.Fatalf("reseed: %v", err)
	}
	return pool
}

// newMigratedPool returns a pool whose search_path is a fresh schema migrated
// with migrations/account_sim and holding no seed rows.
func newMigratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv(testDatabaseEnv)
	if dsn == "" {
		t.Skipf("%s is not set", testDatabaseEnv)
	}
	ctx := t.Context()

	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatalf("random schema suffix: %v", err)
	}
	schema := pgx.Identifier{"sim_test_" + hex.EncodeToString(suffix)}.Sanitize()

	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(admin.Close)
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop schema: %v", err)
		}
	})

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.MaxConns = 8
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect schema pool: %v", err)
	}
	// Registered after the drop, so it runs first and releases connections.
	t.Cleanup(pool.Close)

	applySQLDir(t, pool, filepath.Join("..", "..", "migrations", "account_sim"))
	return pool
}

// applySQLDir runs every .sql file of dir in name order, as cmd/db does.
func applySQLDir(t *testing.T, pool *pgxpool.Pool, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	slices.Sort(names)
	for _, name := range names {
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if _, err := pool.Exec(t.Context(), string(body)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
}

func countRows(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func dbCaixa(t *testing.T, pool *pgxpool.Pool, customerID string) int64 {
	t.Helper()
	var caixa int64
	if err := pool.QueryRow(t.Context(),
		`SELECT caixa FROM pov_account WHERE customer_id = $1`, customerID,
	).Scan(&caixa); err != nil {
		t.Fatalf("read caixa: %v", err)
	}
	return caixa
}

type recordingBroker struct {
	mu   sync.Mutex
	keys []string
}

func (b *recordingBroker) Publish(_ context.Context, routingKey string, _ []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.keys = append(b.keys, routingKey)
	return nil
}

func TestPGXStore_DepositReplayAndRelay(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	ctx := t.Context()
	store := sim.NewPGXStore(pool)
	before := dbCaixa(t, pool, sim.CustomerFernanda)
	cmd := sim.Command{
		CustomerID: sim.CustomerFernanda, IdempotencyKey: "dep-1",
		Kind: sim.CmdDeposit, Amount: 1_000_000, Origin: "Conta corrente",
	}

	first, err := sim.Apply(ctx, store, cmd)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if first.Replay {
		t.Fatal("first apply is a replay")
	}
	if got := dbCaixa(t, pool, sim.CustomerFernanda); got != before+1_000_000 {
		t.Fatalf("caixa = %d, want %d", got, before+1_000_000)
	}
	var eventID, routing string
	var schemaVersion int
	if err := pool.QueryRow(ctx,
		`SELECT event_id::text, routing_key, (payload->>'schema_version')::int FROM outbox`,
	).Scan(&eventID, &routing, &schemaVersion); err != nil {
		t.Fatalf("read outbox: %v", err)
	}
	if eventID != first.EventID || routing != event.NameAccountEventRecorded || schemaVersion != event.SchemaVersionCents {
		t.Fatalf("outbox = %s %s v%d, want %s", eventID, routing, schemaVersion, first.EventID)
	}

	second, err := sim.Apply(ctx, store, cmd)
	if err != nil {
		t.Fatalf("replay Apply: %v", err)
	}
	if !second.Replay || second.EventID != first.EventID {
		t.Fatalf("replay = %+v, want %s with replay", second, first.EventID)
	}
	if got := countRows(t, pool, "outbox"); got != 1 {
		t.Fatalf("outbox rows = %d, want 1", got)
	}
	if got := dbCaixa(t, pool, sim.CustomerFernanda); got != before+1_000_000 {
		t.Fatalf("caixa after replay = %d, want %d", got, before+1_000_000)
	}

	// The existing relay publishes the committed row and marks it.
	broker := &recordingBroker{}
	if err := outbox.Publish(ctx, outbox.NewPGXStore(pool), broker); err != nil {
		t.Fatalf("relay: %v", err)
	}
	if !slices.Equal(broker.keys, []string{event.NameAccountEventRecorded}) {
		t.Fatalf("published = %v", broker.keys)
	}
}

func TestPGXStore_OverCashRollsBack(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	store := sim.NewPGXStore(pool)
	caixa := dbCaixa(t, pool, sim.CustomerFernanda)

	_, err := sim.Apply(t.Context(), store, sim.Command{
		CustomerID: sim.CustomerFernanda, IdempotencyKey: "wd-over",
		Kind: sim.CmdWithdrawal, Amount: caixa + 1, Destination: "Conta corrente",
	})
	if !errors.Is(err, sim.ErrInsufficient) {
		t.Fatalf("err = %v, want ErrInsufficient", err)
	}
	if got := dbCaixa(t, pool, sim.CustomerFernanda); got != caixa {
		t.Fatalf("caixa = %d, want %d", got, caixa)
	}
	if got := countRows(t, pool, "outbox"); got != 0 {
		t.Fatalf("outbox rows = %d, want 0", got)
	}
	if got := countRows(t, pool, "pov_idempotency"); got != 0 {
		t.Fatalf("keys = %d, want 0", got)
	}
}

// slowLookupStore pauses after LookupKey so concurrent callers overlap inside
// their transactions. Without per-customer serialization every caller would
// miss the key and the pov_idempotency primary key would reject the others.
type slowLookupStore struct {
	inner *sim.PGXStore
}

func (s slowLookupStore) WithTx(ctx context.Context, fn func(sim.Tx) error) error {
	return s.inner.WithTx(ctx, func(tx sim.Tx) error { return fn(slowLookupTx{Tx: tx}) })
}

type slowLookupTx struct {
	sim.Tx
}

func (t slowLookupTx) LookupKey(ctx context.Context, customerID, key string) (string, bool, error) {
	eventID, ok, err := t.Tx.LookupKey(ctx, customerID, key)
	if err != nil {
		return "", false, err
	}
	select {
	case <-time.After(100 * time.Millisecond):
	case <-ctx.Done():
		return "", false, ctx.Err()
	}
	return eventID, ok, nil
}

func TestPGXStore_ConcurrentSameKey(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	store := slowLookupStore{inner: sim.NewPGXStore(pool)}
	caixa := dbCaixa(t, pool, sim.CustomerThiago)
	cmd := sim.Command{
		CustomerID: sim.CustomerThiago, IdempotencyKey: "wd-race",
		Kind: sim.CmdWithdrawal, Amount: 10_000, Destination: "Conta corrente",
	}

	const callers = 4
	results := make([]sim.Result, callers)
	errs := make([]error, callers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range callers {
		wg.Go(func() {
			<-start
			results[i], errs[i] = sim.Apply(t.Context(), store, cmd)
		})
	}
	close(start)
	wg.Wait()

	fresh := 0
	for i := range callers {
		if errs[i] != nil {
			t.Fatalf("Apply %d: %v", i, errs[i])
		}
		if results[i].EventID != results[0].EventID {
			t.Fatalf("event ids differ: %s vs %s", results[i].EventID, results[0].EventID)
		}
		if !results[i].Replay {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatalf("fresh applies = %d, want 1", fresh)
	}
	if got := countRows(t, pool, "outbox"); got != 1 {
		t.Fatalf("outbox rows = %d, want 1", got)
	}
	if got := dbCaixa(t, pool, sim.CustomerThiago); got != caixa-10_000 {
		t.Fatalf("caixa = %d, want %d", got, caixa-10_000)
	}
}

// badOutboxStore forwards to PGXStore but stages an outbox row PostgreSQL
// rejects, so the transaction fails after the account row was updated.
type badOutboxStore struct {
	inner *sim.PGXStore
}

func (s badOutboxStore) WithTx(ctx context.Context, fn func(sim.Tx) error) error {
	return s.inner.WithTx(ctx, func(tx sim.Tx) error { return fn(badOutboxTx{Tx: tx}) })
}

type badOutboxTx struct {
	sim.Tx
}

func (t badOutboxTx) InsertOutbox(ctx context.Context, row outbox.Row) error {
	row.EventID = "not-a-uuid"
	return t.Tx.InsertOutbox(ctx, row)
}

func TestPGXStore_FailureMidTransaction(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	client := startAccountServer(t, badOutboxStore{inner: sim.NewPGXStore(pool)})
	caixa := dbCaixa(t, pool, sim.CustomerFernanda)

	_, err := client.Deposit(t.Context(), &accountv1.DepositRequest{
		CustomerId: sim.CustomerFernanda, IdempotencyKey: "dep-fail", AmountCents: 1_000_000, Origin: "Conta corrente",
	})
	wantCode(t, err, codes.Internal)
	msg := status.Convert(err).Message()
	for _, leak := range []string{"INSERT", "outbox", "uuid", "SQLSTATE", "postgres"} {
		if strings.Contains(msg, leak) {
			t.Fatalf("status message %q leaks %q", msg, leak)
		}
	}
	if got := dbCaixa(t, pool, sim.CustomerFernanda); got != caixa {
		t.Fatalf("caixa = %d, want %d after rollback", got, caixa)
	}
	if got := countRows(t, pool, "outbox"); got != 0 {
		t.Fatalf("outbox rows = %d, want 0", got)
	}
	if got := countRows(t, pool, "pov_idempotency"); got != 0 {
		t.Fatalf("keys = %d, want 0", got)
	}
}

func TestPGXStore_GRPCQueries(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	client := startAccountServer(t, sim.NewPGXStore(pool))
	want := seeded(t, sim.CustomerMariana)

	got, err := client.GetAccount(t.Context(), &accountv1.GetAccountRequest{CustomerId: sim.CustomerMariana})
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if got.GetAcoesCents() != want.Acoes || got.GetEtfsCents() != want.ETFs ||
		got.GetRendaFixaCents() != want.RendaFixa || got.GetCaixaCents() != want.Caixa {
		t.Fatalf("GetAccount = %+v, want %+v", got, want)
	}
	list, err := client.ListAccounts(t.Context(), &accountv1.ListAccountsRequest{})
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	if len(list.GetAccounts()) != 3 {
		t.Fatalf("accounts = %d, want 3", len(list.GetAccounts()))
	}
}

// TestPGXStore_SQLSeedPositions runs the account_sim migrations and the SQL
// seed that `cmd/db seed` applies, twice to prove reseed is idempotent, then
// checks the seed matrix rows over gRPC, the cash-only commands, and that the
// Go Reseed lands on the same state.
func TestPGXStore_SQLSeedPositions(t *testing.T) {
	t.Parallel()
	pool := newMigratedPool(t)
	seeds := filepath.Join("..", "..", "seeds", "account_sim")
	applySQLDir(t, pool, seeds)
	applySQLDir(t, pool, seeds)
	store := sim.NewPGXStore(pool)
	client := startAccountServer(t, store)
	assertSeedState(t, client)

	assertCashOnlyCommands(t, client, func(eventID string) []byte {
		t.Helper()
		var payload []byte
		if err := pool.QueryRow(t.Context(),
			`SELECT payload FROM outbox WHERE event_id = $1::uuid`, eventID,
		).Scan(&payload); err != nil {
			t.Fatalf("read outbox %s: %v", eventID, err)
		}
		return payload
	})

	// A position bought after the seed is dropped by reseed.
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO pov_position (customer_id, product_id, units_cents, applied_cents) VALUES ($1, 'tbill', 5000, 5000)`,
		sim.CustomerThiago,
	); err != nil {
		t.Fatalf("insert extra position: %v", err)
	}
	applySQLDir(t, pool, seeds)
	assertSeedState(t, client)
	if got := countRows(t, pool, "pov_position"); got != 11 {
		t.Fatalf("positions after SQL reseed = %d, want 11", got)
	}

	if _, err := pool.Exec(t.Context(),
		`INSERT INTO pov_position (customer_id, product_id, units_cents, applied_cents) VALUES ($1, 'tbill', 5000, 5000)`,
		sim.CustomerThiago,
	); err != nil {
		t.Fatalf("insert extra position: %v", err)
	}
	if err := sim.Reseed(t.Context(), store); err != nil {
		t.Fatalf("Reseed: %v", err)
	}
	assertSeedState(t, client)
	if got := countRows(t, pool, "pov_position"); got != 11 {
		t.Fatalf("positions after Go reseed = %d, want 11", got)
	}
}

// TestPGXStore_Purchase runs the purchase matrix over PostgreSQL, loaded from
// the SQL seed as `cmd/db seed` does, then checks that the Go reseed drops
// the bought positions and restores the cash.
func TestPGXStore_Purchase(t *testing.T) {
	t.Parallel()
	pool := newMigratedPool(t)
	applySQLDir(t, pool, filepath.Join("..", "..", "seeds", "account_sim"))
	store := sim.NewPGXStore(pool)
	client := startAccountServer(t, store)

	assertPurchases(t, client, func(eventID string) []byte {
		t.Helper()
		var payload []byte
		if err := pool.QueryRow(t.Context(),
			`SELECT payload FROM outbox WHERE event_id = $1::uuid`, eventID,
		).Scan(&payload); err != nil {
			t.Fatalf("read outbox %s: %v", eventID, err)
		}
		return payload
	}, func() int { return countRows(t, pool, "outbox") })

	var units, applied int64
	if err := pool.QueryRow(t.Context(),
		`SELECT units_cents, applied_cents FROM pov_position WHERE customer_id = $1 AND product_id = 'acoesg'`,
		sim.CustomerThiago,
	).Scan(&units, &applied); err != nil {
		t.Fatalf("read acoesg position: %v", err)
	}
	if units != 3_544_000 || applied != 3_520_000 {
		t.Fatalf("acoesg units %d applied %d, want 3544000 and 3520000", units, applied)
	}

	if err := sim.Reseed(t.Context(), store); err != nil {
		t.Fatalf("Reseed: %v", err)
	}
	assertSeedState(t, client)
	if got := countRows(t, pool, "pov_position"); got != 11 {
		t.Fatalf("positions after reseed = %d, want 11", got)
	}
}

// TestPGXStore_ConcurrentPurchases sends two purchases with different keys
// that together exceed the cash. The customer lock serializes them, so one
// commits and the other sees the lower cash and is refused.
func TestPGXStore_ConcurrentPurchases(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	store := sim.NewPGXStore(pool)
	caixa := dbCaixa(t, pool, sim.CustomerThiago)
	amount := caixa/2 + 1

	const callers = 2
	errs := make([]error, callers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range callers {
		wg.Go(func() {
			<-start
			_, errs[i] = sim.Apply(t.Context(), store, sim.Command{
				CustomerID: sim.CustomerThiago, IdempotencyKey: fmt.Sprintf("buy-race-%d", i),
				Kind: sim.CmdPurchase, ProductID: "cobalto", Amount: amount,
			})
		})
	}
	close(start)
	wg.Wait()

	ok, refused := 0, 0
	for i := range callers {
		switch {
		case errs[i] == nil:
			ok++
		case errors.Is(errs[i], sim.ErrInsufficient):
			refused++
		default:
			t.Fatalf("Apply %d: %v", i, errs[i])
		}
	}
	if ok != 1 || refused != 1 {
		t.Fatalf("accepted %d refused %d, want 1 and 1", ok, refused)
	}
	if got := dbCaixa(t, pool, sim.CustomerThiago); got != caixa-amount {
		t.Fatalf("caixa = %d, want %d", got, caixa-amount)
	}
	if got := countRows(t, pool, "outbox"); got != 1 {
		t.Fatalf("outbox rows = %d, want 1", got)
	}
}
