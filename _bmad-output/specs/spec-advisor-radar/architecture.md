# Architecture

Contract for how Advisor Radar is built. Diagrams live in `architecture-diagrams.md`. Case, SLA, and breaker transitions live in `state-machines.md`.

## Module

One Go module. Commands under `cmd/`: `account-sim`, `advisory`, `triage`, `cases`, `timeline-indexer`, `bff`. Packages under `internal/` by responsibility, short names, no cycles, no `utils` package. Interfaces are small and declared by the consuming package. `context.Context` crosses every I/O boundary. Errors wrap with `%w`. Concurrency is bounded. Worker pools stop on `signal.NotifyContext`.

Copy `radar-triage` into this module when building `triage`. Do not import it across the repository boundary.

The frontend is `web/`: React, TypeScript, Vite. It calls only the BFF.

Record these ADRs under `specs/` before the code that depends on them:

1. Synchronous queries versus asynchronous state changes.
2. Transactional outbox and idempotent inbox.
3. SLA escalation by TTL and dead-letter exchange.
4. Typed decision model for triage, with heuristic fallback.
5. Resilience decorator order and error classes.
6. Persistence per service.

## Services

| Service | Owns | Stores | Speaks |
|---|---|---|---|
| account-sim | Fictional account events and customer messages, including the market-day burst | PostgreSQL outbox | Publishes to RabbitMQ |
| advisory | Book, segment, alert rules | PostgreSQL | gRPC; consumes and publishes |
| triage | Message classification and the resilience stack | PostgreSQL inbox and results | Consumes and publishes |
| cases | Cases, SLA, state machine, escalation | MySQL | gRPC; consumes and publishes |
| timeline-indexer | Customer 360 index | Elasticsearch | Read by the BFF |
| bff | HTTP for the frontend, gRPC aggregation, SSE | Nothing | HTTP and SSE |

Alert rules are one small interface each. gRPC interceptors cover tracing, deadline, and retry. The timeline index is replaceable by PostgreSQL if Elasticsearch is cut; the 360 capability stays.

## Synchronous versus asynchronous

