# Architecture

Phase-2 delta. Services, events, and rules that this file does not change stay in the adopted phase-1 `architecture.md`. Diagrams are in `architecture-diagrams.md`.

## Command path

A client action changes state. The BFF has no database, so it cannot publish through an outbox.

1. The BFF accepts the versioned HTTP command and calls account-sim over gRPC with the caller's deadline.
2. account-sim validates, writes the new account state and the event in one transaction, and returns `event_id`. A withdrawal above available cash is refused and writes nothing.
3. The BFF responds `202 Accepted` with `event_id` and no protocol field. The UI formats the protocol as the first two UUID groups of that `event_id`, uppercased. Triage, rules, cases, and the queue stay asynchronous on the phase-1 events. Progress after that response is the Bastidores SSE stream.
4. A repeated POST with the same `Idempotency-Key` returns the original result and does not append a second outbox row.

Write this as ADR 0008 under `specs/adr/` before the code, as an extension of ADR 0001.

## Services

| Service | Change |
|---|---|
| account-sim | Owns fictional account state: total assets and four class balances in integer cents (ações, ETFs, renda fixa, caixa). No individual asset. gRPC commands for the four actions, plus an account query. Dedupes on the idempotency key. A withdrawal debits caixa and total assets and leaves the other classes untouched. |
| bff | New POV HTTP routes. gRPC to account-sim for the command and the account. Builds recent activity and message history from timeline-indexer. Serves the Bastidores SSE from an in-memory per-customer hub. Still stores nothing durable. |
| advisory | Updates `book.aum` and `book.segment` from `account.event.recorded`. Today the book comes only from the seed. |
| triage, cases, timeline-indexer | No contract change. POV events are ordinary events. |
| web | Selection, client list, and client app under `/advisor-radar`. The team queue link is `/advisor-radar/fila`. |

account-sim is the source of truth for the balance. Advisory does not write the balance on its own.

## Events

No new event name. Every body still carries `event_id`, `occurred_at`, `customer_id`, and `schema_version`. Trace context stays on the headers.

| Action | Event | Payload added for the POV |
|---|---|---|
| Deposit | `account.event.recorded` | `kind: aporte`, `amount`, `before`, `after`, origin |
| Withdrawal | `account.event.recorded` | `kind: saque`, `amount`, `before`, `after`, destination |
| Free message | `message.received` | `channel: chat` or `e-mail`, `text` |
| Complaint | `message.received` | `channel: chat`, `text` (preset or free text) |

POV account events use `schema_version` 2. `amount`, `before`, `after`, and the class balances are integer USD cents. Origin and destination ride on that version. `schema_version` 1 stays whole dollars, which is what the current seed and `AccountPayload` already store. Consumers accept both versions and scale by version before a rule runs. A version-2 update of `book.aum` converts cents into that dollar book. A deposit of USD 10,000 is `1000000` cents.

## HTTP

Prefix is `/v1`, the prefix the BFF already uses. The animation label `POST /api/v1/client-pov/{id}/complaints` is not the contract.

| Route | Result |
|---|---|
| `GET /v1/client-pov/customers` | The three POV clients: segment, assets, base SLA, advisor |
| `GET /v1/client-pov/customers/{id}` | Home: assets, cash available to withdraw, allocation, advisor, recent activity, message history |
| `POST /v1/client-pov/customers/{id}/deposits` | `202` with `event_id`. Body amounts are integer cents. |
| `POST /v1/client-pov/customers/{id}/withdrawals` | `202`, or `422` when the amount exceeds caixa |
| `POST /v1/client-pov/customers/{id}/messages` | `202` |
| `POST /v1/client-pov/customers/{id}/complaints` | `202` |
| `GET /v1/client-pov/customers/{id}/stream` | SSE. In-memory hub for that customer. |

Every POST requires `Idempotency-Key`. The BFF checks the rate limit before the gRPC call. Over 10 new commands in 60 seconds, or 40 in 24 hours, it returns `429` and does not call account-sim. Replaying a known key returns the original result and does not spend the budget.

The hub does not persist. The outbox step is marked when the command returns `event_id`. The published step is marked when the BFF consumes that `event_id` on `account.event.recorded` or `message.received`. For a message, `message.triaged` marks classified and on the queue. For an account action, `alert.raised` marks evaluated and on the queue; if no rule fires, those steps stay waiting.

## Seed

Reuse the advisory book's `customer_id` values (fixed UUIDv7, ADR 0007) for Fernanda Lima, Thiago Azevedo, and Mariana Costa. Reseed restores these three accounts so the demo can be repeated. Thiago keeps the segment-upgrade history already in the seed.

Activity rows in the artboard stay illustrative. Class balances are integer cents and sum to assets. Caixa is the cash available to withdraw.

| Client | Segment | Ações | ETFs | Renda fixa | Caixa | Assets |
|---|---|---:|---:|---:|---:|---:|
| Mariana Costa | Singular | 9,090,000 | 6,060,000 | 3,680,000 | 6,000,000 | 24,830,000 |
| Fernanda Lima | Essencial | 164,000 | 369,000 | 172,200 | 114,800 | 820,000 |
| Thiago Azevedo | Advance | 204,000 | 544,000 | 0 | 6,052,000 | 6,800,000 |

Mariana's caixa is USD 60,000 so a withdrawal above USD 49,660 can fire "saque relevante". The US$ 20,000 chip is under that line; "Tudo" is over it. Her other classes keep the artboard ratio 42:28:17 of the remainder. Fernanda and Thiago match the artboard percentages. A USD 10,000 deposit on Fernanda fires "aporte grande" and crosses the USD 10,000 segment band. A zero class is stored and hidden on the bar.

## Observability

The trace starts on the BFF HTTP request, crosses the gRPC call, is stored with the outbox row, and continues on the RabbitMQ headers. That `trace_id` is what Bastidores shows once OpenTelemetry is instrumented. It is not instrumented yet. Until then, Bastidores shows `event_id` and the steps the BFF has observed.

New counts: POV actions by type, refusals by rule (withdrawal above available cash), requests collapsed by idempotency.

`slog` JSON. Info lines do not include customer free text or full account values. Do not log tokens, credentials, connection strings, or model keys.

## Cut order and sequence

Cut only in this order:

1. Live Bastidores. The confirmation keeps `event_id` and the link to the queue.
2. Desktop layout of the client app. A centered phone layout covers desktop.
3. Light theme of the client app.
4. Free message. Preset complaints, deposits, and withdrawals remain.

Never cut selection, the client list, a preset complaint, or deposit and withdrawal through gRPC, the outbox, and idempotency.

Day 1: ADR 0008, the three accounts and the seed, the gRPC commands with idempotency, and the advisory book update. Day 2: BFF routes, selection, the client list, the phone client app, backstage, and a rehearsal of the script in `ux.md`.

## Documentation

- ADR 0008: client commands enter the state owner over gRPC; that service publishes through the outbox.
- README updated with selection, the POV, and the demo script.
- POV HTTP contract written next to the current BFF contract.
- `docs/design/clients/README.md` kept as the design reference.
