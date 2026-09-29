---
title: 'Open cases from triaged messages'
type: 'feature'
created: '2026-09-29'
status: 'done'
review_loop_iteration: 0
baseline_revision: '257cf651722d66265a348540bcf39edeeb21adfc'
followup_review_recommended: true
context:
  - '{project-root}/AGENTS.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/architecture.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/SPEC.md'
  - '{project-root}/specs/adr/0008-client-command-grpc-outbox.md'
warnings: []
deferred:
  - summary: >-
      cases resolves the advisor id by matching the advisory advisor name against ListOperators.
    evidence: |-
      advisory GetCustomer returns only the advisor display name; a duplicate or renamed name dead-letters every qualifying message for that customer. Story 8 adds advisor_id to GetCustomer and switches the cases adapter.
    location: >-
      internal/cases/advisory.go
    severity: medium
---

<intent-contract>

## Intent

**Problem:** Phase 1 left `cases.Open` uncalled: nothing consumes `message.triaged`, so a complaint, a closing request, or a churn signal never becomes a case. Mariana's complaint therefore has no case, no Singular SLA, and nothing for the story 8 `case_open` moment to read.

**Approach:** cases consumes `message.triaged` on its own queue through its existing `inbox`. It opens one case per customer when the intent is `reclamacao` or `encerramento`, or when `churn_risk` ≥ 0.5, taking segment and advisor from advisory `GetCustomer` over gRPC with a deadline. A later qualifying message while a case is open is added to that case's history instead of opening another.

## Boundaries & Constraints

**Always:**
- Load `golang-how-to` first and apply the Go skills listed in `/workspace/repos/advisor-radar/CLAUDE.md`.
- **Decision rule.** Put it in a pure function in `internal/cases`, e.g. `Qualifies(TriagedMessage) (reason, bool)`. It qualifies when intent ∈ {`reclamacao`, `encerramento`} or `churn_risk` ≥ `ChurnRiskOpen` (0.5, a named constant). Every other intent, and a churn risk below 0.5, is claimed in the inbox and ignored.
- **Local payload type.** cases declares its own `TriagedMessage` with the JSON fields it needs: `source_event_id`, `intent`, `frustration`, `churn_risk`, `wants_human`, `degraded`, `needs_review`. Do not import `internal/triagepipe`.
- **Customer lookup port.** cases declares a small consumer interface, e.g. `CustomerLookup.Lookup(ctx, customerID) (Customer{Segment, AdvisorID}, error)`. An adapter over `advisoryv1.AdvisoryServiceClient.GetCustomer` sits in `internal/cases`, or in `cmd/cases` if simpler. The adapter maps `NotFound` to a sentinel `ErrUnknownCustomer`. Each call carries a deadline: the delivery context plus a 2 s default when that context has none. The lookup runs **before** the database transaction, never inside it.
- **Unusable customer.** An empty segment, a segment `BaseMinutes` does not know, or an empty advisor is a permanent error.
- **One transaction per message**, in this order:
  1. `ClaimInbox(event_id)` of the triaged envelope. On a duplicate, stop and succeed.
  2. If the message does not qualify, commit (the inbox row only).
  3. Serialize per customer with a transaction-scoped advisory lock, using the two-key form with a cases namespace constant, as the story 2 account-sim pgx store does.
  4. Look for a non-`Resolvido` case of the customer. If one exists, append one `case_history` row to it (kind `mensagem`, text "Nova mensagem do cliente: {reason}"). Do not open a second case and publish nothing.
  5. Otherwise open a new case in the same transaction. This reuses `Open`'s body, refactored into a tx-level helper that `Open` still calls, so existing `Open` callers and tests keep their behavior. The new case gets:
     - a new UUIDv7 id;
     - `SignalID` = the payload `source_event_id` when it is a valid UUID, else the triaged `event_id`;
     - segment and advisor from the lookup;
     - clock factors: `ChurnRisk` = churn_risk ≥ 0.5, `Frustration` = `int(math.Round(frustration))`, `HumanRequested` = wants_human ≥ 0.5, `RelevantWithdrawal` = false;
     - `OccurredAt` = the triaged envelope's `occurred_at`;
     - a `case_history` row of kind `caso` whose text is "Caso aberto a partir de mensagem com reclamação", "Caso aberto a partir de pedido de encerramento", or "Caso aberto a partir de risco de saída", matching the seed wording.

  The existing outbox (`case.opened`) and the SLA delay arm are unchanged, so escalation still uses the TTL queue.
