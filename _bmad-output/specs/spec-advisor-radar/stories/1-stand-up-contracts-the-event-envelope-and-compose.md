---
title: 'Stand up contracts, the event envelope, and Compose'
type: 'feature'
created: '2026-09-27'
status: 'done'
review_loop_iteration: 0
baseline_revision: '49127426f0b64f99f505221889fbe8090235b666'
followup_review_recommended: true
context:
  - '{project-root}/AGENTS.md'
  - '{project-root}/_bmad-output/specs/spec-advisor-radar/SPEC.md'
  - '{project-root}/_bmad-output/specs/spec-advisor-radar/architecture.md'
warnings: []
deferred:
  - summary: >-
      AGENTS.md and CLAUDE.md still say the repository has no Go module and no application code.
    evidence: |-
      The State section was left as written before this story. After the module lands, that paragraph is false and can send a later agent looking for a greenfield tree. The fix edits agent-context files, so this pass defers it.
    location: >-
      AGENTS.md State section
    severity: medium
---

<intent-contract>

## Intent

**Problem:** The repository has CI, agent rules, and the product contract, but no Go module, no service processes, no shared event body, and no local infrastructure project. Later stories have nowhere to put behavior.

**Approach:** Add one module with six command entrypoints that start and stop cleanly, a shared event envelope for the body fields the architecture requires, the six architecture decision records those commands depend on, and a Compose project for the local databases and broker.

## Boundaries & Constraints

**Always:** One module `github.com/leohteixeira/advisor-radar`, Go 1.26. Commands are only `cmd/account-sim`, `cmd/advisory`, `cmd/triage`, `cmd/cases`, `cmd/timeline-indexer`, and `cmd/bff`. Packages stay under `internal/`, short names, no `utils`. `context.Context` is the shutdown signal. Errors wrap with `%w`. Logs are `log/slog` JSON and never include credentials. Compose project name is `advisor-radar`. Published host ports are PostgreSQL 5435, RabbitMQ 5673, RabbitMQ management 15673, and Elasticsearch 9201. One PostgreSQL server holds four databases (`account_sim`, `advisory`, `triage`, `cases`). Elasticsearch is a single node with a 512 MB heap. The JSON event body is exactly `event_id`, `occurred_at`, `customer_id`, and `schema_version` (integer 1). The event name is the routing key, not a body field. Known names are `account.event.recorded`, `message.received`, `message.triaged`, `alert.raised`, `case.opened`, `case.status.changed`, and `case.sla.breached`. ADRs live under `specs/adr/` and match `architecture.md`.

**Never:** Business behavior (burst, rules, classification, cases, timeline, HTTP, SSE, gRPC handlers). A frontend. Application ports. A shared database. `message.reclassify.requested`. Cutting the broker or adding a cron. Importing `/workspace/repos/radar-triage`. Logging secrets. Committing `.env`.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Valid body | Known name, non-empty ids, non-zero time, schema 1 | JSON object with the four body fields only; name is omitted | No error expected |
| Missing id | Empty `event_id` or `customer_id` | Rejected | Wrapped validation error |
| Zero time | `occurred_at` is the zero time | Rejected | Wrapped validation error |
| Bad schema | `schema_version` below 1 | Rejected | Wrapped validation error |
| Unknown name | Name not in the MVP set | Rejected | Wrapped validation error |
| Signal stop | A service process is running | Process exits 0 after SIGINT or SIGTERM | No error expected |
| Compose project | `compose.yaml` rendered | Project `advisor-radar`; host ports 5435, 5673, 15673, 9201; Postgres databases `account_sim`, `advisory`, `triage`, and `cases` | No error expected |

</intent-contract>

## Code Map

- `.github/workflows/ci.yml` lines 65–97 -- Go gate this story must pass: `go mod tidy` clean, `gofmt`, `go vet`, `go build ./...`, `go test -race -shuffle=on ./...`. `go-version-file: go.mod` requires a module.
- `.github/workflows/ci.yml` lines 45–62 and `lefthook.yml` web job -- expect `web/pnpm-lock.yaml`. Read-only. The frontend is a later story; do not add `web/` and do not weaken this job.
- `lefthook.yml` go job -- same Go gate as CI, with `GOTMPDIR` under `.gotmp/`.
- `.gitignore` -- ignores `bin/`, `.gotmp/`, `.env`, and `_bmad/render/`. Local passwords stay in `.env`.
- `AGENTS.md` and `_bmad-output/specs/spec-advisor-radar/architecture.md` -- command list, event body, ports, ADR list, no `utils`.
- No `go.mod`, `cmd/`, `compose.yaml`, or `specs/` yet. Remote is `github.com/leohteixeira/advisor-radar`. Branch `main` is the product line and is clean.

## Tasks & Acceptance

