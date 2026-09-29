package cases_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/leohteixeira/advisor-radar/internal/book"
	"github.com/leohteixeira/advisor-radar/internal/cases"
	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/identity"
)

// casesTestDatabaseEnv names a PostgreSQL DSN for the gated pgx tests. Unset,
// they skip. Each test migrates its own throwaway schema and drops it after.
const casesTestDatabaseEnv = "CASES_TEST_DATABASE_URL"

// newCasesPool returns a pool whose search_path is a fresh schema migrated
// with migrations/cases.
func newCasesPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv(casesTestDatabaseEnv)
	if dsn == "" {
		t.Skipf("%s is not set", casesTestDatabaseEnv)
	}
	ctx := t.Context()

	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatalf("random schema suffix: %v", err)
	}
	schema := pgx.Identifier{"cases_test_" + hex.EncodeToString(suffix)}.Sanitize()

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
	cfg.MaxConns = 12
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect schema pool: %v", err)
	}
	// Registered after the drop, so it runs first and releases connections.
	t.Cleanup(pool.Close)

	applyCasesSQLDir(t, pool, filepath.Join("..", "..", "migrations", "cases"))
	return pool
}

// applyCasesSQLDir runs every .sql file of dir in name order, as cmd/db does.
func applyCasesSQLDir(t *testing.T, pool *pgxpool.Pool, dir string) {
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

func countWhere(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", query, err)
	}
	return n
}

func complaintBody(t *testing.T, customerID string) []byte {
	t.Helper()
	return triagedBody(t, identity.MustNewV7(), customerID,
		cases.TriagedMessage{Origin: cases.OriginClientApp, SourceEventID: identity.MustNewV7(), Intent: "reclamacao", ChurnRisk: 0.8})
}

func TestPGXStore_IntakeLifecycle(t *testing.T) {
	t.Parallel()
	pool := newCasesPool(t)
	store := cases.NewPGXStore(pool)
	ctx := t.Context()
	customerID := identity.MustNewV7()
	advisorID := identity.MustNewV7()
	lookup := singular(advisorID)

	// Open.
	firstBody := complaintBody(t, customerID)
	first, err := cases.Intake(ctx, store, lookup, firstBody)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if first.Decision != cases.DecisionOpened {
		t.Fatalf("open decision = %q", first.Decision)
	}
	row, ok, err := store.GetCase(ctx, first.CaseID)
	if err != nil || !ok {
		t.Fatalf("GetCase = %v, %v", ok, err)
	}
	if row.State != cases.StateAberto || row.AdvisorID != advisorID || row.SLATotalMinutes != 30 {
		t.Fatalf("opened case = %+v, want Aberto, advisor, 30 min", row)
	}
	unpublished, err := store.ListUnpublished(ctx)
	if err != nil {
		t.Fatalf("ListUnpublished: %v", err)
	}
	if len(unpublished) != 1 || unpublished[0].RoutingKey != event.NameCaseOpened {
		t.Fatalf("outbox = %d rows, want one case.opened", len(unpublished))
	}
	if _, err := uuid.Parse(unpublished[0].EventID); err != nil {
		t.Fatalf("case.opened event id %q is not a UUID", unpublished[0].EventID)
	}
	delays, err := store.ListUnpublishedDelays(ctx)
	if err != nil {
		t.Fatalf("ListUnpublishedDelays: %v", err)
	}
	if len(delays) != 1 || delays[0].CaseID != first.CaseID || delays[0].TTLMs != 30*60*1000 {
		t.Fatalf("delays = %+v, want one 30 min arm", delays)
	}
	if n := countWhere(t, pool,
		`SELECT count(*) FROM case_history WHERE case_id = $1 AND kind = 'caso' AND text = $2`,
		first.CaseID, "Caso aberto a partir de mensagem com reclamação"); n != 1 {
		t.Fatalf("caso history rows = %d, want 1", n)
	}

	// Duplicate delivery.
	dup, err := cases.Intake(ctx, store, lookup, firstBody)
	if err != nil || dup.Decision != cases.DecisionDuplicate {
		t.Fatalf("duplicate = %+v, %v", dup, err)
	}

	// Join while the case is in Em atendimento.
	if err := cases.Advance(ctx, store, first.CaseID, cases.StateEmAtendimento); err != nil {
		t.Fatalf("advance: %v", err)
	}
	joined, err := cases.Intake(ctx, store, lookup, complaintBody(t, customerID))
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	if joined.Decision != cases.DecisionJoined || joined.CaseID != first.CaseID {
		t.Fatalf("join = %+v, want joined %s", joined, first.CaseID)
	}
	if n := countWhere(t, pool,
		`SELECT count(*) FROM case_history WHERE case_id = $1 AND kind = 'mensagem' AND text = $2`,
		first.CaseID, "Nova mensagem do cliente: reclamação"); n != 1 {
		t.Fatalf("mensagem history rows = %d, want 1", n)
	}
	if n := countWhere(t, pool, `SELECT count(*) FROM outbox WHERE routing_key = $1`, event.NameCaseOpened); n != 1 {
		t.Fatalf("case.opened rows = %d, want 1 after join", n)
	}

	// Resolve, then a new qualifying message reopens.
	for _, to := range []string{cases.StateAguardandoCliente, cases.StateResolvido} {
		if err := cases.Advance(ctx, store, first.CaseID, to); err != nil {
			t.Fatalf("advance to %s: %v", to, err)
		}
	}
	if n := countWhere(t, pool, `SELECT count(*) FROM outbox WHERE routing_key = $1`, event.NameCaseStatusChanged); n != 3 {
		t.Fatalf("case.status.changed rows = %d, want 3", n)
	}
	reopened, err := cases.Intake(ctx, store, lookup, complaintBody(t, customerID))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if reopened.Decision != cases.DecisionOpened || reopened.CaseID == first.CaseID {
		t.Fatalf("reopen = %+v, want a new case", reopened)
	}
	if n := countWhere(t, pool, `SELECT count(*) FROM cases WHERE customer_id = $1`, customerID); n != 2 {
		t.Fatalf("customer cases = %d, want 2", n)
	}
	if n := countWhere(t, pool,
		`SELECT count(*) FROM cases WHERE customer_id = $1 AND state <> 'Resolvido'`, customerID); n != 1 {
		t.Fatalf("open customer cases = %d, want 1", n)
	}
}

