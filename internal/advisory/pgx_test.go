package advisory_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"

	advisoryv1 "github.com/leohteixeira/advisor-radar/gen/advisory/v1"
	"github.com/leohteixeira/advisor-radar/internal/advisory"
	"github.com/leohteixeira/advisor-radar/internal/event"
	"github.com/leohteixeira/advisor-radar/internal/sim"
)

// advisoryTestDatabaseEnv names a PostgreSQL DSN for the gated pgx tests.
// Unset, they skip. Each test works in its own throwaway schema.
const advisoryTestDatabaseEnv = "ADVISORY_TEST_DATABASE_URL"

// newAdvisoryPool returns a pool whose search_path is a fresh, empty schema.
func newAdvisoryPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv(advisoryTestDatabaseEnv)
	if dsn == "" {
		t.Skipf("%s is not set", advisoryTestDatabaseEnv)
	}
	ctx := t.Context()

	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatalf("random schema suffix: %v", err)
	}
	schema := pgx.Identifier{"advisory_test_" + hex.EncodeToString(suffix)}.Sanitize()

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
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect schema pool: %v", err)
	}
	// Registered after the drop, so it runs first and releases connections.
	t.Cleanup(pool.Close)
	return pool
}

// applySQL runs one repository SQL file, as cmd/db does.
func applySQL(t *testing.T, pool *pgxpool.Pool, parts ...string) {
	t.Helper()
	path := filepath.Join(append([]string{"..", ".."}, parts...)...)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if _, err := pool.Exec(t.Context(), string(body)); err != nil {
		t.Fatalf("apply %s: %v", path, err)
	}
}

// newSeededAdvisoryPool returns a fresh schema with every advisory migration
// and the cast seed applied, as cmd/db does.
func newSeededAdvisoryPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := newAdvisoryPool(t)
	for _, name := range []string{"001_book.sql", "002_uuidv7.sql", "003_investor_profile.sql", "004_revaluation.sql"} {
		applySQL(t, pool, "migrations", "advisory", name)
	}
	applySQL(t, pool, "seeds", "advisory", "001_cast.sql")
	return pool
}

// TestPGX_InvestorProfileMigration applies 003 to a book that already has a
// row, then the seed: the existing row gets the migration default, later
// inserts must state a profile, and the CHECK refuses an unknown one.
func TestPGX_InvestorProfileMigration(t *testing.T) {
	t.Parallel()
	pool := newAdvisoryPool(t)
	ctx := t.Context()
	applySQL(t, pool, "migrations", "advisory", "001_book.sql")
	applySQL(t, pool, "migrations", "advisory", "002_uuidv7.sql")
	const early = "01a0e3a4-9a44-7000-8000-00000000e001"
	if _, err := pool.Exec(ctx, `
INSERT INTO book (customer_id, name, segment, aum, advisor_id, since)
VALUES ($1, 'Cliente Antigo', 'Essencial', 100, '01a0e3a4-9a44-750d-9e38-ca5f92ecf52a', '2020')`, early); err != nil {
		t.Fatalf("insert before 003: %v", err)
	}
	applySQL(t, pool, "migrations", "advisory", "003_investor_profile.sql")

	reader := advisory.NewBookReader(pool)
	got, err := reader.InvestorProfile(ctx, early)
	if err != nil {
		t.Fatalf("InvestorProfile: %v", err)
	}
	if got.Profile != advisory.ProfileConservador || got.AssessedOn.Format(time.DateOnly) != "2026-01-01" {
		t.Errorf("existing row = %+v, want the migration default", got)
	}

	if _, err := pool.Exec(ctx, `
INSERT INTO book (customer_id, name, segment, aum, advisor_id, since)
VALUES ('01a0e3a4-9a44-7000-8000-00000000e002', 'Sem Perfil', 'Essencial', 100, '01a0e3a4-9a44-750d-9e38-ca5f92ecf52a', '2020')`); err == nil {
		t.Error("insert without a profile succeeded after 003 dropped the default")
	}
	if _, err := pool.Exec(ctx, `UPDATE book SET investor_profile = 'agressivo' WHERE customer_id = $1`, early); err == nil {
		t.Error("CHECK accepted an unknown investor profile")
	}
}

