# Advisor Radar

Fictional advisory desk. A visitor can open the team queue or enter Orla Invest as one seeded client and cause the same events the team already sees.

The public app is rooted at `/advisor-radar`.

## Selection

`/advisor-radar` is the selection screen. It does not redirect to the queue.

- **Visão do time** opens `/advisor-radar/fila`. Review, the manager panel, and the customer 360 stay at `/advisor-radar/revisao`, `/advisor-radar/painel`, and `/advisor-radar/clientes/{id}`.
- **Visão do cliente** opens `/advisor-radar/client-pov`.

The screen states three pillars (proactive alerts, message triage, cases with SLA) and plays an eight-step complaint walkthrough. The walkthrough pauses and stays still when `prefers-reduced-motion` is set.

## Client point of view

`/advisor-radar/client-pov` lists three seeded clients:

| Client | Segment | Assets | Available cash |
|---|---|---:|---:|
| Mariana Costa | Singular | USD 248,300 | USD 60,000 |
| Fernanda Lima | Essencial | USD 8,200 | USD 1,148 |
| Thiago Azevedo | Advance | USD 68,000 | USD 60,520 |

The phone app can deposit, withdraw, send a free message, or file a preset complaint. Desktop actions use a 440px panel. The client app is dark by default and can switch to the light theme. The team list and the selection screen stay light.

An accepted action returns `202` with `event_id`. The confirmation protocol is the first two UUID groups of that id, uppercased. Bastidores follows `GET /v1/client-pov/customers/{id}/stream`. The queue link is `/advisor-radar/fila`.

Money on the POV wire is integer USD cents. Schema version 1 events stay whole dollars. Schema version 2 and 3 account events are cents; advisory converts them into the dollar book before the phase-1 rules run. Schema version 3 is the purchase (`kind: aplicacao`, from `POST /v1/client-pov/customers/{id}/purchases`): cash moves into a position at the catalog price, so `before` equals `after`, and the payload adds `product_id`, `asset_class`, and `risk`. A purchase whose risk is above the client's investor profile raises the alert-only `perfil` card "Compra acima do perfil de investidor" in the team queue; it opens no case. The phase-2 commands stay at version 2.

Names, balances, and Orla Invest are fictional. No message is sent to a real person.

## Demo script

1. Open `/advisor-radar` and walk the eight steps in about 30 seconds.
2. Enter as Mariana Costa and send "Estou pensando em sair".
3. Show Bastidores moving through the outbox, RabbitMQ, and triage.
4. Open `/advisor-radar/fila`. The churn alert is at the top of the queue with the Singular SLA.
5. Return as Fernanda Lima, deposit US$ 10.000, and show the large-deposit and segment-change alerts.
6. Submit the same withdrawal twice with the same idempotency key and show a single event.
7. Press "Avançar um dia" three times. On day 3 Cobalto Semicondutores falls 53,5%: Mariana's home shows "Mariana, sua carteira caiu 15,5% hoje" with the day-change pill, the team queue shows her `queda` alert, and Thiago, who holds little Cobalto, gets no alert.

The simulated day is global. The day-3 shock hits every account that holds Cobalto, not only the demo clients, so a non-demo account with a large Cobalto position may raise its own drop alert. Advancing is limited to 20 days per 10 minutes for everyone; a reseed (`go run ./cmd/db seed`) returns the day to 0 and the values to the seed.

## Configuration

Services read their settings from environment variables, loaded from the local root `.env` (never committed). Client commands reach account-sim over gRPC ([ADR 0008](specs/adr/0008-client-command-grpc-outbox.md)); add these two lines to your local `.env`:

| Variable | Example | Purpose |
|---|---|---|
| `ACCOUNT_SIM_DATABASE_URL` | `postgres://…@127.0.0.1:5435/account_sim` | account-sim PostgreSQL database (state, idempotency keys, outbox). Unset, account-sim only waits for a signal. |
| `ACCOUNT_SIM_BROKER_URL` | `amqp://…@127.0.0.1:5673/` | RabbitMQ the outbox relay publishes to. Unset, the relay does not run. |
| `ACCOUNT_SIM_GRPC_ADDR` | `0.0.0.0:8460` | account-sim gRPC listen address. The server starts only when `ACCOUNT_SIM_DATABASE_URL` is also set. |
| `ACCOUNT_SIM_GRPC_TARGET` | `127.0.0.1:8460` | account-sim target the BFF and advisory dial. The BFF requires it for the client POV; unset, the POV home is `404` and POV commands are `502`. advisory reads each client's balance through it for `GetMomentFacts`, with the caller's deadline or 2 s; unset, advisory logs a warning and `GetMomentFacts` answers `Unavailable`, so the home moment falls back to `welcome`. |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `http://127.0.0.1:4418` | OTLP/HTTP collector every service exports traces and metrics to; `4418` is the `otel-lgtm` Compose service. Unset, OpenTelemetry stays off: no exporter, no dial, no export errors. Only OTLP over HTTP is supported, so point it at an HTTP port (`4418`), not the gRPC one (`4417`). |

