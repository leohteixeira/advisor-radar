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

Money on the POV wire is integer USD cents. Schema version 1 events stay whole dollars. Schema version 2 account events are cents; advisory converts them into the dollar book before the phase-1 rules run.

Names, balances, and Orla Invest are fictional. No message is sent to a real person.

## Demo script

1. Open `/advisor-radar` and walk the eight steps in about 30 seconds.
2. Enter as Mariana Costa and send "Estou pensando em sair".
3. Show Bastidores moving through the outbox, RabbitMQ, and triage.
4. Open `/advisor-radar/fila`. The churn alert is at the top of the queue with the Singular SLA.
5. Return as Fernanda Lima, deposit US$ 10.000, and show the large-deposit and segment-change alerts.
6. Submit the same withdrawal twice with the same idempotency key and show a single event.

## Configuration

Services read their settings from environment variables, loaded from the local root `.env` (never committed). Client commands reach account-sim over gRPC ([ADR 0008](specs/adr/0008-client-command-grpc-outbox.md)); add these two lines to your local `.env`:

| Variable | Example | Purpose |
|---|---|---|
| `ACCOUNT_SIM_DATABASE_URL` | `postgres://…@127.0.0.1:5435/account_sim` | account-sim PostgreSQL database (state, idempotency keys, outbox). Unset, account-sim only waits for a signal. |
| `ACCOUNT_SIM_BROKER_URL` | `amqp://…@127.0.0.1:5673/` | RabbitMQ the outbox relay publishes to. Unset, the relay does not run. |
| `ACCOUNT_SIM_GRPC_ADDR` | `0.0.0.0:8460` | account-sim gRPC listen address. The server starts only when `ACCOUNT_SIM_DATABASE_URL` is also set. |
| `ACCOUNT_SIM_GRPC_TARGET` | `127.0.0.1:8460` | account-sim target the BFF dials (used from story 3). |

`ACCOUNT_SIM_TEST_DATABASE_URL` points the gated `internal/sim` PostgreSQL tests at a database; each test migrates and drops its own schema. Unset, those tests skip.

`scripts/gen-proto.sh` regenerates every `proto/*/v1/*.proto` into `gen/`. It requires protoc 29.3, protoc-gen-go v1.36.5, and protoc-gen-go-grpc 1.5.1, and adds the Go install directory (`GOBIN`, else `$(go env GOPATH)/bin`) to `PATH` for the plugins.

## Contract

The BFF HTTP contract, including the POV routes, is in [specs/http/bff.md](specs/http/bff.md). Client commands are recorded in [specs/adr/0008-client-command-grpc-outbox.md](specs/adr/0008-client-command-grpc-outbox.md). OpenTelemetry is not instrumented.