// TestPGX_SeedAndMomentReads loads the migrations and the cast seed and
// reads what the moment and profile RPCs read.
func TestPGX_SeedAndMomentReads(t *testing.T) {
	t.Parallel()
	pool := newSeededAdvisoryPool(t)
	ctx := t.Context()
	reader := advisory.NewBookReader(pool)

	profiles := []struct {
		id, profile, assessed string
	}{
		{sim.CustomerFernanda, advisory.ProfileConservador, "2026-03-12"},
		{sim.CustomerMariana, advisory.ProfileModerado, "2026-01-20"},
		{sim.CustomerThiago, advisory.ProfileArrojado, "2026-08-04"},
	}
	for _, p := range profiles {
		got, err := reader.InvestorProfile(ctx, p.id)
		if err != nil {
			t.Fatalf("InvestorProfile(%s): %v", p.id, err)
		}
		if got.Profile != p.profile || got.AssessedOn.Format(time.DateOnly) != p.assessed {
			t.Errorf("InvestorProfile(%s) = %+v, want %s %s", p.id, got, p.profile, p.assessed)
		}
	}
	var missing int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM book WHERE investor_profile IS NULL OR profile_assessed_on IS NULL`).Scan(&missing); err != nil || missing != 0 {
		t.Errorf("book rows without a profile = %d, %v", missing, err)
	}

	customer, err := reader.GetCustomer(ctx, sim.CustomerMariana)
	if err != nil || customer.AdvisorID != "01a0e3a4-9a44-750d-9e38-ca5f92ecf52a" || customer.Advisor != "Ana Paula Ribeiro" {
		t.Errorf("GetCustomer = %+v, %v", customer, err)
	}
	// Cases intake reads advisor_id from this RPC alone, so check the gRPC
	// mapping over the real book.
	conn := dialBufconn(t, func(s *grpc.Server) {
		advisoryv1.RegisterAdvisoryServiceServer(s, advisory.NewGRPCServer(reader))
	})
	res, err := advisoryv1.NewAdvisoryServiceClient(conn).GetCustomer(ctx, &advisoryv1.GetCustomerRequest{CustomerId: sim.CustomerMariana})
	if err != nil || res.GetAdvisorId() != "01a0e3a4-9a44-750d-9e38-ca5f92ecf52a" || res.GetSegment() != "Singular" {
		t.Errorf("GetCustomer RPC = %v, %v", res, err)
	}

	unknown := "01a0e3a4-9a44-7000-8000-00000000e003"
	if _, err := reader.InvestorProfile(ctx, unknown); !errors.Is(err, advisory.ErrUnknownCustomer) {
		t.Errorf("InvestorProfile(unknown) = %v, want ErrUnknownCustomer", err)
	}
	if _, err := reader.MomentBook(ctx, unknown, time.Now().Add(-24*time.Hour)); !errors.Is(err, advisory.ErrUnknownCustomer) {
		t.Errorf("MomentBook(unknown) = %v, want ErrUnknownCustomer", err)
	}

	// Thiago's seeded upgrade is in the window but carries no schema version.
	since := time.Now().Add(-24 * time.Hour)
	mb, err := reader.MomentBook(ctx, sim.CustomerThiago, since)
	if err != nil {
		t.Fatalf("MomentBook(thiago): %v", err)
	}
	if mb.Segment != "Advance" || len(mb.Alerts) != 1 || mb.Alerts[0].SchemaVersion != 0 ||
		mb.Alerts[0].From != "Essencial" || mb.Alerts[0].To != "Advance" || mb.Revaluation != nil {
		t.Errorf("MomentBook(thiago) = %+v", mb)
	}
	if f := advisory.EvaluateMoments(advisory.MomentInput{Segment: mb.Segment, Alerts: mb.Alerts, Now: time.Now()}); f.SegmentUpgraded {
		t.Error("the seeded upgrade counts as a live one")
	}

	// A live deposit by Fernanda records a version 2 segment alert.
	store := advisory.NewPGXStore(pool)
	deposit := event.Envelope{
		Name:          event.NameAccountEventRecorded,
		EventID:       "01a0e3a4-9a44-7000-8000-00000000e004",
		OccurredAt:    time.Now().UTC().Add(-time.Minute),
		CustomerID:    sim.CustomerFernanda,
		SchemaVersion: event.SchemaVersionCents,
		Payload:       sim.AccountPayload{Kind: "aporte", Amount: 1_000_000, Before: 820_000, After: 1_820_000},
	}
	if err := advisory.Apply(ctx, store, deposit); err != nil {
		t.Fatalf("Apply deposit: %v", err)
	}
	mb, err = reader.MomentBook(ctx, sim.CustomerFernanda, since)
	if err != nil {
		t.Fatalf("MomentBook(fernanda): %v", err)
	}
	if mb.Segment != "Advance" || len(mb.Alerts) != 1 || mb.Alerts[0].SchemaVersion != event.SchemaVersionCents {
		t.Fatalf("MomentBook(fernanda) = %+v", mb)
	}
	f := advisory.EvaluateMoments(advisory.MomentInput{
		Segment: mb.Segment, Alerts: mb.Alerts, Now: time.Now(),
		Balance: advisory.Balance{PatrimonyCents: 1_820_000, CashCents: 1_114_800},
	})
	if !f.SegmentUpgraded || f.UpgradedSegment != "Advance" {
		t.Errorf("facts after the deposit = %+v, want segment_upgraded Advance", f)
	}
	var aporteVersion int
	if err := pool.QueryRow(ctx,
		`SELECT source_schema_version FROM alerts WHERE customer_id = $1 AND kind = $2`,
		sim.CustomerFernanda, advisory.KindAporte).Scan(&aporteVersion); err != nil || aporteVersion != 2 {
		t.Errorf("aporte source_schema_version = %d, %v", aporteVersion, err)
	}
}

// TestPGX_PurchaseSuitability applies v3 aplicacao events over the seeded
// book: Fernanda (conservador) buying cobalto raises one perfil alert and its
// outbox row; Thiago (arrojado) buying cobalto raises nothing. Both books
// follow the unchanged patrimony.
func TestPGX_PurchaseSuitability(t *testing.T) {
	t.Parallel()
	pool := newSeededAdvisoryPool(t)
	ctx := t.Context()
	store := advisory.NewPGXStore(pool)

	purchase := func(eventID, customerID string, patrimony float64) event.Envelope {
		return event.Envelope{
			Name:          event.NameAccountEventRecorded,
			EventID:       eventID,
			OccurredAt:    time.Now().UTC(),
			CustomerID:    customerID,
			SchemaVersion: event.SchemaVersionPositions,
			Payload: sim.AccountPayload{
				Kind: sim.KindAplicacao, Amount: 100_000, Before: patrimony, After: patrimony,
				ProductID: "cobalto", AssetClass: sim.ClassAcoes, Risk: 5,
			},
		}
	}
	fernanda := purchase("01a0e3a4-9a44-7000-8000-00000000e101", sim.CustomerFernanda, 820_000)
	if err := advisory.Apply(ctx, store, fernanda); err != nil {
		t.Fatalf("Apply fernanda: %v", err)
	}
	if err := advisory.Apply(ctx, store, fernanda); err != nil {
		t.Fatalf("redeliver fernanda: %v", err)
	}
	if err := advisory.Apply(ctx, store, purchase("01a0e3a4-9a44-7000-8000-00000000e102", sim.CustomerThiago, 6_800_000)); err != nil {
		t.Fatalf("Apply thiago: %v", err)
	}

	rows, err := pool.Query(ctx, `
SELECT customer_id::text, rule, source_schema_version, payload
FROM alerts
WHERE kind = $1`, advisory.KindPerfil)
	if err != nil {
		t.Fatalf("query alerts: %v", err)
	}
	type alertRow struct {
		customer, rule string
		version        int
		payload        advisory.AlertPayload
	}
	got, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (alertRow, error) {
		var r alertRow
		err := row.Scan(&r.customer, &r.rule, &r.version, &r.payload)
		return r, err
	})
	if err != nil {
		t.Fatalf("scan alerts: %v", err)
	}
	if len(got) != 1 || got[0].customer != sim.CustomerFernanda || got[0].rule != "Compra acima do perfil de investidor" ||
		got[0].version != event.SchemaVersionPositions {
		t.Fatalf("perfil alerts = %+v, want one for Fernanda", got)
	}
	p := got[0].payload
	if p.Amount != 1_000 || p.ProductID != "cobalto" || p.Risk != 5 || p.Profile != advisory.ProfileConservador || p.MaxRisk != 2 {
		t.Fatalf("payload = %+v", p)
	}
	var outbox int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox WHERE routing_key = $1 AND payload->'payload'->>'kind' = $2`,
		event.NameAlertRaised, advisory.KindPerfil).Scan(&outbox); err != nil || outbox != 1 {
		t.Fatalf("perfil outbox rows = %d, %v, want 1", outbox, err)
	}

	reader := advisory.NewBookReader(pool)
	for id, want := range map[string]float64{sim.CustomerFernanda: 8_200, sim.CustomerThiago: 68_000} {
		c, err := reader.GetCustomer(ctx, id)
		if err != nil || c.AUM != want {
			t.Fatalf("book %s = %+v, %v, want aum %v", id, c, err, want)
		}
	}
	// A customer outside the book fails the book update with
	// ErrUnknownCustomer (so the consumer dead-letters it) and claims nothing.
	const stranger = "01a0e3a4-9a44-7000-8000-00000000e1ff"
	deposit := event.Envelope{
		Name:          event.NameAccountEventRecorded,
		EventID:       "01a0e3a4-9a44-7000-8000-00000000e104",
		OccurredAt:    time.Now().UTC(),
		CustomerID:    stranger,
		SchemaVersion: event.SchemaVersionCents,
		Payload:       sim.AccountPayload{Kind: "aporte", Amount: 100_000, Before: 820_000, After: 920_000},
	}
	for _, env := range []event.Envelope{purchase("01a0e3a4-9a44-7000-8000-00000000e103", stranger, 820_000), deposit} {
		if err := advisory.Apply(ctx, store, env); !errors.Is(err, advisory.ErrUnknownCustomer) {
			t.Fatalf("Apply %s for a stranger = %v, want ErrUnknownCustomer", env.EventID, err)
		}
		var claimed int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM inbox WHERE event_id = $1::uuid`, env.EventID).Scan(&claimed); err != nil || claimed != 0 {
			t.Fatalf("inbox rows for %s = %d, %v, want 0", env.EventID, claimed, err)
		}
	}
}

// TestPGX_Revaluation applies the day-3 revaluations over the seeded book:
// Mariana's −15.5% raises one queda and her latest revaluation reads back as
// the portfolio_drop fact on day 3 only; Thiago's −1.6% raises nothing; a
// later revaluation replaces the latest one, a redelivered older day does
// not, and any day of a new epoch does.
func TestPGX_Revaluation(t *testing.T) {
	t.Parallel()
	pool := newSeededAdvisoryPool(t)
	ctx := t.Context()
	store := advisory.NewPGXStore(pool)

	const epoch = "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
	revaluation := func(eventID, customerID string, day int, amount, before float64, productID string, productBP int) event.Envelope {
		return event.Envelope{
			Name:          event.NameAccountEventRecorded,
			EventID:       eventID,
			OccurredAt:    time.Now().UTC(),
			CustomerID:    customerID,
			SchemaVersion: event.SchemaVersionPositions,
			Payload: sim.AccountPayload{
				Kind: sim.KindReavaliacao, Amount: amount, Before: before, After: before + amount,
				SimDay: day, ProductID: productID, ProductChangeBP: productBP, Epoch: epoch,
			},
		}
	}
	const marianaDay3 = "01a0e3a4-9a44-7000-8000-00000000e201"
	mariana := revaluation(marianaDay3, sim.CustomerMariana, 3, -3_852_000, 24_830_000, "cobalto", -5350)
	for range 2 {
		if err := advisory.Apply(ctx, store, mariana); err != nil {
			t.Fatalf("Apply mariana: %v", err)
		}
	}
	thiago := revaluation("01a0e3a4-9a44-7000-8000-00000000e202", sim.CustomerThiago, 3, -109_140, 6_800_000, "cobalto", -5350)
	if err := advisory.Apply(ctx, store, thiago); err != nil {
		t.Fatalf("Apply thiago: %v", err)
	}

	rows, err := pool.Query(ctx, `SELECT customer_id::text FROM alerts WHERE source_event_id = ANY($1::uuid[]) AND kind = $2`,
		[]string{marianaDay3, thiago.EventID}, advisory.KindQueda)
	if err != nil {
		t.Fatalf("query quedas: %v", err)
	}
	quedas, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("scan quedas: %v", err)
	}
	if len(quedas) != 1 || quedas[0] != sim.CustomerMariana {
		t.Fatalf("queda alerts = %v, want one for Mariana", quedas)
	}

	reader := advisory.NewBookReader(pool)
	c, err := reader.GetCustomer(ctx, sim.CustomerMariana)
	if err != nil || c.AUM != 209_780 || c.Segment != "Singular" {
		t.Fatalf("book mariana = %+v, %v, want aum 209780 Singular", c, err)
	}
	mb, err := reader.MomentBook(ctx, sim.CustomerMariana, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("MomentBook(mariana): %v", err)
	}
	want := advisory.Revaluation{
		Epoch: epoch, SimDay: 3, AmountCents: -3_852_000, BeforeCents: 24_830_000,
		ProductID: "cobalto", ProductChangeBP: -5350, SourceEventID: marianaDay3,
	}
	if mb.Revaluation == nil || *mb.Revaluation != want {
		t.Fatalf("MomentBook(mariana).Revaluation = %+v, want %+v", mb.Revaluation, want)
	}

	facts := func(day int) *advisoryv1.MomentFacts {
		t.Helper()
		balances := fakeBalances{balances: map[string]advisory.Balance{
			sim.CustomerMariana: {PatrimonyCents: 20_978_000, CashCents: 6_000_000, SimDay: day},
		}}
		conn := dialBufconn(t, func(s *grpc.Server) {
			advisoryv1.RegisterAdvisoryServiceServer(s, advisory.NewGRPCServer(reader, advisory.WithAccountReader(balances)))
		})
		res, err := advisoryv1.NewAdvisoryServiceClient(conn).GetMomentFacts(ctx, &advisoryv1.GetMomentFactsRequest{CustomerId: sim.CustomerMariana})
		if err != nil {
			t.Fatalf("GetMomentFacts day %d: %v", day, err)
		}
		return res
	}
	if f := facts(3); !f.GetPortfolioDrop() || f.GetDropBp() != 1551 || f.GetDropProductId() != "cobalto" ||
		f.GetDropProductBp() != -5350 || f.GetDropDay() != 3 {
		t.Fatalf("facts on day 3 = %v, want portfolio_drop 1551 cobalto -5350 day 3", f)
	}
	if f := facts(4); f.GetPortfolioDrop() {
		t.Fatalf("facts on day 4 = %v, want no portfolio_drop", f)
	}

	flat := revaluation("01a0e3a4-9a44-7000-8000-00000000e203", sim.CustomerMariana, 4, 0, 20_978_000, "", 0)
	if err := advisory.Apply(ctx, store, flat); err != nil {
		t.Fatalf("Apply day 4: %v", err)
	}
	mb, err = reader.MomentBook(ctx, sim.CustomerMariana, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("MomentBook(mariana) day 4: %v", err)
	}
	if mb.Revaluation == nil || mb.Revaluation.SimDay != 4 || mb.Revaluation.AmountCents != 0 || mb.Revaluation.ProductID != "" {
		t.Fatalf("latest revaluation after day 4 = %+v", mb.Revaluation)
	}
	if f := facts(4); f.GetPortfolioDrop() {
		t.Fatalf("facts after the flat day 4 = %v, want no portfolio_drop", f)
	}

	latest := func() advisory.Revaluation {
		t.Helper()
		mb, err := reader.MomentBook(ctx, sim.CustomerMariana, time.Now().Add(-24*time.Hour))
		if err != nil || mb.Revaluation == nil {
			t.Fatalf("MomentBook(mariana) = %+v, %v", mb.Revaluation, err)
		}
		return *mb.Revaluation
	}
	late := revaluation("01a0e3a4-9a44-7000-8000-00000000e204", sim.CustomerMariana, 3, -3_852_000, 24_830_000, "cobalto", -5350)
	if err := advisory.Apply(ctx, store, late); err != nil {
		t.Fatalf("Apply a late day 3: %v", err)
	}
	if got := latest(); got.SimDay != 4 || got.SourceEventID != flat.EventID {
		t.Fatalf("latest after a late day 3 = %+v, want day 4 kept", got)
	}
	const nextEpoch = "0b0a7f1c-3a3e-4c1d-9a55-2f1d6c0e8b11"
	reseeded := revaluation("01a0e3a4-9a44-7000-8000-00000000e205", sim.CustomerMariana, 1, 0, 24_830_000, "", 0)
	payload := reseeded.Payload.(sim.AccountPayload)
	payload.Epoch = nextEpoch
	reseeded.Payload = payload
	if err := advisory.Apply(ctx, store, reseeded); err != nil {
		t.Fatalf("Apply day 1 of a new epoch: %v", err)
	}
	if got := latest(); got.SimDay != 1 || got.Epoch != nextEpoch {
		t.Fatalf("latest after a reseed = %+v, want day 1 of %s", got, nextEpoch)
	}
}