- **Migration.** Add `migrations/cases/003_one_open_case.sql` with a partial unique index `ON cases (customer_id) WHERE state <> 'Resolvido'`. It is a database backstop to the lock; the seed cases must still pass it.
- **Consumer in `cmd/cases/main.go`.**
  - Queue `cases.message.triaged`, durable, bound to `message.triaged` on the events exchange.
  - Its own dead-letter queue `cases.message.triaged.dlq`: declare the DLQ and set `x-dead-letter-exchange` `""` and `x-dead-letter-routing-key` to the DLQ name on the main queue.
  - Explicit bound: `Qos` prefetch 1 with one handler goroutine.
  - Permanent errors (bad JSON, envelope validation, `ErrUnknownCustomer`, unusable customer): `Nack(requeue=false)`, which goes to the DLQ.
  - Transient errors: retried in-process with exponential backoff and jitter (from 200 ms, at most 5 attempts, stopping on context done). Once exhausted, `Nack(requeue=false)` sends the message to the DLQ.
  - The consumer runs only when `CASES_BROKER_URL` and `ADVISORY_GRPC_TARGET` are both set. If the broker is set but the advisory target is not, log a warn and run without intake.
  - The advisory connection is closed on shutdown. Shutdown stays graceful and follows the existing `errCh`/workers pattern.
- **Logs.** Info logs carry only event id, customer id, case id, and the decision (`opened`, `joined`, `ignored`). They never carry message text.
- **Complaint demo.** At least one complaint-panel preset (`web/src/screens/ClientAppScreen.tsx` `PRESETS`) must classify as `reclamacao` under the keyword heuristic (`internal/triage/heuristic.go`) as well as the model. If none does, add the minimal accent-free keyword(s) that make the "Uma cobrança que não reconheço" preset classify as `reclamacao`, with a heuristic test. Change nothing else in triage.
- **Docs.** Update `README.md` or the relevant spec only where the cases configuration or event consumers are documented. `ADVISORY_GRPC_TARGET` is already a root `.env` variable, so no env file changes.
- Wrap errors with `%w`; propagate `context.Context`.

**Never:**
- No `ListCases` customer filter, moment facts, BFF, or web change (story 8).
- No change to the SLA clock values, the `Advance`/`HandleBreach` behavior, or the event schemas.
- Account and suitability alerts open no case.
- Do not import `internal/triagepipe` or `internal/advisory` domain packages from `internal/cases`. Using the generated `gen/advisory/v1` client is fine.
- Unit tests do not dial 5435/5673/8400/8420/9201.
- Do not read or edit `.env` / `.env.*`. Never a `utils` package.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Mariana complaint | `message.triaged` intent `reclamacao`, Mariana (Singular), no open case | one case `Aberto`, advisor from advisory, SLA 60 min (30 when a clock factor halves it), `case.opened` in outbox, delay armed, history `caso` row | No error expected |
| Closing request | intent `encerramento` | case opened, history "pedido de encerramento" | — |
| Churn only | intent `investimento`, churn_risk 0.8 | case opened, `ChurnRisk` factor halves the SLA | — |
| Not qualifying | intent `cambio`, churn 0.2 | inbox claimed, no case, no outbox | — |
| Open case exists | second qualifying message, same customer, case in `Em atendimento` | no new case; one `mensagem` history row on the open case; no outbox row | — |
| Resolved case only | customer's only case is `Resolvido` | a new case opens | — |
| Duplicate delivery | same triaged `event_id` twice | second is a no-op | — |
| Advisory down | `GetCustomer` Unavailable / deadline | transient: backoff retries, then DLQ; no inbox row committed | not acked |
| Unknown customer | `GetCustomer` NotFound | permanent → DLQ | — |
| Bad body | invalid JSON or envelope | permanent → DLQ | — |
| Concurrent | two qualifying messages for one customer at once | exactly one open case | lock + unique index |