func TestPGXStore_SeededMessageWithoutOriginOpensNothing(t *testing.T) {
	t.Parallel()
	pool := newCasesPool(t)
	store := cases.NewPGXStore(pool)

	body := triagedBody(t, identity.MustNewV7(), identity.MustNewV7(),
		cases.TriagedMessage{SourceEventID: identity.MustNewV7(), Intent: "reclamacao", ChurnRisk: 0.8})
	res, err := cases.Intake(t.Context(), store, singular(identity.MustNewV7()), body)
	if err != nil || res.Decision != cases.DecisionIgnored {
		t.Fatalf("Intake = %+v, %v, want ignored", res, err)
	}
	if n := countWhere(t, pool, `SELECT count(*) FROM cases`); n != 0 {
		t.Fatalf("cases = %d, want 0", n)
	}
}

func openCase(t *testing.T, store *cases.PGXStore) string {
	t.Helper()
	id := identity.MustNewV7()
	in := cases.OpenInput{
		ID:         id,
		CustomerID: identity.MustNewV7(),
		AdvisorID:  identity.MustNewV7(),
		Segment:    book.SegmentAdvance,
		OccurredAt: time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC),
	}
	if err := cases.Open(t.Context(), store, in); err != nil {
		t.Fatalf("Open: %v", err)
	}
	return id
}

func TestPGXStore_OpenTwiceWritesOneOpenedRow(t *testing.T) {
	t.Parallel()
	pool := newCasesPool(t)
	store := cases.NewPGXStore(pool)
	in := cases.OpenInput{
		ID:         identity.MustNewV7(),
		CustomerID: identity.MustNewV7(),
		AdvisorID:  identity.MustNewV7(),
		Segment:    book.SegmentAdvance,
		OccurredAt: time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC),
	}
	for range 2 {
		if err := cases.Open(t.Context(), store, in); err != nil {
			t.Fatalf("Open: %v", err)
		}
	}
	if n := countWhere(t, pool, `SELECT count(*) FROM outbox WHERE routing_key = $1`, event.NameCaseOpened); n != 1 {
		t.Fatalf("case.opened rows = %d, want 1", n)
	}
	if n := countWhere(t, pool, `SELECT count(*) FROM cases`); n != 1 {
		t.Fatalf("cases = %d, want 1", n)
	}
}

