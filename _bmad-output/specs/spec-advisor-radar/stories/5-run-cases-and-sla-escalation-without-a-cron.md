---
title: 'Run cases and SLA escalation without a cron'
type: 'feature'
created: '2026-09-27'
status: 'done'
review_loop_iteration: 0
baseline_revision: '2f99709fc333c2b38c5c7ada63ca8c372e3b678e'
followup_review_recommended: true
context: []
warnings: []
deferred:
  - summary: >-
      PGXStore SQL for cases, inbox, outbox, and sla_delay is not executed by go test.
    evidence: |-
      Machine tests use memStore. The story forbids dialing port 5435.
      A wrong INSERT in ArmDelay would leave go test green.
    location: >-
      internal/cases/pgx.go
    severity: medium
  - summary: >-
      The live AMQP delay queue, per-message Expiration, and dead-letter bind are not executed by go test.
    evidence: |-
      SLADelayArgs and PublishDelay are covered with fakes. The story forbids dialing port 5673.
      A wrong queue argument or Expiration field would leave go test green.
    location: >-
      cmd/cases/main.go
    severity: medium
---

<intent-contract>

## Intent

**Problem:** A case cannot move through its states, and an expired SLA has nowhere to escalate except a clock that would have to be polled.

**Approach:** Cases stores the four forward states and arms each SLA as a queue message with TTL and a dead-letter exchange. Expiry emits `case.sla.breached` and sets the escalated flag. There is no cron.

## Boundaries & Constraints

**Always:** States in order are Aberto, Em atendimento, Aguardando cliente, Resolvido. Open emits `case.opened` and lands in Aberto. Each advance emits `case.status.changed`. A backward move is rejected and the state stays. Escalation is a flag, not a fifth state. Base minutes are Essencial 1440, Advance 240, Singular 60. Halve that total when any of these is true: churn risk, frustration at Frustrado or Muito frustrado (score at least 2), human requested, relevant withdrawal. Seed clocks are an Advance complaint at 120 minutes, an Advance exit risk at 120, and a Singular investment question at 60. Display is No prazo while remaining is at least `max(33% of total, 20 minutes)`, Vencendo while remaining is below that and still positive, and Vencido at zero or less. The delay is a message TTL in milliseconds. The dead-letter routing key is `case.sla.breached`. Handling that delivery sets escalated, keeps the current state, and writes one outbox row. A second delivery does not write another row. The case row, the outbox row, and the delay arm commit in one transaction. Publish marks the outbox row only after the broker accepts it. With `CASES_DATABASE_URL` unset, the process only logs `service=cases` and waits for a signal. `go test` must not dial port 5435 or 5673. Wrap errors with `%w`. Pass `context.Context` into I/O.

**Never:** Do not add a cron, a ticker, or a scanner that looks for expired rows. Do not add gRPC, the BFF, or the advisor queue. Do not share another service's database. Do not edit `internal/advisory`, `internal/triage`, or `internal/outbox`. Do not read or write `.env`. Do not log customer text.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Open | New case for c02 | State Aberto, one `case.opened`, delay TTL 120 minutes in milliseconds | No error expected |
| Advance | Aberto, then Em atendimento, then Aguardando cliente, then Resolvido | Three `case.status.changed` events and the four states in that order | No error expected |
| Backward | Em atendimento asked to become Aberto | State stays Em atendimento | Returned error |
| Past end | Resolvido advanced again | State stays Resolvido | Returned error |
| Clocks | Essencial, Advance, Singular, then the same Advance with frustration 2 | 1440, 240, 60, then 120 | No error expected |
| Withdrawal half | Singular plus relevant withdrawal | 30 minutes | No error expected |
| Display | 120 minute clock with 100, 30, and 0 minutes left | No prazo, Vencendo, Vencido | No error expected |
| Seed | Empty store, then the three seed cases | k1042 Em atendimento not escalated, k1038 Aguardando cliente escalated, k1031 Resolvido not escalated. A second seed adds nothing | No error expected |
| Expiry | Delay for an open case is released once, then again | One `case.sla.breached`, escalated true, state unchanged. The second release adds no row | No error expected |
| Queue args | SLA of 60 minutes | TTL 3600000 and dead-letter routing key `case.sla.breached`. No cron symbol in the cases process | No error expected |
| Broker refuses | Unpublished breach, broker returns an error | Row stays unpublished. A later accept sends it once | Returned error wraps the broker error |
| Rollback | Insert fails inside the transaction | No case, no outbox row, no armed delay | Returned error wraps the insert error |

