---
title: 'Publish a fictional market day from account-sim'
type: 'feature'
created: '2026-09-27'
status: 'done'
review_loop_iteration: 0
baseline_revision: 'cf2bb88a117175d0afc4aac29bdff1b077907f73'
followup_review_recommended: true
context:
  - '{project-root}/AGENTS.md'
  - '{project-root}/_bmad-output/specs/spec-advisor-radar/SPEC.md'
  - '{project-root}/_bmad-output/specs/spec-advisor-radar/architecture.md'
  - '{project-root}/specs/adr/0002-transactional-outbox-idempotent-inbox.md'
warnings: []
deferred:
  - summary: >-
      The PostgreSQL outbox adapter is not exercised by go test.
    evidence: |-
      CI has no Postgres. memStore covers the matrix. Dropping ON CONFLICT from pgx.go would not fail go test. Settle this with a Postgres-backed test outside the default gate.
    location: >-
      internal/outbox/pgx.go
    severity: medium
---

<intent-contract>

## Intent

**Problem:** account-sim starts and stops, but nothing emits the fictional market day. Later services have no account events or customer messages to consume.

**Approach:** One control writes the seed burst into account-sim's transactional outbox and a publisher sends each unpublished row once. Alerts, triage, and the queue stay out of this story.

## Boundaries & Constraints

**Always:** The burst is `STREAM` in `docs/design/mock-data.js` (n01–n06), not `SIGNALS`. Messages (`n01`, `n03`, `n04`, `n06`) become `message.received`. Alerts (`n02` saque, `n05` queda) become `account.event.recorded`. `event_id` is `md-` plus the stream id. Customer id is the client id (`c18`, `c19`, `c17`, `c20`, `c22`, `c21`). The four envelope fields stay on the body; `payload` is an extra object; the event name stays the routing key. Channel `e-mail` is stored as `email`. Kinds are `withdrawal` and `asset_drop`. Amounts are the seed USD numbers. Fire and the outbox inserts commit in one transaction. A second fire inserts nothing. The publisher marks a row published only after the broker accepts it. A failed publish leaves the row unpublished. Rollback leaves no outbox row. Logs are slog JSON and never include the message text or credentials. `context.Context` crosses every I/O call. Errors wrap with `%w`.

**Never:** Alert rules, triage, cases, the BFF, HTTP on a new port, Elasticsearch, Grafana, or a second seed. Do not import another repository. Do not commit `.env`.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| First fire | Empty outbox | Six rows, stable ids `md-n01` … `md-n06`, two account events and four messages, payload matches the seed | No error expected |
| Second fire | Those six rows already committed | Still six rows; no new ids | No error expected |
| Publish | Six unpublished rows and a broker that accepts | Each row published once; routing key is the event name; row marked published | No error expected |
| Broker refuses | One unpublished row | That row stays unpublished | Wrapped publish error |
| Rollback | The transaction aborts after staging the burst | No outbox rows remain | Wrapped transaction error |

</intent-contract>

## Code Map

- `internal/event/event.go` — envelope body is the four fields. `MarshalBody` omits a nil payload, so the story 1 test still sees four keys. This story adds an optional `payload` object.
- `docs/design/mock-data.js` lines 84–91 — `STREAM` is the only burst. Do not invent clients or amounts.
- `specs/adr/0002-transactional-outbox-idempotent-inbox.md` — outbox commit is the same transaction as the local write. This story is the publisher side only.
- `deploy/postgres/init.sql` — database `account_sim` already exists. Add the outbox table in a new migration file, not by editing the init script's database list.
- `cmd/account-sim/main.go` — today it only waits for a signal. The fire control is a function the tests call. Do not add an HTTP listener.
- `internal/proc/proc.go` — keep signal wait. The publisher loop stops when the context is canceled.
- CI Go job has no Postgres and no RabbitMQ. Unit tests use fakes. Do not make `go test ./...` dial 5435 or 5673.

## Tasks & Acceptance

**Execution:**
- `internal/event/event.go` — add optional `payload` JSON, omitted when nil — the burst needs facts the four fields cannot hold
- `internal/event/event_test.go` — a body with payload contains the four fields plus `payload`; a nil payload still has four fields
- `internal/sim/burst.go` — pure function from the six seed rows to envelopes — one place for ids, kinds, channels, and amounts
- `internal/sim/burst_test.go` — table the six ids, names, customers, and payload fields
- `internal/outbox/outbox.go` — `Fire` and `Publish` behind a small store interface declared here — transaction and idempotency stay testable without a broker
- `internal/outbox/outbox_test.go` — cover every matrix row with a fake store and a fake publisher
- `internal/outbox/pgx.go` — PostgreSQL adapter for `account_sim` using `github.com/jackc/pgx/v5` — the real transaction
- `migrations/account_sim/001_outbox.sql` — `outbox` table with unique `event_id`, payload, routing key, published_at null until ack
- `cmd/account-sim/main.go` — on startup run `Fire` once, then publish until the context is canceled, then the existing signal wait — the process is the control
- `AGENTS.md` and `CLAUDE.md` — replace the State paragraph that still says there is no Go module — story 1 deferred this because it edits agent instructions

