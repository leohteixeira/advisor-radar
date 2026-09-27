---
title: 'Raise deterministic alerts from the book'
type: 'feature'
created: '2026-09-27'
status: 'done'
review_loop_iteration: 0
baseline_revision: 'ebc078cc534a800ea05a50f9ff2ef4e1f9d34208'
followup_review_recommended: true
context: []
warnings:
  - oversized
deferred:
  - summary: >-
      The PostgreSQL advisory adapter and the AMQP connection are not exercised by go test.
    evidence: |-
      CI has no Postgres and no RabbitMQ. The story forbids dialing ports 5435 and 5673 from the default test gate. memStore covers the matrix. Dropping a SQL statement or the dial would not fail go test.
    location: >-
      internal/advisory/pgx.go
    severity: medium
---

<intent-contract>

## Intent

**Problem:** Account facts sit in the seed book, but nothing turns a withdrawal, a drop, a deposit, a segment crossing, or a long silence into a named `alert.raised` event.

**Approach:** Advisory stores the 22-client book and applies the five architecture rules. Each firing rule writes `alert.raised` through an idempotent inbox and a transactional outbox.

## Boundaries & Constraints

**Always:** Compare with strict greater-than. Withdrawal fires when `amount / before > 0.20`. Asset drop fires when `abs(amount) / before > 0.15`. Large deposit fires when `amount > before`. Segment fires only when assets cross USD 10,000 or USD 200,000. Essencial is up to 10,000 inclusive, Advance is above 10,000 through 200,000 inclusive, Singular is above 200,000. Silence fires when recorded days with no contact are greater than 90. One account fact may emit two alerts. Inbox key is the source `event_id`; a second delivery adds no alert. The alert row and the outbox row commit in one transaction. Publish marks the row only after the broker accepts it. Alert ids for the seed are `al-s11` through `al-s17`. A live account event uses `al-` + source `event_id` + `-` + rule key (`withdrawal`, `drop`, `deposit`, `segment`). Card kinds are `saque`, `queda`, `aporte`, `segmento`, `contato`. Rule strings are the Portuguese sentences on SIGNALS s11–s15. `go test` must not dial port 5435 or 5673. With `ADVISORY_DATABASE_URL` unset, the process only logs `service=advisory` and waits for a signal. Wrap errors with `%w`. Pass `context.Context` into I/O.

**Never:** Do not raise "Mensagem com risco" or consume `message.triaged`. Do not add gRPC, HTTP, cases, or a timeline. Do not share the `account_sim` database. Do not add a cron. Do not change `internal/outbox.Fire` or make that package import advisory. Do not read or write `.env`. Do not log customer text.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Seed book | Empty store, then `RaiseSeed` twice | Seven `alert.raised` rows: `al-s11` saque, `al-s12` queda, `al-s13` aporte, `al-s14` segmento, `al-s15` contato, `al-s16` saque, `al-s17` queda. Second call stays at seven | No error expected |
| Crossing deposit | c13 deposit 60000, before 8000, after 68000 | Two alerts: aporte and segmento, from Essencial to Advance | No error expected |
| Exact withdrawal | amount 20000, before 100000 | No alert | No error expected |
| Exact drop | amount -15000, before 100000 | No alert | No error expected |
| Equal deposit | amount 8000, before 8000 | No alert | No error expected |
| Same band | before 50000, after 60000 | No segment alert | No error expected |
| Silence boundary | 90 days, then 94 days on c14 | No alert, then one `contato` alert `al-s15` | No error expected |
| Redelivery | Same account `event_id` applied twice | One alert | No error expected |
| Broker refuses | Unpublished alert, broker returns an error | Row stays unpublished; a later accept sends it once | Returned error wraps the broker error |
| Rollback | Insert fails inside the transaction | No inbox row, no alert, no outbox row | Returned error wraps the insert error |
| Market day | `md-n02` withdrawal 55000/196000/141000 and `md-n05` drop -12000/79000/67000 | `al-md-n02-withdrawal` saque and `al-md-n05-drop` queda | No error expected |
| Message | `message.received` | No alert and no inbox row | No error expected |

</intent-contract>

## Code Map

