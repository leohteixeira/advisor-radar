---
title: 'Record ADR 0008 and schema version 2'
type: 'feature'
created: '2026-09-28'
status: 'done'
review_loop_iteration: 0
baseline_revision: '423d9f4108a85044487e052893cb59020be273fc'
followup_review_recommended: true
context:
  - '{project-root}/AGENTS.md'
  - '{project-root}/_bmad-output/specs/spec-client-pov/architecture.md'
  - '{project-root}/specs/adr/0001-sync-queries-async-state.md'
warnings: []
deferred:
  - summary: >-
      Timeline and BFF local parsers still accept schema_version values above 2.
    evidence: |-
      internal/timeline/index.go and internal/bff/board.go reject only schema_version < 1. Envelope.Validate now allows only 1 and 2. Those parsers do not scale money.
    location: >-
      internal/timeline/index.go:111
    severity: low
---

<intent-contract>

## Intent

**Problem:** A client action has no recorded path into the system, and every account amount is read as whole dollars. A later cent payload would be treated as dollars and fire the phase-1 rules on the wrong scale.

**Approach:** Record ADR 0008 as an extension of ADR 0001, before any POV command code. Accept schema version 2 on the existing envelope: integer USD cents plus origin or destination. Schema version 1 stays whole dollars. The advisory consumer converts to dollars by version before a rule runs.

## Boundaries & Constraints

**Always:** ADR 0008 lives at `specs/adr/0008-client-command-grpc-outbox.md`, status accepted, dated 2026-09-28, and states that the BFF delivers a client action to account-sim over gRPC with the caller's deadline, and account-sim writes state and the outbox event in one transaction. No new event name. `event.SchemaVersionMVP` stays `1`. Add `event.SchemaVersionCents` as `2`. `Envelope.Validate` accepts only versions 1 and 2. `sim.AccountPayload` keeps `kind`, `amount`, `before`, and `after`, and adds `origin` and `destination` with `omitempty`. `AccountPayload.Dollars(schemaVersion)` returns a copy whose three amounts are whole dollars: version 1 unchanged, version 2 integer cents divided by 100. `advisory.Apply` converts with that method before `EvaluateAccount`. Alert payloads stay whole dollars on schema version 1. Version 1 JSON without origin or destination stays unchanged.

**Never:** gRPC command handlers, POV HTTP routes, account seed, `book.aum` updates, new event names, OpenTelemetry, a frontend change, or treating version 2 cents as dollars inside a rule.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Version 1 dollars | schema 1, withdrawal amount 60000, before 8000, after 68000, no origin | `Dollars` returns those three numbers; `Validate` accepts the envelope; rules still see dollars | No error |
| Version 2 cents | schema 2, kind aporte, amount 1000000, before 820000, after 1820000, origin `pix` | `Dollars` returns 10000, 8200, 18200 and keeps origin; deposit and segment decisions use those dollars | No error |
| Fractional cents | schema 2, amount 10000.5 | `Dollars` fails | Error names cents; `Apply` returns the error and writes no alert |
| Unknown version | schema 3 envelope | `Validate` rejects it | Error names schema_version |
| Version 2 envelope | schema 2, known name, required body fields | `Validate` and `MarshalBody` succeed and keep schema_version 2 | No error |

</intent-contract>

## Code Map

- `specs/adr/0001-sync-queries-async-state.md` -- phase-1 split this ADR extends; do not edit it
- `internal/event/event.go:21` -- `SchemaVersionMVP`; `Validate` currently allows any version `>= 1`
- `internal/sim/burst.go:11` -- `AccountPayload` is float dollars today
- `internal/advisory/apply.go:66` -- `Apply` decodes then calls `EvaluateAccount` with no scale step
- `internal/advisory/rules.go:45` -- rules assume dollar amounts; leave the thresholds here
- `internal/book/book.go:32` -- `SegmentFromAssets` uses dollar bounds 10000 and 200000
- `cmd/advisory/main.go:359` -- delivery already copies `schema_version` onto the envelope

## Tasks & Acceptance

**Execution:**
- `specs/adr/0008-client-command-grpc-outbox.md` -- record the gRPC command and outbox decision -- the code that follows must have the decision on disk first
- `internal/event/event.go` -- add `SchemaVersionCents` and accept only versions 1 and 2 -- unknown versions must not enter a consumer
- `internal/sim/burst.go` -- add origin, destination, and `Dollars` -- one reader for every later consumer
- `internal/advisory/apply.go` -- convert before `EvaluateAccount` -- rules stay on the dollar book
- `internal/event/event_test.go` -- cover versions 2 and 3 -- matrix rows for the envelope
- `internal/sim/burst_test.go` -- cover dollar, cent, and fractional rows -- matrix rows for money
- `internal/advisory/advisory_test.go` -- one version-2 aporte that yields dollar deposit and segment decisions, and one fractional-cent apply that writes nothing -- the consumer is the outermost surface