</intent-contract>

## Code Map

- `_bmad-output/specs/spec-advisor-radar/state-machines.md` — forward states, halved clock, display bands, TTL plus dead-letter, no cron.
- `docs/design/mock-data.js` lines 104–123 — `CASE_STATES` and the three seed cases k1042, k1038, k1031. State index 0 is Aberto.
- `docs/design/mock-data.js` lines 37–60 — c02 and c07 are Advance, c09 is Singular.
- `internal/event/event.go` — `case.opened`, `case.status.changed`, and `case.sla.breached` already exist. The name is the routing key.
- `internal/triagepipe/apply.go` — pattern for inbox, local row, and outbox in one transaction. Do not edit that package.
- `cmd/cases/main.go` — today only `proc.Run`. `internal/proc/commands_test.go` requires a JSON log with `service=cases` and exit 0 on SIGINT and SIGTERM when no database URL is set.
- `deploy/postgres/init.sql` — database `cases` already exists.
- `specs/adr/0003-sla-escalation-by-ttl.md` — the accepted decision. Do not add a second ADR.

## Tasks & Acceptance

**Execution:**
- `internal/cases/clock.go` — base minutes, halve, and display band — pure functions
- `internal/cases/clock_test.go` — the clock and display rows
- `internal/cases/machine.go` — open, advance, seed, and breach behind store interfaces declared here — one transaction, no scanner
- `internal/cases/machine_test.go` — one test per remaining matrix row, with a fake store and a fake broker
- `internal/cases/queue.go` — TTL and dead-letter arguments for one SLA — the broker adapter consumes this
- `internal/cases/pgx.go` — PostgreSQL adapter for database `cases` using `github.com/jackc/pgx/v5`
- `migrations/cases/001_cases.sql` — cases, inbox, outbox
- `cmd/cases/main.go` — log `service=cases`, seed only when `CASES_DATABASE_URL` is set, consume the dead-letter queue only when `CASES_BROKER_URL` is set
- `.env.example` — local cases database and broker URLs

**Acceptance Criteria:**
- Given a new Advance complaint, when it is opened, then the state is Aberto, `case.opened` is stored, and the delay TTL is 120 minutes expressed in milliseconds.
- Given `CASES_DATABASE_URL` is unset, when the cases process receives SIGTERM, then it exits 0 and has logged `service=cases`.

## Spec Change Log

## Review Triage Log

