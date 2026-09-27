# Advisor Radar — agent instructions

Repository-specific extension. Read `/workspace/CLAUDE.md` first; what is written here prevails
only inside `repos/advisor-radar`.

The product brief is at `/workspace/docs/advisor_radar_brief.md`. This is a technical demo with
fictional data and no link to any real institution.

## State

Greenfield. The repository currently contains a license and the repository baseline (CI,
lefthook, gitleaks, ignore files, and these instructions). There is no Go module, no
application code, and no frontend yet.

## Architecture rules

- Single Go module. One command per service under `cmd/`: `account-sim`, `advisory`, `triage`,
  `cases`, `timeline-indexer`, and `bff`. Packages under `internal/` are organized by
  responsibility, with short names and no cycles. **Never create a `utils` package.**
- Interfaces are small and declared by the **consuming** package, not by the one implementing them.
- Propagate `context.Context` through every I/O operation. Wrap errors with `%w`. Bound
  concurrency explicitly. Worker pools shut down gracefully with `signal.NotifyContext`.
- Queries between services use gRPC with a propagated deadline. State changes are RabbitMQ
  events. Opening a case from an alert is an asynchronous command. Record that split as an ADR
  under `specs/` before implementing it.
- Every publisher uses a transactional outbox. Every consumer uses an idempotent inbox keyed by
  `event_id`. Retries use backoff and a dead-letter queue per queue. SLA escalation uses a queue
  with TTL and a dead-letter exchange, not a cron.
- Repositories and external integrations sit behind ports and adapters. Alert rules are small
  interfaces, one per rule. gRPC interceptors cover tracing, deadline, and retry.
- Persistence is per service: PostgreSQL for `account-sim`, `advisory`, and `triage`; MySQL for
  `cases`; Elasticsearch for `timeline-indexer`; the BFF stores nothing. Do not share a database
  across services.
- Structured logs use `log/slog` JSON. OpenTelemetry on every service. Propagate trace context
  on RabbitMQ headers so one trace covers the event, the rule, the alert, and the SSE push.
  Never log tokens, credentials, connection strings, or model-provider keys.
- The triage classifier already exists in `/workspace/repos/radar-triage`. Copy it into this
  module when building the `triage` service. Do not import it across the repository boundary.
  Keep the `Classifier` port, the Jev adapter, the keyword heuristic, and the `Fallback`
  decorator. Numbers and dates stay in code and never go to the model. The model decides; it
  does not draft text. Human review applies when intent probability is below the calibrated
  threshold (currently 0.7). When the external model is down, classification is heuristic and
  marked degraded.
- Frontend: React, TypeScript, and Vite, mobile-first. It talks to the BFF only, through
  versioned HTTP/JSON, plus one SSE connection for the advisor queue. Business rules, scoring,
  and SLA stay in the backend. Shipped UI copy is Portuguese. Data, names, and branding are
  fictional.
- The MVP has no real authentication (fixed persona selection or a fictional login), no
  integration with a real brokerage, and no real messages sent to a customer.

### Services

| Service | Owns | Persistence | Exposes |
|---|---|---|---|
| account-sim | Fictional account events and customer messages | PostgreSQL (outbox) | Publishes to RabbitMQ |
| advisory | Book, segmentation, alert rules | PostgreSQL | gRPC; consumes and publishes |
| triage | Message classification with fallback | PostgreSQL (inbox and results) | Consumes and publishes |
| cases | Cases, SLA, state machine, escalation | MySQL | gRPC; consumes and publishes |
| timeline-indexer | Customer 360 index | Elasticsearch | Read by the BFF |
| bff | HTTP for the frontend, gRPC aggregation, SSE | None | HTTP and SSE |

### Events

Every event carries `event_id`, `occurred_at`, `customer_id`, `schema_version`, and trace
context in the headers.

| Event | Producer | Consumers |
|---|---|---|
| `account.event.recorded` | account-sim | advisory, timeline-indexer |
| `message.received` | account-sim | triage, timeline-indexer |
| `message.triaged` | triage | advisory, cases, timeline-indexer |
| `alert.raised` | advisory | bff, cases, timeline-indexer |
| `case.opened`, `case.status.changed` | cases | bff, timeline-indexer |
| `case.sla.breached` | cases | bff, advisory |

### Local ports

Bind development servers to `0.0.0.0` so port forwarding works. These ports are local to this
repository and must stay off the portfolio assignments (Fleet Pulse uses 3300, 8300, and 1883).

| Surface | Port |
|---|---:|
| Web (Vite) | 3400 |
| BFF HTTP and SSE | 8400 |
| RabbitMQ | 5673 |
| RabbitMQ management | 15673 |
| PostgreSQL | 5435 |
| MySQL | 3307 |
| Elasticsearch | 9201 |

## Language

Everything in this repository is written in English: code, identifiers, comments, branch names,
commit messages, specifications, READMEs and any other documentation.

## Commands

```bash
go test ./...                    # after the Go module exists
go test -race -shuffle=on ./...
pnpm --dir web test              # after the frontend exists
pnpm --dir web build
lefthook install                 # once per clone; pre-push matches CI
```

CI (`.github/workflows/ci.yml`) and the lefthook `pre-push` hook run the same gate: gitleaks
over the full history, then `go mod tidy` (clean diff), `gofmt`, `go vet`, `go build`, and
`go test -race -shuffle=on ./...`, plus `pnpm --dir web test` and `pnpm --dir web build`.
Go tests set `GOTMPDIR` to `.gotmp/` in the hook. gitleaks is pinned at 8.30.1 in CI and in
the Dev Container; the hook calls that binary directly. Node is 24.19.0 and pnpm is 11.22.0.

Always run Git from this directory, never from `/workspace`. Do not commit automatically.

Commits use `type(scope): description` with a coherent scope
(`backend`, `frontend`, `infra`, etc.), without agent `Co-Authored-By`
trailers. Stage explicit paths.

Before any Go coding, review, debugging, troubleshooting, or setup task,
load the `samber/cc-skills-golang@golang-how-to` skill first — it routes to whichever other Go
skills the task needs.

## Required Go skills

The following Go skills from `samber/cc-skills-golang` MUST always be applied when working on
this project. Load them at the start of every Go-related task, regardless of whether the user
explicitly mentions them.

- `samber/cc-skills-golang@golang-code-style`
- `samber/cc-skills-golang@golang-concurrency`
- `samber/cc-skills-golang@golang-context`
- `samber/cc-skills-golang@golang-data-structures`
- `samber/cc-skills-golang@golang-design-patterns`
- `samber/cc-skills-golang@golang-documentation`
- `samber/cc-skills-golang@golang-error-handling`
- `samber/cc-skills-golang@golang-modernize`
- `samber/cc-skills-golang@golang-naming`
- `samber/cc-skills-golang@golang-safety`
- `samber/cc-skills-golang@golang-security`
- `samber/cc-skills-golang@golang-testing`
- `samber/cc-skills-golang@golang-troubleshooting`
