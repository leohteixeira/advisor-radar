package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"maps"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/leohteixeira/advisor-radar/internal/identity"
)

// testDatabaseEnv names the PostgreSQL the DB-gated tests of this repository
// share (CI points every *_TEST_DATABASE_URL at one database). Unset, the
// test skips. Each service gets its own throwaway schema there, dropped
// after.
const testDatabaseEnv = "ACCOUNT_SIM_TEST_DATABASE_URL"

var serviceDatabaseEnv = map[string]string{
	"account_sim": "ACCOUNT_SIM_DATABASE_URL",
	"advisory":    "ADVISORY_DATABASE_URL",
	"triage":      "TRIAGE_DATABASE_URL",
	"cases":       "CASES_DATABASE_URL",
}

// Cast ids the walk touches.
const (
	castMariana         = "01a0e3a4-9a44-7566-b5de-eb2e365799f8"
	castFernanda        = "01a0e3a4-9a44-757a-ac8f-dab7db5eb068"
	castThiago          = "01a0e3a4-9a44-75dd-b3a0-403a7a87836e"
	castAna             = "01a0e3a4-9a44-750d-9e38-ca5f92ecf52a"
	castFernandaMessage = "01a0e3a4-9a44-7844-ac70-f9372bd01f7c"
	castFernandaSignal  = "01a0e3a4-9a44-7653-b766-1a6074a0db6b"
	castJulianaResult   = "01a0e3a4-9a44-7703-a783-9b51d7e16ba7"
	castPauloCase       = "01a0e3a4-9a44-76e9-9644-aec5cdb8be90"
)

// serviceSchemas gives every service a fresh schema in the test database and
// points its *_DATABASE_URL at it, as four databases would be. It returns one
// pool per service.
func serviceSchemas(t *testing.T) map[string]*pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv(testDatabaseEnv)
	if dsn == "" {
		t.Skipf("%s is not set", testDatabaseEnv)
	}
	ctx := t.Context()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(admin.Close)

	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatalf("random schema suffix: %v", err)
	}
	pools := make(map[string]*pgxpool.Pool, len(services))
	for _, service := range services {
		schema := "db_test_" + service + "_" + hex.EncodeToString(suffix)
		quoted := pgx.Identifier{schema}.Sanitize()
		if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
			t.Fatalf("create schema %s: %v", schema, err)
		}
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if _, err := admin.Exec(ctx, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
				t.Errorf("drop schema %s: %v", schema, err)
			}
		})

		parsed, err := url.Parse(dsn)
		if err != nil {
			t.Fatalf("parse %s: %v", testDatabaseEnv, err)
		}
		query := parsed.Query()
		query.Set("search_path", schema)
		parsed.RawQuery = query.Encode()
		t.Setenv(serviceDatabaseEnv[service], parsed.String())

		pool, err := pgxpool.New(ctx, parsed.String())
		if err != nil {
			t.Fatalf("connect %s: %v", schema, err)
		}
		// Registered after the drop, so it runs first and releases connections.
		t.Cleanup(pool.Close)
		pools[service] = pool
	}
	return pools
}