</intent-contract>

## Code Map

- `internal/cases/machine.go` -- `Open`, `OpenInput`, `Tx` (`ClaimInbox`, `InsertCase`, `InsertOutbox`, `ArmDelay`); refactor the open body into a tx-level helper; add intake.
- `internal/cases/clock.go` -- `ClockFactors`, `TotalMinutes`, `BaseMinutes`, `FrustrationFrustrado = 2`.
- `internal/cases/pgx.go` -- pgx `Tx`; add `LockCustomer`, `OpenCaseFor(customerID)`, `InsertHistory`.
- `internal/cases/machine_test.go` -- `memStore`/`memTx` fakes to extend.
- `migrations/cases/002_uuidv7.sql` -- `cases`, `case_history`, `inbox` schema (UUID columns).
- `seeds/cases/001_cast.sql` -- existing cast cases; Mariana (`01a0e3a4-9a44-7566-b5de-eb2e365799f8`) has none.
- `cmd/cases/main.go` -- AMQP dial, `declareSLATopology`, `runBreachConsumer`, worker/errCh shutdown pattern; add the triaged consumer and the advisory dial.
- `cmd/advisory/main.go:319` -- `runConsumer`/`handleDelivery`/`permanentDeliveryError` pattern to mirror.
- `internal/triagepipe/apply.go:43` -- `TriagedPayload` JSON field names (read-only reference).
- `internal/triage/heuristic.go`, `web/src/screens/ClientAppScreen.tsx:37` -- heuristic keywords and complaint presets.
- `gen/advisory/v1` -- `GetCustomer` client; `internal/bff/clients.go` `DialGRPC` shows the dial style.

## Tasks & Acceptance

**Execution:**
- `internal/cases/intake.go` -- `TriagedMessage`, `Qualifies`, `CustomerLookup` port, `Intake(ctx, store, lookup, body)` with the transaction above and typed permanent/transient errors.
- `internal/cases/advisory.go` (or `cmd/cases`) -- the `GetCustomer` adapter with the default deadline and `NotFound` mapping.
- `internal/cases/machine.go`, `internal/cases/pgx.go` -- tx-level open helper, lock, open-case lookup, history insert.
- `migrations/cases/003_one_open_case.sql` -- partial unique index.
- `cmd/cases/main.go` -- queue + DLQ topology, bounded consumer with backoff, advisory dial and close.
- `internal/cases/intake_test.go` -- every matrix row over the in-memory store, a fake lookup, and the adapter over bufconn with a stub advisory server (NotFound, Unavailable, deadline present).
- Gated pgx test (`CASES_TEST_DATABASE_URL`, skipped when unset): migrate 001–003, then open → join → resolve → reopen, and the concurrent pair yields one open case. Wire the variable into CI next to `ACCOUNT_SIM_TEST_DATABASE_URL` if the CI Postgres service can create the database cheaply. Otherwise record it as deferred.
- `internal/triage/heuristic.go` (+ test) -- only if the complaint-demo check fails.

**Acceptance Criteria:**
- Given cases, advisory, triage, and account-sim running locally, when Mariana files a complaint with a preset that triages as `reclamacao`, then one case appears for her in `ListCases` with her advisor and the Singular SLA, and a second complaint adds history instead of a second case.
- Given the change, when the Go suite runs, then all pass and no BFF, web, or event schema file changed.

## Verification

**Commands:**
- `gofmt -l . && go vet ./... && go build ./...` -- expected: clean.
- `mkdir -p .gotmp && GOTMPDIR=$PWD/.gotmp go test -race -shuffle=on ./...` -- expected: pass.
- `CASES_TEST_DATABASE_URL='postgres://localdev:localdev@127.0.0.1:5435/cases?sslmode=disable' GOTMPDIR=$PWD/.gotmp go test -race -count=1 ./internal/cases/` -- expected: pass against the local compose Postgres (use a scratch schema or a disposable database if the test resets tables).
- `go mod tidy && git diff --exit-code go.mod go.sum` -- expected: clean.