- `docs/design/mock-data.js` lines 37–81 — `CLIENTS` is the book (22 ids, segment, aum). `SIGNALS` s11–s17 are the seven seed alerts. s13 and s14 are one c13 deposit. s15 is silence, 94 days, not an account event.
- `docs/design/mock-data.js` lines 84–91 — `STREAM` n02 and n05 are account facts already published by account-sim. They are not seed alerts.
- `internal/sim/burst.go` — `AccountPayload` JSON is `kind`, `amount`, `before`, `after`. Kinds in the burst are `withdrawal` and `asset_drop`.
- `internal/event/event.go` — body fields plus optional payload. Name `alert.raised` is the routing key (`json:"-"`).
- `internal/outbox/outbox.go` — account-sim only. Copy the publish-after-ack behavior; do not call `Fire`.
- `specs/adr/0002-transactional-outbox-idempotent-inbox.md` — outbox shares the local write transaction; inbox is keyed by `event_id`.
- `cmd/advisory/main.go` — today only `proc.Run`. `internal/proc/commands_test.go` requires a JSON log line with `service=advisory` and exit 0 on SIGINT and SIGTERM when no database URL is set.
- `deploy/postgres/init.sql` — database `advisory` already exists. Add tables in a new migration, not by editing the database list.
- CI has no Postgres and no RabbitMQ. Adapters exist; tests use fakes.

## Tasks & Acceptance

**Execution:**
- `internal/book/book.go` — 22 clients and segment-from-assets — the seed book and the two bounds
- `internal/book/book_test.go` — every client segment matches the bound for that aum
- `internal/advisory/rules.go` — five pure rules and `RaiseSeed` inputs copied from s11–s17 — one fact can return two decisions
- `internal/advisory/apply.go` — `Apply` and `RaiseSeed` behind store interfaces declared here — inbox, alert, and outbox in one transaction
- `internal/advisory/advisory_test.go` — one test per matrix row, with a fake store and a fake broker
- `internal/advisory/pgx.go` — PostgreSQL adapter for database `advisory` using `github.com/jackc/pgx/v5` — parameterized SQL, context on every call, same DDL as the migration
- `migrations/advisory/001_book.sql` — book, inbox (`event_id` unique), alerts, outbox (`event_id` unique, `published_at` null until ack)
- `cmd/advisory/main.go` — log `service=advisory`, seed and publish only when `ADVISORY_DATABASE_URL` is set, consume `account.event.recorded` only when `ADVISORY_BROKER_URL` is set — CI stays offline
- `.env.example` — local advisory database and broker URLs, placeholder password only

**Acceptance Criteria:**
- Given the 22 clients, when each segment is compared with its aum, then it matches Essencial, Advance, or Singular as defined above.
- Given `ADVISORY_DATABASE_URL` is unset, when the advisory process receives SIGTERM, then it exits 0 and has logged `service=advisory`.

## Spec Change Log

## Review Triage Log

