# ADR 0007 — UUIDv7 identifiers and SQL seeds

- **Status:** accepted
- **Date:** 2026-09-27

## Context

Demo cast (customers, operators, queue cards, cases, review rows, and event ids)
lived in Go maps and in-process BFF seeds. Sequential string ids (`c01`, `s01`)
did not match the product contract for stable, sortable identifiers. Schema was
also created at process startup with `CREATE TABLE`, which duplicated migrations
and blocked CD from owning schema apply.

## Decision

- Persist customer, operator, signal, case, review, and `event_id` values as
  native PostgreSQL `uuid` columns holding UUIDv7 literals. Go validates path and
  body ids with `github.com/google/uuid` and rejects non-v7 values with HTTP 400.
  PostgreSQL 17 has no `uuidv7()`; new runtime inserts generate UUIDv7 in Go.
- Move the visible cast out of Go and TypeScript into per-database SQL under
  `seeds/<service>/`. Segment bounds and rule codes stay in Go (they are rules,
  not cast).
- `cmd/db migrate` applies only `migrations/*.sql` per service database. It is
  the command CD will call. `cmd/db seed` applies `seeds/<service>/` on demand.
  Reseed refreshes clocks of seeded rows only (`now() - interval`) and resets
  those outbox rows' `published_at` to null. Unknown ids are left alone. Inbox
  rows for seeded `event_id` values are insert-if-absent.
- Go process startup no longer runs `CREATE TABLE`. Schema lives in migrations.
- Operators live in `advisory.operators`. Cases store the operator UUIDv7 at
  open time; the live display name is resolved from advisory.
- The manager panel is computed from live data. A case is at risk when it is not
  resolved and has 60 minutes or less of SLA remaining, or is already overdue.
  That threshold lives in the cases service.
- Timeline is a projection. Visible rows come from the owning outboxes
  (`account_sim`, `advisory`, `cases`). The indexer replays those outboxes into
  Elasticsearch with document id = `event_id`.

## Consequences

- Unit tests generate other UUIDv7 values and must not hard-code the cast names
  or dial local ports 5435, 5673, 8400, 8420, or 9201.
- A consistency test reads the seed SQL files and checks every id is UUIDv7 and
  that customer, operator, and event ids match across files.
- BFF remains store-less and aggregates advisory, triage, cases, and timeline
  over gRPC (plus the existing actions HTTP surface).

## Reassessment trigger

Revisit if PostgreSQL gains a native `uuidv7()` the team wants to adopt, or if
the demo cast must be generated procedurally instead of checked-in SQL.
