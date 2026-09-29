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
	pool := newAdvisoryPool(t)
	ctx := t.Context()
	for _, name := range []string{"001_book.sql", "002_uuidv7.sql", "003_investor_profile.sql"} {
		applySQL(t, pool, "migrations", "advisory", name)
	}
	applySQL(t, pool, "seeds", "advisory", "001_cast.sql")
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
		mb.Alerts[0].From != "Essencial" || mb.Alerts[0].To != "Advance" {
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