**Execution:**
- `specs/adr/0001-sync-queries-async-state.md` -- record gRPC reads versus RabbitMQ state changes -- architecture requires the ADR before dependent code
- `specs/adr/0002-transactional-outbox-idempotent-inbox.md` -- record outbox, inbox by `event_id`, backoff, and a dead-letter queue per queue
- `specs/adr/0003-sla-escalation-by-ttl.md` -- record SLA escalation by queue TTL and a dead-letter exchange
- `specs/adr/0004-typed-triage-decision-model.md` -- record the typed classifier with heuristic fallback
- `specs/adr/0005-resilience-decorator-order.md` -- record decorator order and error classes from architecture.md
- `specs/adr/0006-persistence-per-service.md` -- record one PostgreSQL database per service, Elasticsearch for the timeline, and an empty BFF
- `go.mod` -- module `github.com/leohteixeira/advisor-radar`, `go 1.26` -- CI reads this file
- `internal/event/event.go` -- envelope type, known names, validate, JSON body without the name -- shared contract for every later publisher
- `internal/event/event_test.go` -- table-test the I/O rows for the envelope
- `internal/proc/proc.go` -- `Run(ctx, name)` logs the service with `slog` JSON and returns when `ctx` is canceled -- shared startup without business work
- `internal/proc/proc_test.go` -- cancel returns nil; blank name is rejected
- `cmd/account-sim/main.go`, `cmd/advisory/main.go`, `cmd/triage/main.go`, `cmd/cases/main.go`, `cmd/timeline-indexer/main.go`, `cmd/bff/main.go` -- `signal.NotifyContext` then `proc.Run` -- one entrypoint per service
- `internal/proc/commands_test.go` -- build each command, start it, send SIGTERM, expect exit 0 and a JSON log line with that service name
- `compose.yaml` -- project `advisor-radar`, Postgres 17 with init for four databases, RabbitMQ 4 management, Elasticsearch 8 single-node 512 MB heap -- local infrastructure only
- `deploy/postgres/init.sql` -- create `account_sim`, `advisory`, `triage`, and `cases`
- `.env.example` -- local-only variable names for database and broker passwords, no real secrets
- `compose_test.go` -- assert project name, published host ports, and database names from the compose and init files

**Acceptance Criteria:**
- Given a clean module, when `go build ./...` runs, then the six commands compile.
- Given `compose.yaml`, when the published ports are read, then they are exactly 5435, 5673, 15673, and 9201 and no application port is published.
- Given the six ADR files, when each is read, then it states the matching decision from `architecture.md` and does not add a seventh decision.

## Spec Change Log

## Review Triage Log

### 2026-09-27 — Review pass
- verdicts: 29 findings — high 0, medium 10, low 10, false 9, maybe-false 0
- findings:
  - `[low]` `[patch]` Verification said five host ports — corrected the sentence to the four published mappings.
  - `[low]` `[reject]` `schema_version` above 1 is accepted — the matrix only rejects values below 1, and `SchemaVersionMVP` is 1. Rejecting other values would add a guard no caller needs yet.
  - `[medium]` `[patch]` Infrastructure said Grafana could be cut with Elasticsearch — the sentence now cuts only Elasticsearch after a measured failure, and Grafana stays.
  - `[false]` `[reject]` Spec Change Log and Review Triage Log were empty — the change log stays empty until a bad_spec loopback. This pass is the first triage entry.
  - `[false]` `[reject]` Memlog still shows the old MySQL constraint — the log is append-only, and the new decision line says it supersedes that constraint.
  - `[medium]` `[defer]` AGENTS.md and CLAUDE.md State still say there is no Go module — deferred because the fix edits agent-context files.
  - `[low]` `[reject]` Acceptance criteria do not restate the I/O matrix — the tests already cover those rows. Restating them would only edit the story spec.
  - `[medium]` `[patch]` Compose test missed the init mount, heap, and single-node, and a commented port line could pass — the test now requires the mount, `512m`, `discovery.type: single-node`, and uncommented port entries.
  - `[medium]` `[patch]` Elasticsearch healthcheck called curl — the official 8.17 image has no curl. The check now uses bash `/dev/tcp`.
  - `[false]` `[reject]` `Validate` does not use `%w` — it creates the root error. `MarshalBody` wraps that error with `%w`, and the test requires the marshal path to fail.
  - `[low]` `[patch]` Startup failures used `log.Fatal` — each command now logs slog JSON on stderr and exits 1.
  - `[false]` `[reject]` Compose does not run Grafana — this story's compose is the databases and the broker. The dashboard stays in the contract for a later story.
  - `[low]` `[reject]` Edge case: `schema_version` above 1 is accepted — same evidence as the schema finding above.
  - `[low]` `[reject]` Whitespace-only ids pass — the matrix rejects empty ids. Trimming would add a guard the matrix does not require.
  - `[low]` `[reject]` An already-canceled context still logs startup — `main` passes a live signal context. `Run` logs and then returns when the context is done.
  - `[medium]` `[patch]` Compose contract was only a source-text scan — the three demonstrations (commented port, dropped mount, changed heap) now fail the test. `docker compose config` stays a verification command because the Go CI job has no Docker daemon.
  - `[medium]` `[patch]` Three MVP event names never passed `Validate` — the success case now loops every known name constant.
  - `[medium]` `[patch]` Signal stop covered only SIGTERM — each command is now stopped with SIGINT and with SIGTERM.
  - `[false]` `[reject]` Intent note on wrapped errors — `MarshalBody` wraps with `%w`. The matrix error path is that function.
  - `[low]` `[reject]` Intent note that Always says schema 1 while the matrix rejects only values below 1 — the code follows the matrix, and the constant is 1.
  - `[medium]` `[patch]` Intent note that tests skipped SIGINT — covered by the signal-stop patch.
  - `[medium]` `[patch]` Intent note that tests never render Compose — covered by the stronger file assertions. The renderer stays `docker compose config` outside `go test`.
  - `[medium]` `[patch]` Intent note that the heap was not asserted — `ES_JAVA_OPTS` must contain `512m`.
  - `[low]` `[reject]` ADR text is not compared to architecture.md by a test — the six files restate that document. A prose diff would be brittle.
  - `[false]` `[reject]` The compose test forbids ports 3400 and 8400 — that matches the Never rule, and Compose publishes neither.
  - `[low]` `[reject]` Nothing tests that secrets are absent from logs — `.env` is gitignored, and the startup log carries only the service name.
  - `[false]` `[reject]` Cut order and the 8 GB VPS look beyond the original scaffold — the user directed that decision in this run, and the contract records it on purpose.
  - `[false]` `[reject]` The story file is outside the technical approach — the workflow requires that file.
  - `[false]` `[reject]` Blank service name is an extra test — the task list requires `Run` to reject it.