`ACCOUNT_SIM_TEST_DATABASE_URL` points the gated `internal/sim` PostgreSQL tests at a database; each test migrates and drops its own schema. Unset, those tests skip. `CASES_TEST_DATABASE_URL` and `ADVISORY_TEST_DATABASE_URL` do the same for the gated `internal/cases` and `internal/advisory` tests.

cases consumes `message.triaged` on `cases.message.triaged` (dead letters go to `cases.message.triaged.dlq`) when both `CASES_BROKER_URL` and `ADVISORY_GRPC_TARGET` are set; with the broker set and no advisory target it logs a warning and runs without intake. Only messages sent from the client app open or join cases: account-sim marks POV messages and complaints with `origin: "client_app"` on `message.received`, triage copies it into `message.triaged`, and seeded or burst messages (no `origin`) are claimed and ignored. A client-app complaint, closing request, or churn risk of 0.5 or more opens one case per customer, with the segment and advisor read from advisory `GetCustomer`; a later qualifying message while that case is open is added to its history. Run `go run ./cmd/db migrate` to apply `cases/003_one_open_case.sql`.

After `go run ./cmd/db migrate` applies `account_sim/004_pov_positions.sql`, run `go run ./cmd/db seed` (reseed): 004 drops the per-class balance columns and leaves the product catalog, positions, and registration tables empty until the seed loads them.

After `go run ./cmd/db migrate` applies `account_sim/005_pov_preferences.sql`, run `go run ./cmd/db seed` (reseed) to write each client's preferences row; until then Perfil shows `chat` with beta off for every client.

After `go run ./cmd/db migrate` applies `advisory/003_investor_profile.sql`, run `go run ./cmd/db seed` (reseed): 003 gives every existing book row the migration default profile (conservador, assessed 2026-01-01) until the seed writes each customer's profile.

`account_sim/006_pov_sim_day.sql` adds the global simulated day, starting at 0, and `advisory/004_revaluation.sql` the latest revaluation per client; neither needs a reseed.

Deploy advisory before cases. Cases intake reads the advisor from advisory `GetCustomer.advisor_id`; against an advisory that does not send it yet, cases drops every qualifying message as `ErrUnusableCustomer` (dead-lettered, no case opened).

### Observability

`docker compose up -d otel-lgtm` starts Grafana with Tempo, Mimir, and Loki behind one OpenTelemetry collector. Grafana is on <http://127.0.0.1:3410> (user `admin`, password `admin`, local only); OTLP is on `4417` (gRPC) and `4418` (HTTP) of the host. With `OTEL_EXPORTER_OTLP_ENDPOINT` set, each service (`account-sim`, `advisory`, `triage`, `cases`, `timeline-indexer`, `bff`) exports under its own `service.name`, set in code; `OTEL_SERVICE_NAME` exported in the shell overrides it for one process and never goes in `.env`. The standard `OTEL_*` exporter variables apply, for example `OTEL_METRIC_EXPORT_INTERVAL=5000` to export metrics every 5 s instead of every 60 s. Shutdown flushes the last batch within 5 s.

- **Traces.** BFF HTTP requests are server spans named after the route pattern (`GET /v1/client-pov/customers/{id}/screens/{slug}`). Every gRPC client and server carries the `otelgrpc` stats handler, so a query from the BFF continues in advisory, cases, account-sim, or timeline. A screen build is one `sdui.screen` span (`sdui.slug`, `sdui.revision`) with one `sdui.snapshot.<source>` child per Snapshot source and one `sdui.section` child per section (`sdui.section`, `sdui.variant`, and `sdui.omitted_reason` when omitted). Trace context does not cross RabbitMQ yet.
- **Metrics.** `sdui_variant_served_total{slug,section,variant}`, `sdui_component_dropped_total{slug,type,reason}`, and `sdui_screen_build_seconds{slug}` from the BFF screen engine; `pov_purchases_total{asset_class}` from account-sim when a purchase (`aplicacao`) commits, so replays and refusals do not count; `advisory_suitability_alerts_total` from advisory when the suitability-mismatch (`perfil`) rule raises an alert. Labels are bounded: catalog keys, source names, and catalog asset classes, never customer ids or copy.

`scripts/gen-proto.sh` regenerates every `proto/*/v1/*.proto` into `gen/`. It requires protoc 29.3, protoc-gen-go v1.36.5, and protoc-gen-go-grpc 1.5.1, and adds the Go install directory (`GOBIN`, else `$(go env GOPATH)/bin`) to `PATH` for the plugins.

## Contract

The BFF HTTP contract, including the POV routes, is in [specs/http/bff.md](specs/http/bff.md). To add a variant to an existing screen section, follow the [`add-sdui-variant` skill](.claude/skills/add-sdui-variant/SKILL.md). Client commands are recorded in [specs/adr/0008-client-command-grpc-outbox.md](specs/adr/0008-client-command-grpc-outbox.md). OpenTelemetry is described under [Observability](#observability).
