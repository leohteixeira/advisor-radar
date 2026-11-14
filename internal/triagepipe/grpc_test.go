package triagepipe_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	triagev1 "github.com/leohteixeira/advisor-radar/gen/triage/v1"
	"github.com/leohteixeira/advisor-radar/internal/identity"
	"github.com/leohteixeira/advisor-radar/internal/triagepipe"
)

// triageTestDatabaseEnv names a PostgreSQL DSN for the gated review tests.
// Unset, they skip. Each test migrates its own throwaway schema and drops it
// after.
const triageTestDatabaseEnv = "TRIAGE_TEST_DATABASE_URL"

// newTriagePool returns a pool whose search_path is a fresh schema migrated
// with migrations/triage.
func newTriagePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv(triageTestDatabaseEnv)
	if dsn == "" {
		t.Skipf("%s is not set", triageTestDatabaseEnv)
	}
	ctx := t.Context()

	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatalf("random schema suffix: %v", err)
	}
	schema := pgx.Identifier{"triage_test_" + hex.EncodeToString(suffix)}.Sanitize()

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

	dir := filepath.Join("..", "..", "migrations", "triage")
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
		if _, err := pool.Exec(ctx, string(body)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	return pool
}

// seedResult describes one results row. The seed cast stores labels in
// intent, the pipeline stores keys; both spellings must read the same.
type seedResult struct {
	intent   string
	prob     float64
	degraded bool
	payload  string
}

func insertResult(t *testing.T, pool *pgxpool.Pool, r seedResult) string {
	t.Helper()
	ids := make([]string, 3)
	for i := range ids {
		id, err := identity.NewV7()
		if err != nil {
			t.Fatalf("new id: %v", err)
		}
		ids[i] = id
	}
	if r.payload == "" {
		r.payload = `{"channel":"chat","text":"mensagem"}`
	}
	const q = `
INSERT INTO results (
    id, source_event_id, customer_id, intent, intent_prob, frustration,
    churn_risk, wants_human, classifier, model_version, degraded, needs_review,
    created_at, payload
)
VALUES ($1,$2,$3,$4,$5,0,0,0,'test','test',$6,$7,now(),$8::jsonb)`
	if _, err := pool.Exec(t.Context(), q, ids[0], ids[1], ids[2], r.intent, r.prob, r.degraded, r.prob < 0.85, r.payload); err != nil {
		t.Fatalf("insert result: %v", err)
	}
	return ids[0]
}

func TestReviewReader_ListReview_ShowsCanonicalLabels(t *testing.T) {
	t.Parallel()
	pool := newTriagePool(t)
	insertResult(t, pool, seedResult{
		intent: "reclamacao", prob: 0.6, degraded: true,
		payload: `{"channel":"chat","text":"cobrado duas vezes","dist":{"reclamacao":0.6,"tributacao":0.4}}`,
	})

	rows, err := triagepipe.NewReviewReader(pool).ListReview(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.Intent != "Reclamação" || !row.Fallback {
		t.Errorf("intent=%q fallback=%v, want Reclamação from the fallback", row.Intent, row.Fallback)
	}
	if len(row.Dist) != 2 || row.Dist["Reclamação"] != 0.6 || row.Dist["Tributação"] != 0.4 {
		t.Errorf("dist = %v, want canonical labels", row.Dist)
	}
}

func TestGRPCServer_CorrectIntent_AcceptsKeyOrLabel(t *testing.T) {
	t.Parallel()
	pool := newTriagePool(t)
	srv := triagepipe.NewGRPCServer(triagepipe.NewReviewReader(pool))

	tests := []struct {
		name      string
		submitted string
		expected  string
	}{
		{name: "label", submitted: "Tributação", expected: "Tributação"},
		{name: "key", submitted: "tributacao", expected: "Tributação"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := insertResult(t, pool, seedResult{intent: "operacional", prob: 0.3, degraded: true})
			item, err := srv.CorrectIntent(t.Context(), &triagev1.CorrectIntentRequest{Id: id, Intent: tt.submitted})
			if err != nil {
				t.Fatal(err)
			}
			if item.GetIntent() != tt.expected || !item.GetCorrected() {
				t.Errorf("intent=%q corrected=%v, want %q corrected", item.GetIntent(), item.GetCorrected(), tt.expected)
			}
		})
	}

	t.Run("unknown intent", func(t *testing.T) {
		id := insertResult(t, pool, seedResult{intent: "operacional", prob: 0.3})
		_, err := srv.CorrectIntent(t.Context(), &triagev1.CorrectIntentRequest{Id: id, Intent: "Outra"})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
		}
	})
}

// The manager panel reads IntentStats; the review screen reads ListReview.
// A review must move both, and one intent must be one group whatever the
// spelling the row stores.
func TestReviewReader_IntentStats_FollowsReviewsAndGroupsSpellings(t *testing.T) {
	t.Parallel()
	pool := newTriagePool(t)
	reader := triagepipe.NewReviewReader(pool)

	pendingKey := insertResult(t, pool, seedResult{intent: "reclamacao", prob: 0.6, degraded: true})
	insertResult(t, pool, seedResult{intent: "Reclamação", prob: 0.82})
	insertResult(t, pool, seedResult{intent: "tributacao", prob: 0.6, degraded: true})
	insertResult(t, pool, seedResult{intent: "Tributação", prob: 0.9})

	reviewPct, fallbackPct, intents, err := reader.IntentStats(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if reviewPct != 75 || fallbackPct != 50 {
		t.Errorf("review=%d fallback=%d, want 75 and 50", reviewPct, fallbackPct)
	}
	if len(intents) != 2 || intents["Reclamação"] != 2 || intents["Tributação"] != 2 {
		t.Errorf("intents = %v, want Reclamação 2 and Tributação 2", intents)
	}

	if _, err := reader.CorrectIntent(t.Context(), pendingKey, "Tributação"); err != nil {
		t.Fatal(err)
	}

	reviewPct, _, intents, err = reader.IntentStats(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := reader.ListReview(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	pending := 0
	for _, row := range rows {
		if !row.Corrected {
			pending++
		}
	}
	if pending != 2 || reviewPct != 50 {
		t.Errorf("pending=%d review=%d, want 2 pending and 50", pending, reviewPct)
	}
	if len(intents) != 2 || intents["Reclamação"] != 1 || intents["Tributação"] != 3 {
		t.Errorf("intents = %v, want Reclamação 1 and Tributação 3", intents)
	}
}

func TestReviewReader_IntentStats_EmptyDatabase(t *testing.T) {
	t.Parallel()
	pool := newTriagePool(t)

	reviewPct, fallbackPct, intents, err := triagepipe.NewReviewReader(pool).IntentStats(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if reviewPct != 0 || fallbackPct != 0 || len(intents) != 0 {
		t.Errorf("review=%d fallback=%d intents=%v, want zeroes", reviewPct, fallbackPct, intents)
	}
}