func TestPGXStore_AdvanceToResolvido(t *testing.T) {
	t.Parallel()
	pool := newCasesPool(t)
	store := cases.NewPGXStore(pool)
	id := openCase(t, store)

	for _, to := range []string{cases.StateEmAtendimento, cases.StateAguardandoCliente, cases.StateResolvido} {
		if err := cases.Advance(t.Context(), store, id, to); err != nil {
			t.Fatalf("Advance to %s: %v", to, err)
		}
	}
	row, ok, err := store.GetCase(t.Context(), id)
	if err != nil || !ok || row.State != cases.StateResolvido {
		t.Fatalf("GetCase = %+v, %v, %v, want Resolvido", row, ok, err)
	}
	if n := countWhere(t, pool, `SELECT count(*) FROM outbox WHERE routing_key = $1`, event.NameCaseStatusChanged); n != 3 {
		t.Fatalf("case.status.changed rows = %d, want 3", n)
	}
}

func TestPGXStore_HandleBreachTwice(t *testing.T) {
	t.Parallel()
	pool := newCasesPool(t)
	store := cases.NewPGXStore(pool)
	id := openCase(t, store)
	at := time.Date(2026, 9, 29, 16, 0, 0, 0, time.UTC)

	for range 2 {
		if err := cases.HandleBreach(t.Context(), store, id, at); err != nil {
			t.Fatalf("HandleBreach: %v", err)
		}
	}
	row, ok, err := store.GetCase(t.Context(), id)
	if err != nil || !ok || !row.Escalated {
		t.Fatalf("GetCase = %+v, %v, %v, want escalated", row, ok, err)
	}
	if n := countWhere(t, pool, `SELECT count(*) FROM outbox WHERE routing_key = $1`, event.NameCaseSLABreached); n != 1 {
		t.Fatalf("case.sla.breached rows = %d, want 1", n)
	}
}

func TestPGXStore_IntakeIgnoresNonQualifying(t *testing.T) {
	t.Parallel()
	pool := newCasesPool(t)
	store := cases.NewPGXStore(pool)

	body := triagedBody(t, identity.MustNewV7(), identity.MustNewV7(),
		cases.TriagedMessage{Origin: cases.OriginClientApp, SourceEventID: identity.MustNewV7(), Intent: "cambio", ChurnRisk: 0.2})
	res, err := cases.Intake(t.Context(), store, singular(identity.MustNewV7()), body)
	if err != nil || res.Decision != cases.DecisionIgnored {
		t.Fatalf("Intake = %+v, %v", res, err)
	}
	if n := countWhere(t, pool, `SELECT count(*) FROM inbox`); n != 1 {
		t.Fatalf("inbox = %d, want 1", n)
	}
	if n := countWhere(t, pool, `SELECT count(*) FROM cases`); n != 0 {
		t.Fatalf("cases = %d, want 0", n)
	}
	if n := countWhere(t, pool, `SELECT count(*) FROM outbox`); n != 0 {
		t.Fatalf("outbox = %d, want 0", n)
	}
}

func TestPGXStore_ConcurrentIntakeOpensOneCase(t *testing.T) {
	t.Parallel()
	pool := newCasesPool(t)
	store := cases.NewPGXStore(pool)
	customerID := identity.MustNewV7()
	lookup := &fakeLookup{customer: cases.Customer{Segment: book.SegmentAdvance, AdvisorID: identity.MustNewV7()}}

	const n = 8
	bodies := make([][]byte, n)
	for i := range n {
		bodies[i] = complaintBody(t, customerID)
	}
	results := make([]cases.IntakeResult, n)
	errs := make([]error, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			<-start
			results[i], errs[i] = cases.Intake(t.Context(), store, lookup, bodies[i])
		})
	}
	close(start)
	wg.Wait()

	opened := 0
	for i := range n {
		if errs[i] != nil {
			t.Fatalf("Intake %d: %v", i, errs[i])
		}
		if results[i].Decision == cases.DecisionOpened {
			opened++
		}
	}
	if opened != 1 {
		t.Fatalf("opened = %d, want 1", opened)
	}
	if got := countWhere(t, pool, `SELECT count(*) FROM cases WHERE customer_id = $1`, customerID); got != 1 {
		t.Fatalf("customer cases = %d, want 1", got)
	}
	if got := countWhere(t, pool, `SELECT count(*) FROM case_history WHERE kind = 'mensagem'`); got != n-1 {
		t.Fatalf("mensagem rows = %d, want %d", got, n-1)
	}
}