**Acceptance Criteria:**
- Given ADR 0001, when ADR 0008 is read, then it extends that decision and does not replace the phase-1 query path.
- Given a version-1 account fact already in tests, when advisory applies it, then the raised amounts stay the same whole dollars.

## Spec Change Log

## Review Triage Log

### 2026-09-28 — Review pass
- verdicts: 17 findings — high 1, medium 3, low 6, false 7, maybe-false 0
- findings:
  - `[false]` `[reject]` Version 1 matrix row uses withdrawal numbers that add like a deposit — `Dollars` is asked to return 60000/8000/68000 unchanged, and `burst_test` asserts that; it does not check cash arithmetic.
  - `[low]` `[reject]` Acceptance criteria omit matrix rows — the fix would edit this build's spec; the matrix rows already have tests.
  - `[low]` `[reject]` Tasks omit the `aporte`/`saque` kind aliases — the fix would edit this build's spec; the matrix requires `kind: aporte` to reach the deposit rule.
  - `[medium]` `[patch]` `saque` was accepted with no Apply test — added `TestApply_SchemaVersionCentsSaque`.
  - `[low]` `[patch]` `destination` was never round-tripped — the version 2 `Dollars` case now keeps `Destination`.
  - `[false]` `[reject]` Cent tests do not assert alert `schema_version` — `raise` still sets `event.SchemaVersionMVP` on `alert.raised`.
  - `[low]` `[reject]` NaN and fractional `before`/`after` lack their own rows — they share `centsToDollars`, which the fractional `amount` test already fails.
  - `[false]` `[reject]` `Apply` does not call `Validate` for schema 3 — `handleDelivery` rejects schema 3 as a permanent error before `Apply`; `event_test` covers version 3.
  - `[low]` `[defer]` Timeline and BFF parsers still allow `schema_version >= 1` — those parsers predate this story and do not scale money; advisory is the rule consumer.
  - `[low]` `[patch]` ADR 0008 read as if messages carried cents — the schema 2 sentence now limits cents, origin, and destination to account payloads.
  - `[false]` `[reject]` Spec Change Log is empty — that log stays empty until a bad-spec loopback, and this pass did not revert.
  - `[false]` `[reject]` Timeline might show cents as dollars — `entryFromEvent` does not use the numeric amount, and this story does not change that contract.
  - `[high]` `[patch]` Fractional cents passed `Validate` and `Apply` returned a retryable error, so the consumer would requeue forever — `ErrMoneyScale` plus `classifyApplyError` mark that failure permanent.
  - `[medium]` `[patch]` Removing the `saque` case left the suite green — same saque Apply test as the blind-hunter row.
  - `[false]` `[reject]` Version 1 JSON omitempty is untested as bytes — the version 1 `Dollars` case compares the whole struct, and empty origin and destination stay empty.
  - `[medium]` `[patch]` `saque` is outside the Always list — architecture withdrawals use `kind: saque`; the Apply test locks that alias.
  - `[false]` `[reject]` The story markdown is an extra surface — it is this run's spec file, not a product behavior.

## Auto Run Result

Status: done

Summary: ADR 0008 records the gRPC command into account-sim and the outbox publish. Schema version 2 is integer cents on account payloads. Advisory converts to whole dollars before the phase-1 rules. Invalid cents are a permanent delivery, not a requeue.

Files changed:
- `specs/adr/0008-client-command-grpc-outbox.md` — accepted decision extending ADR 0001
- `internal/event/event.go` — accept only schema versions 1 and 2
- `internal/sim/burst.go` — origin, destination, and `Dollars`
- `internal/advisory/apply.go` — convert before `EvaluateAccount`
- `internal/advisory/rules.go` — `aporte` and `saque` use the existing dollar rules
- `cmd/advisory/main.go` — money-scale errors do not requeue

Review findings breakdown:
- Patched: 1 high (permanent nack for bad cents), 3 medium (saque coverage), 2 low (destination round-trip, ADR wording).
- Deferred: timeline and BFF parsers still allow schema versions above 2. They do not scale amounts.
- Rejected: matrix arithmetic wording, spec-only edits, alert schema already version 1, shared cents helper, Validate-before-Apply, empty change log, timeline display, omitempty struct equality, and the story file itself.

Follow-up review recommendation: true. `classifyApplyError` is unit-tested. The live `runConsumer` nack path was not executed against RabbitMQ.

Verification: `go test ./internal/event/ ./internal/sim/ ./internal/advisory/ ./cmd/advisory/` passed.

Residual risk: a schema version 2 body that never reaches `Envelope.Validate` can still be indexed by timeline or the BFF board. This story has no screen, so visual coverage does not apply here.

## Verification

**Commands:**
- `go test ./internal/event/ ./internal/sim/ ./internal/advisory/` -- expected: all tests pass