## Spec Change Log

### 2026-09-29 — Orchestrator resolution of an intent gap (client-app origin)
- Trigger: live smoke and review showed the account-sim cast seed republishes a Mariana churn message on every reseed, so intake gives her an open case at startup; SPEC.md requires she opens on `portfolio_review` after reseed and switches to `case_open` only after a complaint from the client app.
- Resolution (standing user instruction: do not block on small decisions): only messages sent from the client app open or join cases. `message.received` payload gains optional `origin` (`client_app` on POV message/complaint commands), triage copies it into `message.triaged`, and cases requires it in `Qualifies`. Cast and burst messages keep feeding the advisor queue and timeline and open no case.
- KEEP: the intent/churn rule, inbox, per-customer lock, unique index, DLQ topology, and retry helper are unchanged.

## Review Triage Log

### 2026-09-29 — Review pass
- verdicts: 35 findings — high 3, medium 14, low 13, false 5, maybe-false 0
- findings:
  - `[high]` `[patch]` blind: `breachEventID`/`statusEventID` are not UUIDs; intake-armed SLA delays make HandleBreach fail and requeue forever on Postgres — all case event ids are name-based UUIDs from the case id.
  - `[medium]` `[patch]` blind: `Open` lost idempotency (fresh v7 case.opened id) — deterministic `opened:<caseID>` UUID.
  - `[medium]` `[patch]` blind: lookup before the inbox claim turns duplicates into retries/DLQ when advisory is down — pre-read inbox and open case; lookup only when opening.
  - `[low]` `[reject]` blind: exhausted transient failures park in the DLQ with no redrive — the intent prescribes retries then DLQ; redrive tooling is outside this story.
  - `[low]` `[patch]` blind: setup errors after workers start skip the shutdown sequence — setup moved before workers start.
  - `[medium]` `[defer]` blind: advisor resolved by display name through ListOperators — `GetCustomer` should return the advisor id; story 8 already changes `advisory/v1` and now carries it.
  - `[false]` `[reject]` blind: `needs_review`/`degraded` do not gate opening — the intent qualifies on intent or churn only.
  - `[low]` `[reject]` blind: no trace context or gRPC interceptors on the new consumer — OpenTelemetry is story 17.
  - `[low]` `[patch]` blind: consumer ack/nack and ListOperators failure untested — outcome function table test and stub-server test added.
  - `[low]` `[patch]` blind: Ack/Nack errors discarded — logged at warn with the event id.
  - `[false]` `[reject]` blind: a joined message publishes nothing — the intent says join adds history and publishes nothing.
  - `[low]` `[reject]` blind: CI test URL reuses the `account_sim_test` database — each test uses its own schema; no service shares data at runtime.
  - `[low]` `[patch]` blind: complaint demo preset untested — heuristic test with the "Estou pensando em sair" preset text.
  - `[medium]` `[patch]` edge: duplicate delivery while advisory is down is dead-lettered — grouped with the lookup reorder.
  - `[medium]` `[patch]` edge: joining an open case requires the advisory lookup — grouped with the lookup reorder.
  - `[low]` `[patch]` edge: concurrent Advance to Resolvido lets a message join a resolved case — `OpenCaseFor` selects `FOR UPDATE`.
  - `[low]` `[reject]` edge: a backlog message opens a case whose TTL starts at now while `opened_at` is in the past — needs a replayed backlog; the fix changes the shared SLA arm path.
  - `[low]` `[patch]` edge: ListOperators errors all transient — same code mapping as GetCustomer.
  - `[low]` `[patch]` edge: early returns abandon workers — grouped with the setup reorder.
  - `[false]` `[reject]` edge: migration 003 aborts on existing duplicate open cases — nothing opened cases before this story and the seed has one case per customer.
  - `[low]` `[reject]` edge: fake `LockCustomer` ignores ctx and is not reentrant — test fake only; no test depends on either.
  - `[medium]` `[patch]` edge: removed deterministic case.opened id — grouped with the event id fix.
  - `[medium]` `[patch]` edge: join claim requires lookup — grouped with the lookup reorder.
  - `[low]` `[patch]` edge: one-case claim vs Advance race — grouped with `FOR UPDATE`.
  - `[medium]` `[patch]` verification-gap: `handleTriaged` ack/requeue/DLQ untested — pure `triagedOutcome` with a table test.
  - `[medium]` `[patch]` verification-gap: payload JSON tags only round-trip the consumer's own struct — literal producer-shaped JSON test.
  - `[high]` `[patch]` verification-gap other: breach path reachable and loops — grouped with the event id fix.
  - `[medium]` `[patch]` verification-gap other: `Open` not idempotent — grouped with the event id fix.
  - `[medium]` `[defer]` intent: advisor id from name lookup — grouped with the advisor-id deferral.
  - `[false]` `[reject]` intent: named preset "Uma cobrança…" does not classify — the intent requires at least one preset to; "Estou pensando em sair" does.
  - `[medium]` `[patch]` intent: lookup before claim — grouped with the lookup reorder.
  - `[medium]` `[patch]` intent: consumer wiring untested — grouped with the outcome test.
  - `[medium]` `[patch]` intent: `Open` event id changed — grouped with the event id fix.
  - `[false]` `[reject]` intent: DLQ warn log carries error text — the intent restricts Info logs; the error text carries no message text.
  - `[high]` `[patch]` orchestrator (live smoke): seeded Mariana churn message opens her case at every reseed — resolved as recorded in the Spec Change Log (client-app `origin`).