### 2026-09-27 — Review pass
- verdicts: 35 findings — high 0, medium 10, low 12, false 13, maybe-false 0
- findings:
  - `[false]` `[reject]` `RunPublisher` ticker is an expiry cron — the ticker polls unpublished outbox and delay rows, the same pattern as account-sim, advisory, and triage. It does not scan expired case rows. The Never clause targets a clock that looks for expired rows.
  - `[low]` `[reject]` missing case id requeues forever — a delay is armed in the same transaction as the case insert, and there is no delete path, so a TTL body from this process names an existing case. A permanent-error guard would be new.
  - `[low]` `[reject]` JSON breach fallback treats `event_id` as the case id — `PublishDelays` sends the plain case id, which never enters that branch. Closing the fallback would be a new guard.
  - `[low]` `[reject]` unknown segment arms a zero TTL — the matrix uses Essencial, Advance, and Singular. Rejecting other segments would add a guard the matrix does not show.
  - `[false]` `[reject]` display bands are not wired into the process — the Display matrix row is the pure `DisplayBand` function, and `TestDisplayBand` covers 100, 30, and 0.
  - `[medium]` `[patch]` `ChurnRisk` and `HumanRequested` were untested — `TestTotalMinutes_SegmentsAndHalve` now expects 120 for Advance plus each factor.
  - `[medium]` `[patch]` `RunPublisher` never ran the delay broker — `TestRunPublisher_RetriesThenStops` uses one fake for `Publish` and `PublishDelay`, keeps a refusal unpublished, sends once after accept, and returns nil on cancel.
  - `[false]` `[reject]` `memTx.ClaimInbox` races on `store.inbox` — each test owns its store and does not call `WithTx` concurrently. `go test -race` passed. Production uses pgx.
  - `[low]` `[reject]` `EnsureSchema` and the migration are two DDL copies — same split as the earlier services. The migration comment drift is cosmetic.
  - `[low]` `[reject]` breach ack and nack are not logged — logging the body would record customer text. Sibling consumers also skip that log.
  - `[low]` `[reject]` AMQP publish has no trace headers — that belongs to the excluded observability story and would add a new surface.
  - `[false]` `[reject]` cases does not consume `alert.raised` — this story's process seeds, publishes delays, and handles `case.sla.breached`. Opening from alerts is not in the matrix.
  - `[low]` `[reject]` `Open` accepts an unknown segment — same claim as the zero-TTL finding. Everyday callers pass book segments.
  - `[false]` `[reject]` a second `Open` of the same id adds another delay — pgx `InsertCase`, outbox `event_id`, and `sla_delay.case_id` are all `ON CONFLICT DO NOTHING`.
  - `[low]` `[reject]` a breach for a missing case requeues forever — same unreachable path as the blind finding. No delete, and the arm commits with the case.
  - `[low]` `[reject]` ack or nack errors are ignored — returning those errors would add control flow the sibling consumers do not have.
  - `[false]` `[reject]` `MarkDelayPublished` with zero rows republishes — a zero-row update means the delay is already marked. A duplicate delay still escalates once, because `HandleBreach` claims the inbox.
  - `[false]` `[reject]` a failed mark after broker accept republishes the outbox row — that is the at-least-once rule. The consumer inbox drops the second delivery.
  - `[low]` `[reject]` an AMQP disconnect leaves the loops hanging — reconnect is a new surface. The earlier services do not reconnect either.
  - `[false]` `[reject]` concurrent `ClaimInbox` on the fake store races — same test-only claim. The race suite did not fail.
  - `[medium]` `[patch]` `RunPublisher` delay path had no test — same fix as the machine test above.
  - `[medium]` `[patch]` plain case-id breach bodies were untested — decode moved to `ApplyBreachDelivery`. `TestApplyBreachDelivery_PlainAndJSON` escalates once for a plain id and once for JSON `payload.case_id`.
  - `[medium]` `[patch]` halve factors `ChurnRisk` and `HumanRequested` had no rows — same clock-test fix as above.
  - `[medium]` `[defer]` `PGXStore` SQL never runs under `go test` — recorded in `deferred`. The story forbids dialing 5435.
  - `[low]` `[reject]` the anti-cron test only searches for the substring `cron` — the matrix asks for no cron symbol. The ticker is the outbox poll, not an expiry scanner.
  - `[medium]` `[patch]` breach decode was hard-wired to `*PGXStore` — `ApplyBreachDelivery` takes `Store`, so the plain and JSON bodies are tested on `memStore`.
  - `[false]` `[reject]` the publisher ticker violates the Never — reading that attaches "looks for expired rows" to cron, ticker, and scanner. The code polls unpublished rows only.
  - `[false]` `[reject]` the delay arm should be the broker message inside the transaction — Always says the arm commits with the case and the outbox, and publish marks the row only after the broker accepts. The AMQP send is after commit.
  - `[medium]` `[defer]` the live delay-queue topology is untested — recorded in `deferred`. The story forbids dialing 5673.
  - `[medium]` `[patch]` expiry was only `HandleBreach`, not the dead-letter body — `ApplyBreachDelivery` now covers the plain body `PublishDelays` sends and the JSON envelope.
  - `[false]` `[reject]` the open test assumes frustration 3 to reach 120 minutes — an Advance complaint at 120 is the halved clock. The factor is the input that produces the matrix total.
  - `[false]` `[reject]` seed rows arm no delay — seed cases are already past Aberto. The matrix checks their states and the escalated flag. Only a live `Open` arms a delay.
  - `[medium]` `[defer]` broker refusal is proven on a fake, not on AMQP — same dial ban as the topology deferral.
  - `[false]` `[reject]` the unset `CASES_DATABASE_URL` path is untested — `internal/proc/commands_test.go` still starts `cases` and requires `service=cases` plus exit 0 on SIGINT and SIGTERM. That package passed.
  - `[low]` `[reject]` no test asserts that `go test` avoids ports 5435 and 5673 — the suite uses fakes and does not dial. An extra port assertion would be a new guard.