### 2026-09-27 — Review pass
- verdicts: 29 findings — high 0, medium 16, low 7, false 6, maybe-false 0
- findings:
  - `[medium]` `[patch]` Poison deliveries were requeued forever — decode and validate failures now Nack with requeue false.
  - `[low]` `[reject]` RunPublisher does not log swallowed publish errors — the row stays unpublished and the next tick retries. A log line is optional and must not include payloads.
  - `[medium]` `[patch]` One worker returning left the other on a connection that defers then close — run now cancels and waits for both goroutines.
  - `[medium]` `[patch]` AMQP dial had no timeout — dial uses a 10 second timeout and returns when the context is already canceled.
  - `[low]` `[reject]` Rules are functions, not one interface each — the five functions are the rules. Extra interfaces would not change which alerts fire.
  - `[false]` `[reject]` No test crosses USD 200,000 — `ruleSegment` uses `SegmentFromAssets`, which already splits at 10,000 and 200,000. The c13 case runs that same branch.
  - `[medium]` `[patch]` handleDelivery skipped `Validate` — invalid envelopes, including an empty event id, are rejected before Apply.
  - `[low]` `[reject]` The story Code Map still says main only calls proc.Run — that section is the planning snapshot. This result records what shipped. The fix would only edit the spec.
  - `[low]` `[reject]` Silence days are not a book column — RaiseSeed still emits `al-s15` from the seed fact, the same way withdrawal amounts are seed inputs.
  - `[medium]` `[patch]` Seed publish and rollback tests were count-only — tests now check `alert.raised`, the Portuguese rule strings, and `errors.Is` on the forced insert error.
  - `[medium]` `[patch]` Edge case: poison requeue — same Nack change as the first row.
  - `[medium]` `[patch]` Edge case: sibling worker not drained — same cancel-and-wait change.
  - `[medium]` `[patch]` Edge case: dial can block — same dial timeout.
  - `[medium]` `[patch]` A closed deliveries channel returned nil — the consumer now returns an error when the channel closes before cancel.
  - `[low]` `[reject]` NaN withdrawal amounts can raise an alert — JSON cannot carry NaN. A numeric guard would be a new branch no delivery can hit.
  - `[low]` `[reject]` NaN drop amounts can raise an alert — same JSON limit as the withdrawal case.
  - `[medium]` `[patch]` Empty event id can be claimed — covered by `Validate` before Apply.
  - `[medium]` `[patch]` RunPublisher had no test — `TestRunPublisher_RetriesThenStops` covers refuse, recovery, and cancel.
  - `[medium]` `[patch]` Outbox routing key and rule text were not asserted — the seed publish test now checks both.
  - `[medium]` `[patch]` Live JSON map payloads were untested — `TestApply_MarketDayJSONMap` unmarshals the market-day account facts into a map.
  - `[medium]` `[patch]` Verification note that bad bodies requeue — permanent decode errors no longer requeue.
  - `[false]` `[reject]` The 24 hour and 5 business day windows are not a ledger — the seed facts are one before/after pair each. The rule text names the window; the comparison is the ratio in the Always rules.
  - `[low]` `[reject]` Apply does not rewrite the book row — rules use the event's before and after. The stored book is the current 22-client snapshot.
  - `[medium]` `[defer]` PostgreSQL SQL is not executed by go test — recorded in deferred. The default gate must not dial Postgres.
  - `[medium]` `[defer]` The AMQP dial is not executed by go test — same deferred gap. The default gate must not dial RabbitMQ.
  - `[false]` `[reject]` SIGTERM logging was not retested in this diff — `TestCommands_SignalStop` still requires `service=advisory` and it passed.
  - `[false]` `[reject]` gRPC, a dead-letter queue, and per-rule interfaces are absent — the story sentence is the book, the five rules, `alert.raised`, the inbox, and the outbox.
  - `[false]` `[reject]` Raised alerts are not on the advisor queue — showing the queue is a later story. This story publishes `alert.raised`.
  - `[false]` `[reject]` Silence is not consumed from AMQP — the seed silence fact is book state, not an `account.event.recorded` payload.

## Design Notes

Silence is book state (`days` on c14), not an `account.event.recorded` payload. The other six seed alerts are account facts. n05 is a drop of 12000/79000, which is over 15%, so the rule fires even though the card text rounds the reason to 15%.

Seed alert ids stay `al-s11` … `al-s17` so a restarted seed does not mint new ids. Live ids append the rule key because c13 shows one fact can raise two alerts.

## Verification

**Commands:**
- `gofmt -l .` -- expected: empty
- `go mod tidy && git diff --exit-code go.mod go.sum` -- expected: clean
- `go vet ./...` -- expected: exit 0
- `go build ./...` -- expected: exit 0
- `mkdir -p .gotmp && GOTMPDIR="$PWD/.gotmp" go test -race -shuffle=on ./...` -- expected: pass

## Auto Run Result

Status: done

Summary: Advisory stores the 22-client book, applies the five rules to the seed and to `account.event.recorded`, and publishes each firing rule as `alert.raised` through an idempotent inbox and a transactional outbox.

Files changed:
- `internal/book` — 22 clients and segment bounds
- `internal/advisory` — five rules, RaiseSeed, Apply, publish-after-ack, pgx adapter
- `migrations/advisory/001_book.sql` — book, inbox, alerts, outbox
- `cmd/advisory/main.go` — seed when the database URL is set, then consume and publish until shutdown
- `.env.example` — local advisory database and broker URLs

Review: eight medium patches (permanent Nack, worker shutdown, dial timeout, closed-channel error, Validate, publisher retry test, rule and routing-key assertions, JSON map payload). Deferred: real Postgres SQL and the AMQP dial. Rejected findings are in the triage log.

Follow-up review recommended: true. Patched medium entries: 8. Unverified risk: `PGXStore` and the AMQP dial are not executed by `go test`.

Verification: gofmt clean, go vet, go build, and `go test -race -shuffle=on ./...` passed after the patches. `go mod tidy` left `go.mod` and `go.sum` unchanged.