## Auto Run Result

Status: done

- **Summary:** cases consumes `message.triaged` on `cases.message.triaged` (DLQ `cases.message.triaged.dlq`, prefetch 1, in-process backoff retries). It opens one case per customer when a client-app message (`origin: client_app`) is `reclamacao` or `encerramento`, or has churn risk ≥ 0.5. Segment and advisor come from advisory `GetCustomer` under a 2 s deadline. A later qualifying message joins the open case's history. Duplicates and joins need no advisory call. Case event ids are now name-based UUIDs, which fixes `Advance` and `HandleBreach` on PostgreSQL and makes `Open` idempotent.
- **Files:**
  - `internal/cases/intake.go`: rule, port, and one-transaction intake.
  - `internal/cases/advisory.go`: gRPC lookup adapter.
  - `internal/cases/{machine,pgx}.go`: tx-level open, lock, `FOR UPDATE` open-case read, history, deterministic event ids.
  - `migrations/cases/003_one_open_case.sql`: partial unique index.
  - `cmd/cases/main.go`: topology, consumer, setup before workers, and a pure outcome function.
  - `internal/sim/{account,burst}.go` and `internal/triagepipe/apply.go`: the `origin` field.
  - Tests: `internal/cases/*_test.go`, `cmd/cases/main_test.go`, `internal/triage/triage_test.go`, `internal/triagepipe/apply_test.go`, `internal/sim/grpc_test.go`.
  - `.github/workflows/ci.yml`: `CASES_TEST_DATABASE_URL`.
  - Docs: `README.md`, `specs/adr/0008-client-command-grpc-outbox.md`, `specs/http/bff.md`.
- **Review:** 35 findings.
  - 21 patched: 3 high, 11 medium, 7 low, including the orchestrator-resolved seed/origin gap.
  - 2 deferred: advisor id by name, carried into story 8.
  - 12 rejected with evidence, 5 of them false.
- **Follow-up review recommended:** true. High patches changed event ids on every case event and added the origin gate across account-sim, triage and cases. Nobody has yet checked whether downstream consumers (BFF, timeline) depend on the old id shape.
- **Verification:**
  - gofmt, vet and build are clean, and tidy is clean.
  - `go test -race -shuffle=on ./...` passes.
  - The gated cases and account-sim pgx tests pass against local Postgres.
  - `pnpm --dir web test` passes 143/143.
  - Live smoke before the review: Mariana got one case with the Singular SLA halved by churn to 30 min, a second complaint joined it, and a bad body reached the DLQ.
- **Residual risks:**
  - Messages left in the DLQ have no redrive tooling.
  - A backlog message opens a case whose TTL starts at processing time.