// snapshot reads every table of a service as sorted JSON rows, so two
// snapshots are equal exactly when the service holds the same data. What a
// seed stamps anew each time is normalized: a timestamp reads as set or not
// set, the outbox serial and the simulation epoch are left out, and so is the
// occurred_at inside an event body.
func snapshot(t *testing.T, pool *pgxpool.Pool) map[string][]string {
	t.Helper()
	ctx := t.Context()
	const columnsQ = `
SELECT table_name, column_name, data_type
FROM information_schema.columns
WHERE table_schema = current_schema() AND table_name <> 'schema_migrations'
ORDER BY table_name, ordinal_position`
	rows, err := pool.Query(ctx, columnsQ)
	if err != nil {
		t.Fatalf("list columns: %v", err)
	}
	selects := map[string][]string{}
	for rows.Next() {
		var table, column, dataType string
		if err := rows.Scan(&table, &column, &dataType); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		quoted := pgx.Identifier{column}.Sanitize()
		switch {
		case table == "outbox" && column == "id", column == "epoch":
			continue
		case dataType == "timestamp with time zone":
			selects[table] = append(selects[table], fmt.Sprintf("(%s IS NOT NULL) AS %s", quoted, quoted))
		case table == "outbox" && column == "payload":
			selects[table] = append(selects[table], fmt.Sprintf("(%s #- '{occurred_at}') AS %s", quoted, quoted))
		default:
			selects[table] = append(selects[table], quoted)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("list columns: %v", err)
	}
	if len(selects) == 0 {
		t.Fatal("the service has no tables")
	}

	out := make(map[string][]string, len(selects))
	for table, columns := range selects {
		q := fmt.Sprintf("SELECT to_jsonb(x)::text FROM (SELECT %s FROM %s) x ORDER BY 1",
			strings.Join(columns, ", "), pgx.Identifier{table}.Sanitize())
		tableRows, err := pool.Query(ctx, q)
		if err != nil {
			t.Fatalf("read %s: %v", table, err)
		}
		lines, err := pgx.CollectRows(tableRows, pgx.RowTo[string])
		if err != nil {
			t.Fatalf("read %s: %v", table, err)
		}
		out[table] = lines
	}
	return out
}

// eventBody is an event envelope as the services store it in an outbox.
func eventBody(eventID, customerID string, version int, payload string) string {
	return fmt.Sprintf(`{"event_id":%q,"occurred_at":"2026-11-13T18:58:09Z","customer_id":%q,"schema_version":%d,"payload":%s}`,
		eventID, customerID, version, payload)
}

// walk writes into every service what a demo walk leaves behind, the rows
// the e2e campaign found after a reseed: Thiago's withdrawal and its alerts,
// a purchase, an advanced day, a review correction, a case opened from the
// app, and changes to the cast itself.
func walk(t *testing.T, pools map[string]*pgxpool.Pool) {
	t.Helper()
	withdrawal, purchase := identity.MustNewV7(), identity.MustNewV7()
	message, triaged := identity.MustNewV7(), identity.MustNewV7()
	saqueAlert, segmentAlert, openCommand := identity.MustNewV7(), identity.MustNewV7(), identity.MustNewV7()
	appCase, caseOpened := identity.MustNewV7(), identity.MustNewV7()

	steps := []struct {
		service string
		sql     string
		args    []any
	}{
		// account-sim: the events of the walk, their idempotency keys, moved
		// balances and positions, an advanced day, changed preferences.
		{"account_sim", `INSERT INTO outbox (event_id, routing_key, payload, published_at) VALUES
			($1, 'account.event.recorded', $2::jsonb, now()),
			($3, 'account.event.recorded', $4::jsonb, now()),
			($5, 'message.received', $6::jsonb, now())`, []any{
			withdrawal, eventBody(withdrawal, castThiago, 2, `{"kind":"saque","amount":5900000,"before":6800000,"after":900000,"destination":"conta"}`),
			purchase, eventBody(purchase, castFernanda, 3, `{"kind":"aplicacao","amount":100000,"before":820000,"after":820000,"product_id":"cobalto","asset_class":"acoes","risk":5}`),
			message, eventBody(message, castMariana, 1, `{"channel":"chat","text":"Quero encerrar minha conta.","origin":"client_app"}`),
		}},
		{"account_sim", `INSERT INTO pov_idempotency (customer_id, idem_key, event_id) VALUES ($1, 'walk-withdrawal', $2), ($3, 'walk-purchase', $4)`,
			[]any{castThiago, withdrawal, castFernanda, purchase}},
		{"account_sim", `UPDATE pov_account SET caixa = 152000 WHERE customer_id = $1`, []any{castThiago}},
		{"account_sim", `INSERT INTO pov_position (customer_id, product_id, units_cents, applied_cents) VALUES ($1, 'cobalto', 100000, 100000)`, []any{castFernanda}},
		{"account_sim", `UPDATE pov_sim SET sim_day = 3`, nil},
		{"account_sim", `INSERT INTO pov_advance (idem_key, sim_day, event_ids) VALUES ('walk-advance', 3, ARRAY['e1'])`, nil},
		{"account_sim", `UPDATE pov_preferences SET channel = 'email', beta = true WHERE customer_id = $1`, []any{castMariana}},
		{"account_sim", `UPDATE outbox SET published_at = now()`, nil},

		// advisory: the alerts the withdrawal raised, the open-case command,
		// a Contatado mark on a cast signal, a revaluation, a moved book.
		{"advisory", `INSERT INTO inbox (event_id) VALUES ($1), ($2)`, []any{withdrawal, purchase}},
		{"advisory", `INSERT INTO alerts (id, customer_id, kind, rule, source_event_id, raised_at, payload, source_schema_version) VALUES
			($1, $3, 'saque', 'Saque acima de 20% do patrimônio em 24 horas', $4, now(), '{"kind":"saque","reason":"Saque de US$ 59.000,00, 87% do patrimônio"}', 2),
			($2, $3, 'segmento', 'Patrimônio cruzou a faixa de US$ 10 mil', $4, now(), '{"kind":"segmento","reason":"Desceu de Advance para Essencial"}', 2)`,
			[]any{saqueAlert, segmentAlert, castThiago, withdrawal}},
		{"advisory", `INSERT INTO outbox (event_id, routing_key, payload, published_at) VALUES
			($1, 'alert.raised', $2::jsonb, now()),
			($3, 'alert.raised', $4::jsonb, now()),
			($5, 'case.open.requested', $6::jsonb, NULL)`, []any{
			saqueAlert, eventBody(saqueAlert, castThiago, 1, `{"kind":"saque","rule":"Saque acima de 20% do patrimônio em 24 horas","reason":"Saque de US$ 59.000,00, 87% do patrimônio"}`),
			segmentAlert, eventBody(segmentAlert, castThiago, 1, `{"kind":"segmento","rule":"Patrimônio cruzou a faixa de US$ 10 mil","reason":"Desceu de Advance para Essencial"}`),
			openCommand, eventBody(openCommand, castThiago, 1, `{"signal_id":"`+saqueAlert+`"}`),
		}},
		{"advisory", `INSERT INTO signal_action (signal_id, contacted_at) VALUES ($1, now())`, []any{castFernandaSignal}},
		{"advisory", `INSERT INTO revaluation (customer_id, epoch, sim_day, amount_cents, before_cents, product_id, product_change_bp, source_event_id)
			VALUES ($1, gen_random_uuid(), 3, -109140, 900000, 'cobalto', -5350, $2)`, []any{castThiago, identity.MustNewV7()}},
		{"advisory", `UPDATE book SET aum = 9000, segment = 'Essencial', investor_profile = 'conservador' WHERE customer_id = $1`, []any{castThiago}},
		{"advisory", `UPDATE outbox SET published_at = now() WHERE routing_key <> 'case.open.requested'`, nil},

		// triage: Fernanda's cast message classified live after an earlier
		// seed (the review item that read 36 minutes beside an 18-minute
		// queue card), the walk's message, a correction on a cast result.
		{"triage", `INSERT INTO inbox (event_id) VALUES ($1)`, []any{message}},
		{"triage", `INSERT INTO results (id, source_event_id, customer_id, intent, intent_prob, frustration, churn_risk, wants_human,
				classifier, model_version, degraded, needs_review, corrected_intent, created_at, payload) VALUES
			($1, $2, $3, 'Contato', 0.6, 0, 0.1, 1, 'heuristic', 'h1', true, true, NULL, now() - interval '36 minutes', '{"channel":"chat","text":"Consegue pedir para o meu assessor me ligar?"}'),
			($4, $5, $6, 'Encerramento', 0.9, 0, 0.9, 0, 'heuristic', 'h1', true, false, NULL, now(), '{"channel":"chat","text":"Quero encerrar minha conta."}')`,
			[]any{identity.MustNewV7(), castFernandaMessage, castFernanda, triaged, message, castMariana}},
		{"triage", `UPDATE results SET corrected_intent = 'Câmbio' WHERE id = $1`, []any{castJulianaResult}},
		{"triage", `INSERT INTO outbox (event_id, routing_key, payload, published_at) VALUES ($1, 'message.triaged', $2::jsonb, now())`,
			[]any{triaged, eventBody(triaged, castMariana, 1, `{"source_event_id":"`+message+`","origin":"client_app","intent":"Encerramento","churn_risk":0.9}`)}},
		{"triage", `UPDATE outbox SET published_at = now()`, nil},

		// cases: a case opened from the app with its history and armed SLA
		// delay, and a cast case resolved, re-timed, and annotated.
		{"cases", `INSERT INTO cases (id, customer_id, signal_id, advisor_id, state, sla_total_minutes, escalated, opened_at, sla_started_at)
			VALUES ($1, $2, NULL, $3, 'Aberto', 30, true, now(), now())`, []any{appCase, castMariana, castAna}},
		{"cases", `INSERT INTO case_history (id, case_id, kind, text, occurred_at, signal_id) VALUES
			($1, $2, 'caso', 'Caso aberto a partir de pedido de encerramento', now(), NULL),
			($3, $4, 'nota', 'Nota escrita durante a caminhada', now(), $5)`,
			[]any{identity.MustNewV7(), appCase, identity.MustNewV7(), castPauloCase, saqueAlert}},
		{"cases", `UPDATE cases SET state = 'Resolvido', sla_total_minutes = 720, sla_started_at = now(), escalated = true WHERE id = $1`, []any{castPauloCase}},
		{"cases", `INSERT INTO sla_delay (case_id, ttl_ms, published_at) VALUES ($1, 1800000, now())`, []any{appCase}},
		{"cases", `INSERT INTO inbox (event_id) VALUES ($1), ($2)`, []any{triaged, openCommand}},
		{"cases", `INSERT INTO outbox (event_id, routing_key, payload, published_at) VALUES ($1, 'case.opened', $2::jsonb, now())`,
			[]any{caseOpened, eventBody(caseOpened, castMariana, 1, `{"case_id":"`+appCase+`","state":"Aberto","escalated":false,"sla_total_minutes":30}`)}},
		{"cases", `UPDATE outbox SET published_at = now()`, nil},
	}
	for _, step := range steps {
		if _, err := pools[step.service].Exec(t.Context(), step.sql, step.args...); err != nil {
			t.Fatalf("walk %s: %v\n%s", step.service, err, step.sql)
		}
	}
}

// A reseed returns every service to the cast: whatever a walk recorded or
// changed, each table then holds exactly what the first seed gave it, with
// every event unpublished again. Nothing from before the reseed is left for
// the timeline replay, the queue, the review, or the manager panel to show.
//
// Not parallel: it points the process's *_DATABASE_URL at its schemas.
func TestSeed_ReseedLeavesOnlyTheCast(t *testing.T) {
	pools := serviceSchemas(t)
	ctx := t.Context()
	root, err := repoRoot()
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	if err := migrateAll(ctx, root); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := seedAll(ctx, root, io.Discard); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cast := make(map[string]map[string][]string, len(services))
	for _, service := range services {
		cast[service] = snapshot(t, pools[service])
	}
	walk(t, pools)
	for _, service := range services {
		walked := snapshot(t, pools[service])
		var changed []string
		for table, rows := range walked {
			if !slices.Equal(rows, cast[service][table]) {
				changed = append(changed, table)
			}
		}
		slices.Sort(changed)
		t.Logf("%s: the walk changed %v", service, changed)
		if len(changed) == 0 {
			t.Fatalf("%s: the walk changed nothing, so the reseed proves nothing", service)
		}
	}

	if err := seedAll(ctx, root, io.Discard); err != nil {
		t.Fatalf("reseed: %v", err)
	}
	for _, service := range services {
		reseeded := snapshot(t, pools[service])
		for _, table := range slices.Sorted(maps.Keys(reseeded)) {
			got, want := reseeded[table], cast[service][table]
			if slices.Equal(got, want) {
				continue
			}
			t.Errorf("%s.%s after the reseed holds %d rows, the cast %d", service, table, len(got), len(want))
			for _, row := range got {
				if !slices.Contains(want, row) {
					t.Errorf("  left from the walk: %s", row)
				}
			}
			for _, row := range want {
				if !slices.Contains(got, row) {
					t.Errorf("  missing from the cast: %s", row)
				}
			}
		}
	}

	// Fernanda's message keeps no result from the walk, which would show it
	// with its old age in review, and is claimed again: her seeded queue
	// signal already stands for it, so triage must not classify it anew.
	var results, claims int
	if err := pools["triage"].QueryRow(ctx,
		`SELECT (SELECT count(*) FROM results WHERE source_event_id = $1), (SELECT count(*) FROM inbox WHERE event_id = $1)`,
		castFernandaMessage,
	).Scan(&results, &claims); err != nil {
		t.Fatalf("read triage: %v", err)
	}
	if results != 0 || claims != 1 {
		t.Errorf("triage holds %d results and %d inbox claims for Fernanda's message, want 0 and 1", results, claims)
	}
}