- A read of another service (a customer's book, a case) is gRPC. The caller's deadline propagates.
- A state change is a RabbitMQ event: account event, triaged message, raised alert, case opened or moved, SLA breached.
- Creating a case from an alert is an asynchronous command, not a gRPC call in the alert transaction.

## Events

Every event body carries `event_id`, `occurred_at`, `customer_id`, and `schema_version`. Trace context rides the headers so one trace spans the event, the rule, the alert, and the SSE push.

| Event | Producer | Consumers |
|---|---|---|
| `account.event.recorded` | account-sim | advisory, timeline-indexer |
| `message.received` | account-sim | triage, timeline-indexer |
| `message.triaged` | triage | advisory, cases, timeline-indexer |
| `alert.raised` | advisory | bff, cases, timeline-indexer |
| `case.opened`, `case.status.changed` | cases | bff, timeline-indexer |
| `case.sla.breached` | cases | bff, advisory |

`message.reclassify.requested` is not in the MVP.

Publish through the outbox in the same transaction as the local write. Consume through an inbox keyed by `event_id`: a second delivery is a no-op. Each queue has backoff retries and its own dead-letter queue.

## Alert rules

Thresholds come from the design seed. Amounts are USD.

| Rule | Fires when | Card |
|---|---|---|
| Relevant withdrawal or wire-out | Amount over 20% of assets in 24 hours | Saque relevante |
| Asset drop | Drop over 15% in 5 business days | Queda de patrimônio |
| Large deposit | Deposit greater than the previous assets | Aporte grande |
| Segment change | Assets cross the Essencial/Advance or Advance/Singular bound | Mudança de segmento |
| Silence | No contact for more than 90 days | Sem contato há muito tempo |

A triaged message with churn risk is shown as "Mensagem com risco". That is a presentation of `message.triaged`, not a sixth account rule. Segment bounds are in the SPEC assumptions.

## Triage

Keep the prototype types and behavior below. Change only what the resilience section requires.

`Classifier.Classify(ctx, Message) (Result, error)`.

`Message` carries `ID`, `Channel`, `Text`, and `Previous` (recent texts from the same customer). The model state is channel, message text, and those previous texts. Numbers and dates never enter that state.

Wire intents, question text, keyword tables, and the HTTP contract are in `triage.md`. Display labels are in `ux.md`.

`Result` carries intent, probability of the chosen intent, frustration score from 0 to 3, churn probability, human-request probability, classifier name (`jev` or `heuristic`), model version, degraded flag, latency, and gateway cost when present. Record `model` from the response on every result. Do not depend on a confidence field. Human review uses the chosen intent's probability and the 0.85 gate. The degraded flag does not add a second gate.

`Fallback` applies a timeout to the primary. If the caller's context is already canceled, return that error and do not classify. Any other primary error runs the heuristic, sets `Degraded`, and invokes the degrade hook. The prototype's `NeedsReview` treats degraded as review; whether the product queue does too is an open question in the SPEC.

The labeled set is the 16 Portuguese messages in `radar-triage/testdata/messages.json`, copied with the package. The probe prints intent, churn, and human accuracy, p50 and p95 latency, and cost for Jev and the heuristic. Boolean probe cutoff is 0.5. The review gate stays 0.85. Known baseline after the label fix: Jev 16/16 on intent, churn, and human; heuristic 11/16 on intent; p50 about 290 ms.

## Resilience

The Jev call is the only synchronous external dependency. It must not stall the consumer and must not spend budget without a limit. A gateway budget cap is set before the simulator is left running.

Outside-in, per message:

| Layer | Role |
|---|---|
| Fallback | Any failure becomes a degraded heuristic result, with the reason recorded |
| Bulkhead | Semaphore on in-flight model calls, equal to the consumer prefetch |
| Circuit breaker | Opens after 3 consecutive message failures, or 5 failures in the last 10, or an immediate quota error; then answers without calling the API |
| Retry | Limited attempts, only inside the remaining deadline |
| Rate limiter | Local token bucket; each real attempt takes one token |
| Timeout | 2 seconds on that HTTP attempt |

The breaker sits outside retry so one message is one breaker outcome. The limiter sits inside retry so each attempt spends a token. The prototype retries inside `jev.Client`. Move that retry out to this stack so a single HTTP attempt is what the limiter and the timeout see.

Retry policy: honor a valid `Retry-After` on HTTP 429; otherwise exponential backoff with jitter. If the wait would pass the remaining deadline, stop and fall back. Do not retry other 4xx.

Breaker failure classes: 429, 5xx, timeout, and network errors. Also 401, 402, and 403, because they repeat. HTTP 400 `invalid_request` is our payload: fall back, log an error, and do not count it as a provider failure. `402` with `quota_for_entity_exceeded` opens the breaker immediately and raises an operational alert.

Open the breaker after 3 consecutive message-level failures, or when 5 of the last 10 message outcomes failed. `402 quota_for_entity_exceeded` opens it immediately. Stay open for 15 seconds, then allow one half-open trial. A success closes. A failure reopens for another 15 seconds. While open, classify with the heuristic immediately.

Rate limit and prefetch are environment variables. Pick the first rate by watching 429s in the k6 run. The gateway does not publish a fixed number. The bulkhead limit equals prefetch.

Libraries: `github.com/sony/gobreaker/v2` for the breaker, `golang.org/x/time/rate` for the bucket. Bulkhead and retry use the standard library.

Metrics: breaker state gauge and transition counter; fallbacks by reason (timeout, open breaker, local rate limit, 429, budget, network, invalid payload); attempts per message and 429 responses; time waiting on the limiter; bulkhead occupancy. Also triage latency, fallback rate, review rate, model version, and provider.

Tests against a fake HTTP server: 429 with and without `Retry-After`, 402, repeated 5xx opening the breaker, half-open recovery, deadline exhausted while waiting, and caller cancellation. Caller cancellation still skips the fallback.

## Observability and operations

OpenTelemetry on every service. Logs are `log/slog` JSON. Dashboard on `grafana/otel-lgtm`: consume lag, p95 from event to on-screen alert, DLQ depth, cases with SLA at risk, fallback rate, review rate.

k6 produces a written throughput number. The runbook is one page and covers three situations: DLQ growing, model unavailable (breaker open), gateway budget exhausted.

The README states that a real deployment would need a review of customer text sent to a third party. This demo does not send real customer text.

## Infrastructure

Docker Compose on one Hostinger VPS. The public name is `leohts.tech`. Caddy or Traefik terminates TLS for that name. Elasticsearch heap is 512 MB. Plan for about 4 GB free for the full set. Bind development servers to `0.0.0.0`.

## Advisor actions

"Contatado" and "Adiar 1 h" are rows in advisory's PostgreSQL, keyed by signal id. A snooze stores `snoozed_until` one hour after the action. "Contatado" stores `contacted_at`, and that timestamp is the first-contact measure on the manager panel. Undo deletes the row. The BFF writes and reads these through advisory and stores nothing. A reload shows the same actions.

| Surface | Port |
|---|---:|
| Web (Vite) | 3400 |
| BFF HTTP and SSE | 8400 |
| RabbitMQ | 5673 |
| RabbitMQ management | 15673 |
| PostgreSQL | 5435 |
| MySQL | 3307 |
| Elasticsearch | 9201 |

These ports stay off the other portfolio assignments.

## Cut order

1. Elasticsearch. The timeline is read from PostgreSQL.
2. Jev. Heuristic only, with the `Classifier` port left in place.
3. MySQL. Cases move to PostgreSQL.

Do not cut RabbitMQ, gRPC, outbox, inbox, or observability.