## Design Notes

The seed cases are already past Aberto. `RaiseSeed` writes those three rows and does not arm a second delay for a case that is already escalated. A live open arms one delay. The dead-letter body carries the case id. Handling it is idempotent on that id.

Expiry does not change the state. Advisory is a later consumer of `case.sla.breached`; this story only publishes it.

## Verification

**Commands:**
- `gofmt -l .` -- expected: empty
- `go mod tidy && git diff --exit-code go.mod go.sum` -- expected: clean
- `go vet ./...` -- expected: exit 0
- `go build ./...` -- expected: exit 0
- `mkdir -p .gotmp && GOTMPDIR="$PWD/.gotmp" go test -race -shuffle=on ./...` -- expected: pass

## Auto Run Result

**Summary:** Cases stores the four forward states, halves the SLA clock from the segment and the four factors, and arms one delay row in the same transaction as the case and `case.opened`. After commit, the publisher sends that delay to `cases.sla.delay` with a per-message TTL. Dead-lettering runs `ApplyBreachDelivery`, which sets escalated, keeps the state, and writes one `case.sla.breached` outbox row. A second delivery is a no-op. There is no expiry scanner.

**Files:**
- `internal/cases/clock.go` — base minutes, halve, and display band
- `internal/cases/machine.go` — open, advance, seed, breach, delay publish, and the outbox poller
- `internal/cases/queue.go` — TTL milliseconds and the dead-letter routing key
- `internal/cases/pgx.go` — PostgreSQL adapter, including `sla_delay`
- `migrations/cases/001_cases.sql` — cases, inbox, outbox, and `sla_delay`
- `cmd/cases/main.go` — seed, delay topology, publisher, and breach consumer
- `.env.example` — local cases database and broker URLs
- `internal/cases/clock_test.go` and `internal/cases/machine_test.go` — matrix rows, including the patched clock, publisher, and breach-body tests

**Review:** Three medium groups were patched: the two missing halve factors, `RunPublisher` with a dual fake, and breach-body decode via `ApplyBreachDelivery`. PGX SQL and the live AMQP delay topology were deferred because `go test` must not dial 5435 or 5673. Rejected findings were either untrue at the cited code (ticker is not an expiry scan, a second open does not arm another delay, display is the pure function) or low items whose fix would add a guard or a log of customer text.

**Follow-up review:** true. Patched medium groups: 3. High patches: 0. Unverified risk: `PGXStore` and the live delay queue, Expiration, and dead-letter bind are not executed by `go test`.

**Verification:** `gofmt -l .` empty. `go mod tidy` left `go.mod` and `go.sum` clean. `go vet ./...` and `go build ./...` exited 0. `GOTMPDIR=$PWD/.gotmp go test -race -shuffle=on -count=1 ./...` passed, including `internal/cases` and `internal/proc`.

**Residual risk:** A wrong SQL statement or a wrong AMQP Expiration field stays green until a process with `CASES_DATABASE_URL` and `CASES_BROKER_URL` is run.