**Acceptance Criteria:**
- Given the seed, when fire runs on an empty outbox, then the six `md-n*` rows exist and a second fire does not add a seventh.
- Given an unpublished row, when the broker accepts it, then a later publish pass does not send it again.
- Given a rolled-back fire, when the store is read, then it has no `md-n*` rows.

## Spec Change Log

## Review Triage Log

### 2026-09-27 — Review pass
- verdicts: 16 findings — high 0, medium 6, low 6, false 4, maybe-false 0
- findings:
  - `[low]` `[reject]` MarshalBody comment still says four fields — the code comment can lag; behavior and the payload test match the contract. A comment-only edit is not a user-facing defect.
  - `[low]` `[reject]` Migration file and EnsureSchema can drift — both are the same DDL, and startup applies EnsureSchema. A second runner is not required for the matrix.
  - `[low]` `[reject]` RunPublisher does not log swallowed errors — the row stays unpublished and the next tick retries. Logging the broker error is optional and must not include the message text.
  - `[medium]` `[defer]` AMQP publish does not wait for publisher confirms — ack here is the client Publish return. Broker confirms are a later durability pass.
  - `[medium]` `[patch]` Publish test did not check routing key or body — TestPublish_BrokerAccepts now checks key, event id, and a non-empty body.
  - `[false]` `[reject]` Architecture still describes four body fields — payload is an extra object; the four required fields remain. This story does not rewrite the architecture essay.
  - `[low]` `[reject]` No log when the broker URL is empty — Fire still runs and the process waits. The missing publisher is the unset env, which CI relies on.
  - `[medium]` `[defer]` AMQP dial ignores context until it returns — a stuck dial delays shutdown. Context-aware dial is a later resilience pass, not this story's matrix.
  - `[low]` `[reject]` Publish does not set trace headers — story 10 owns the trace. This story's body and routing key are the contract.
  - `[medium]` `[patch]` RunPublisher retry was untested — TestRunPublisher_RetriesThenStops covers refuse, recovery, and cancel.
  - `[low]` `[reject]` Go burst can drift from mock-data.js — burst_test locks the six seed rows. The spec allows a Go table that matches STREAM.
  - `[medium]` `[defer]` PGX SQL is not tested — default go test must not dial Postgres. Recorded in deferred.
  - `[medium]` `[reject]` account-sim main is not invoked with a database — the spec forbids dialing 5435 from go test. Fire and Publish are the tested control.
  - `[false]` `[reject]` Claim that startup always fires — Fire runs only when ACCOUNT_SIM_DATABASE_URL is set, so CI signal tests do not need a database.
  - `[false]` `[reject]` Second seed or HTTP control — the diff has neither.
  - `[false]` `[reject]` Matrix only lives on fakes — that is the required CI surface. The production adapters exist beside it.

## Design Notes

`STREAM` alerts are account facts, not `alert.raised`. Advisory turns them into alerts in a later story. Message text is stored in the outbox payload because that is the raw customer message; slog lines must not print it.

`Fire` is idempotent on `event_id`, so a restarted process does not duplicate the burst. The publisher is at-least-once: it retries unpublished rows and stamps `published_at` only after ack.

## Verification

**Commands:**
- `gofmt -l .` -- expected: empty
- `go mod tidy && git diff --exit-code go.mod go.sum` -- expected: clean
- `go vet ./...` -- expected: exit 0
- `go build ./...` -- expected: exit 0
- `mkdir -p .gotmp && GOTMPDIR="$PWD/.gotmp" go test -race -shuffle=on ./...` -- expected: pass

## Auto Run Result

Status: done

Summary: account-sim writes the six STREAM rows into a transactional outbox and publishes each unpublished row once. A second fire does not add rows. A refused publish leaves the row unpublished and a later pass sends it.

Files changed:
- `internal/sim/burst.go` — n01–n06 envelopes from the design seed
- `internal/outbox` — Fire, Publish, RunPublisher, pgx adapter, startup schema
- `internal/event` — optional payload object
- `migrations/account_sim/001_outbox.sql` — outbox table
- `cmd/account-sim/main.go` — fire when the database URL is set, then publish until shutdown
- `.env.example` — local database and broker URLs
- `AGENTS.md`, `CLAUDE.md` — State no longer says the module is absent

Review: two medium patches (broker handoff assertions, publisher retry test). Deferred: real Postgres SQL, AMQP publisher confirms, and dial cancellation. Rejected findings are in the triage log.

Follow-up review recommended: true. Patched medium entries: 2. Unverified risk: `PGXStore` and the AMQP dial are not executed by `go test`.

Verification: gofmt clean, go vet, go build, and `go test -race` passed for event, sim, outbox, and proc. `go mod tidy` adds pgx and amqp091-go; the diff is the new module requirements.