## Design Notes

The event name stays off the JSON body because `architecture.md` lists four body fields and says trace context rides headers. Callers keep the name on the Go value (`json:"-"`) and later stories use it as the routing key. `schema_version` is the integer `1`.

One PostgreSQL process with four database names satisfies "databases are not shared" without four servers. Application processes stay on the host; this story does not assign gRPC listen ports because the contract does not list them.

Local passwords are `${VAR:-localdev}` inside Compose so `docker compose config` works without a secret file. `.env.example` documents the same placeholder. Do not copy the gateway key into Compose.

## Verification

**Commands:**
- `gofmt -l .` -- expected: empty
- `go mod tidy && git diff --exit-code go.mod go.sum` -- expected: clean
- `go vet ./...` -- expected: exit 0
- `go build ./...` -- expected: exit 0
- `mkdir -p .gotmp && GOTMPDIR="$PWD/.gotmp" go test -race -shuffle=on ./...` -- expected: pass
- `docker compose config` -- expected: project `advisor-radar` and the four published host ports

## Auto Run Result

Status: done

Summary: One Go module now has six commands that start and stop on SIGINT and SIGTERM, a shared event envelope whose JSON body is the four required fields, six ADRs, and a Compose project for PostgreSQL (four databases, including `cases`), RabbitMQ, and Elasticsearch. MySQL is not part of the stack.

Files changed:
- `go.mod`, `go.sum` — module `github.com/leohteixeira/advisor-radar` on Go 1.26
- `cmd/*/main.go` — one entrypoint per service
- `internal/event/event.go` — envelope, known names, validation, JSON body
- `internal/proc/proc.go` — slog JSON startup and shutdown on context cancel
- `compose.yaml`, `deploy/postgres/init.sql`, `.env.example` — local infra, no MySQL
- `specs/adr/0001` through `0006` — the six architecture decisions
- `internal/event/event_test.go`, `internal/proc/proc_test.go`, `internal/proc/commands_test.go`, `compose_test.go` — matrix and signal coverage
- `SPEC.md`, `architecture.md`, `AGENTS.md`, `CLAUDE.md`, `.memlog.md` — cases on PostgreSQL; Elasticsearch and Grafana stay until a measurement on the 8 GB VPS fails

Review: five medium patches (Grafana cut sentence, Compose assertions, Elasticsearch healthcheck, all seven event names, SIGINT plus SIGTERM) and two low patches (slog on startup failure, verification port count). Deferred: the State section in AGENTS.md and CLAUDE.md still describes a greenfield tree. Rejected findings are listed in the triage log, including schema versions above 1, whitespace ids, wrapping on `Validate` itself, and putting Grafana in this Compose project.

Follow-up review recommended: true. Patched medium entries: 5. Patched low entries: 2. Unverified risk: the Elasticsearch healthcheck uses bash `/dev/tcp` and was not executed inside `elasticsearch:8.17.0`.

Verification:
- `gofmt -l .` empty
- `go mod tidy` left `go.mod` and `go.sum` clean
- `go vet ./...` exit 0
- `go build ./...` exit 0
- `go test -race -shuffle=on ./...` pass
- `docker compose config` exit 0, project `advisor-radar`

Residual risk: the web CI job still expects `web/pnpm-lock.yaml`, which this story does not add. The Elasticsearch healthcheck is unproven inside the image.