func TestPGXStore_OneOpenCaseIndex(t *testing.T) {
	t.Parallel()
	pool := newCasesPool(t)
	ctx := t.Context()

	// The seed cast passes the index.
	applyCasesSQLDir(t, pool, filepath.Join("..", "..", "seeds", "cases"))

	const insert = `
INSERT INTO cases (id, customer_id, advisor_id, state, sla_total_minutes, opened_at)
VALUES ($1, $2, $3, $4, 60, now())`
	customerID := identity.MustNewV7()
	advisorID := identity.MustNewV7()
	for _, state := range []string{cases.StateResolvido, cases.StateResolvido, cases.StateAberto} {
		if _, err := pool.Exec(ctx, insert, identity.MustNewV7(), customerID, advisorID, state); err != nil {
			t.Fatalf("insert %s case: %v", state, err)
		}
	}
	_, err := pool.Exec(ctx, insert, identity.MustNewV7(), customerID, advisorID, cases.StateEmAtendimento)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("second open case err = %v, want unique violation", err)
	}
}

// A reseed returns the cases to the seed cast. A case opened from the app
// during a walk is resolved, so the client's home leaves case_open, and a
// cast customer holding such a case does not break the one-open-case index
// when the cast upsert reopens their seed case.
func TestSeedCast_ReseedResolvesAppOpenedCases(t *testing.T) {
	t.Parallel()
	pool := newCasesPool(t)
	ctx := t.Context()
	seeds := filepath.Join("..", "..", "seeds", "cases")
	applyCasesSQLDir(t, pool, seeds)

	const (
		mariana     = "01a0e3a4-9a44-7566-b5de-eb2e365799f8"
		paulo       = "01a0e3a4-9a44-7571-9cd6-29d603ed75d1"
		pauloCast   = "01a0e3a4-9a44-76e9-9644-aec5cdb8be90"
		advisorAna  = "01a0e3a4-9a44-750d-9e38-ca5f92ecf52a"
		insertOpen  = `INSERT INTO cases (id, customer_id, advisor_id, state, sla_total_minutes, opened_at) VALUES ($1, $2, $3, $4, 30, now())`
		stateOfCase = `SELECT state FROM cases WHERE id = $1`
	)
	marianaCase := identity.MustNewV7()
	if _, err := pool.Exec(ctx, insertOpen, marianaCase, mariana, advisorAna, cases.StateAberto); err != nil {
		t.Fatalf("open Mariana's app case: %v", err)
	}
	// Paulo's cast case was resolved during the walk and a new one opened.
	if _, err := pool.Exec(ctx, `UPDATE cases SET state = $1 WHERE id = $2`, cases.StateResolvido, pauloCast); err != nil {
		t.Fatalf("resolve Paulo's cast case: %v", err)
	}
	pauloCase := identity.MustNewV7()
	if _, err := pool.Exec(ctx, insertOpen, pauloCase, paulo, advisorAna, cases.StateEmAtendimento); err != nil {
		t.Fatalf("open Paulo's app case: %v", err)
	}

	applyCasesSQLDir(t, pool, seeds)

	for id, want := range map[string]string{
		marianaCase: cases.StateResolvido,
		pauloCase:   cases.StateResolvido,
		pauloCast:   cases.StateEmAtendimento,
	} {
		var got string
		if err := pool.QueryRow(ctx, stateOfCase, id).Scan(&got); err != nil {
			t.Fatalf("state of %s: %v", id, err)
		}
		if got != want {
			t.Errorf("case %s state = %q, want %q", id, got, want)
		}
	}
	if got := countWhere(t, pool, `SELECT count(*) FROM cases WHERE customer_id = $1 AND state <> $2`, mariana, cases.StateResolvido); got != 0 {
		t.Fatalf("Mariana open cases after reseed = %d, want 0", got)
	}
}
