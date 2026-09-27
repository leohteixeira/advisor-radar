---
title: 'Serve the live queue from the BFF'
type: 'feature'
created: '2026-09-27'
status: 'done'
review_loop_iteration: 0
baseline_revision: '8fb25c5b6f7cd2fe04c44c11e5cdfd05ee8d58f9'
followup_review_recommended: true
context: []
warnings:
  - oversized
deferred:
  - summary: >-
      The SSE ring drop after 64 events is not executed by a test.
    evidence: |-
      Catch-up tests apply two events. Dropping the newest id instead of the oldest would leave go test green.
    location: >-
      internal/bff/board.go
    severity: medium
---

<intent-contract>

## Intent

**Problem:** The advisor queue has no HTTP surface, so a new alert or triaged message cannot appear without a reload.

**Approach:** The BFF serves the queue as versioned JSON and pushes each new signal on one SSE stream. It keeps that board only in memory. It has no database.

## Boundaries & Constraints

**Always:** `GET /v1/queue` returns JSON `{"items":[...]}`. `GET /v1/queue/stream` is `text/event-stream`. The SSE event name is `signal`. Each event has an `id`. A client that sends `Last-Event-ID` receives every retained event after that id. The payload fields match `SIGNALS` and `STREAM` in `docs/design/mock-data.js`. The board starts with seed signals `s01` through `s17`. `alert.raised` and `message.triaged` append one signal and emit one SSE event. A second delivery of the same `event_id` changes nothing. Alert card kinds stay `saque`, `queda`, `aporte`, `segmento`, and `contato`. Wire intents become the Portuguese labels in `ux.md`. The listen address comes from `BFF_HTTP_ADDR` and binds on all interfaces when set (`0.0.0.0:8400` in the example). With `BFF_HTTP_ADDR` unset, the process logs `service=bff` and waits for a signal. With `BFF_BROKER_URL` unset, no broker is dialed. `go test` must not dial ports 5435, 5673, or 8400. Wrap errors with `%w`. Pass `context.Context` into I/O. Shutdown closes listeners and subscribers.

**Never:** Do not add a database, a migration, or a SQL driver call in the BFF. Do not add gRPC. Do not build the React queue, Contatado, Adiar 1 h, filters, detail, timeline, review, or the manager panel. Do not edit `internal/advisory`, `internal/triage`, `internal/triagepipe`, `internal/cases`, or `internal/outbox`. Do not read or write `.env`. Do not log customer text.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Seed board | No events applied | `GET /v1/queue` has 17 items and ids `s01` through `s17` | No error expected |
| Alert | One `alert.raised` for customer `c19`, kind `saque`, event id `al-md-n02-withdrawal` | Queue gains that id once. One SSE event named `signal` carries the same id and `alert` `saque` | No error expected |
| Message | One `message.triaged` with intent `reclamacao` and event id `tr-md-n01` | Queue gains that id. Intent label is `Reclamação`. One SSE `signal` event | No error expected |
| Duplicate | The same alert event id again | Item count stays the same. No second SSE event | No error expected |
| Catch-up | Client sends `Last-Event-ID` of the first live event | The stream sends only later retained events | No error expected |
| Unknown cursor | `Last-Event-ID` is not in the ring | The stream sends the retained window | No error expected |
| Bad body | Broker delivery is not the envelope | No item is added | Permanent failure, not retried by the test double |
| No listen | `BFF_HTTP_ADDR` unset | Process logs `service=bff` and does not bind | Wait for cancel |
| Disconnect | Subscriber context canceled | That subscriber ends. Other subscribers still receive | Returned context error |

</intent-contract>

## Code Map

- `cmd/bff/main.go` — today `proc.Run`. Log `service=bff` before any env check, as `cmd/advisory/main.go` does. Listen only when `BFF_HTTP_ADDR` is set. Consume only when `BFF_BROKER_URL` is set.
- `internal/proc/commands_test.go` — starts `bff` with no env and requires a JSON log `service=bff` plus exit 0 on SIGINT and SIGTERM.
- `docs/design/mock-data.js` lines 63–91 — `SIGNALS` `s01`–`s17` and `STREAM` field names. SSE comment on line 1 names the event `signal`.
- `ux.md` "Queue and live updates" — event name `signal`, payload shape, `Last-Event-ID`.
- `architecture.md` — BFF persistence is nothing. HTTP and SSE port 8400. Do not add gRPC in this story.
- `internal/event/event.go` — routing keys `alert.raised` and `message.triaged`.
- `internal/advisory/apply.go` `AlertPayload` — `kind`, `rule`, `amount`, `before`, `after`, `from`, `to`, `days`. Kinds are already the card words.
- `internal/triagepipe/apply.go` `TriagedPayload` — `text`, `intent`, `intent_prob`, `frustration`, `churn_risk`, `wants_human`, `degraded`, `channel`.
- `.env.example` — add `BFF_HTTP_ADDR` and `BFF_BROKER_URL` with the local example values. Do not put a secret there.

## Tasks & Acceptance

**Execution:**
- `internal/bff/board.go` — in-memory board, seed `s01`–`s17`, apply alert and triaged message, duplicate `event_id` is a no-op, bounded ring for SSE ids
- `internal/bff/board_test.go` — matrix rows for seed, alert, message, duplicate, and bad body, with no sockets
- `internal/bff/http.go` — `GET /v1/queue` and `GET /v1/queue/stream` using `httptest` in the test file
- `internal/bff/http_test.go` — queue JSON, SSE event name `signal`, `Last-Event-ID` catch-up, unknown cursor, subscriber cancel
- `cmd/bff/main.go` — log, optional listen, optional consume, graceful shutdown
- `.env.example` — `BFF_HTTP_ADDR=0.0.0.0:8400` and `BFF_BROKER_URL=amqp://localdev:localdev@127.0.0.1:5673/`

**Acceptance Criteria:**
- Given the seed board, when `GET /v1/queue` is called, then the body lists `s01` through `s17` and the process has no database.
- Given a live `alert.raised`, when a client is on `/v1/queue/stream`, then one SSE event named `signal` arrives and a reload of `/v1/queue` still contains that signal.
- Given `BFF_HTTP_ADDR` is unset, when the process receives SIGTERM, then it exits 0 and has logged `service=bff`.

## Spec Change Log

## Review Triage Log

### 2026-09-27 — Review pass
- verdicts: 26 findings — high 0, medium 6, low 14, false 6, maybe-false 0
- findings:
  - `[false]` `[reject]` live signals omit `ago` — `STREAM` objects in `mock-data.js` have no `ago`. That field is only on the seed `SIGNALS`.
  - `[medium]` `[patch]` `ChurnConf` followed `intent_prob` — `STREAM` n01 is alta on intent and média on churn. `ChurnConf` now uses `confidenceBand(churn_risk)`.
  - `[low]` `[reject]` live `dist` has one key — `message.triaged` does not carry the other intents. Inventing them would fake the distribution.
  - `[low]` `[reject]` live alerts omit `reason` — `AlertPayload` has `rule`, not `reason`. The matrix alert row checks id and `saque`.
  - `[low]` `[reject]` `Items` shares `Dist` maps — HTTP encodes the copy and does not mutate it. A deep copy is an extra guard.
  - `[low]` `[reject]` the item list is unbounded — the demo board is the seed plus the market-day burst. A retention policy is new behavior.
  - `[low]` `[reject]` the consumer has no dead-letter queue or reconnect — the same shape as the earlier consumers. Adding a dead-letter topology is a new surface.
  - `[low]` `[reject]` the SSE stream has no heartbeat and ignores write errors — a comment frame and a log are new. The handler already ends the subscriber on cancel.
  - `[low]` `[reject]` there is no CORS header — the React app is story 7. This story's client is `httptest`.
  - `[medium]` `[patch]` message card fields were unasserted — `TestBoard_Message` now checks `Human`, `Fallback`, `Channel`, and `ChurnConf`.
  - `[false]` `[reject]` a marshal failure leaves the item without an SSE event — `Signal` is only maps, numbers, strings, and bools. `json.Marshal` does not fail on that value.
  - `[low]` `[reject]` a live id equal to `s01` duplicates the seed — published ids are `al-` and `tr-`. Seeding those ids into `seen` would be a new guard.
  - `[low]` `[reject]` a newline in `event_id` breaks the SSE frame — our publishers do not emit that id. Rejecting it would be a new guard.
  - `[low]` `[reject]` a failed JSON encode is ignored — the client already disconnected. Returning 500 after a partial body does not repair it.
  - `[low]` `[reject]` shutdown can nack one in-flight delivery — the next loop sees `ctx.Done` and returns. One requeue on the way out is the existing consumer pattern.
  - `[low]` `[reject]` `commands_test` inherits `BFF_HTTP_ADDR` — this shell and CI do not set it. The test passed without binding. Clearing env would be a new guard around a path that is already idle.
  - `[medium]` `[patch]` `Human`, `Fallback`, `Channel`, and `ChurnConf` had no assertions — same `TestBoard_Message` fix as above.
  - `[medium]` `[patch]` SSE tests ignored `Content-Type` — the three stream tests now require `text/event-stream`.
  - `[medium]` `[defer]` the ring of 64 events is untested — recorded in `deferred`. Catch-up and the unknown cursor are covered with two events.
  - `[medium]` `[patch]` `ChurnConf` used the intent band — same `confidenceBand(churn_risk)` fix as the first patch row.
  - `[false]` `[reject]` the alert row does not assert the SSE name — `TestHTTP_LiveSSEThenQueue` requires `event: signal` for `al-md-n02-withdrawal`. The handler writes that name for every signal.
  - `[false]` `[reject]` a bad body is not nacked on AMQP — the matrix says the test double does not retry. `TestBoard_BadBody` checks `IsPermanent`.
  - `[false]` `[reject]` the unset address path has no test in this diff — `internal/proc/commands_test.go` already starts `bff` with no env and requires `service=bff` plus exit 0. That package passed.
  - `[low]` `[reject]` listen shutdown has no dedicated test — the matrix row is the unset-address process, which `commands_test` covers.
  - `[low]` `[reject]` the seed is not compared to `mock-data.js` at test time — the seventeen ids and the field names are in `TestHTTP_QueueJSON` and the seed literal.
  - `[false]` `[reject]` an unset HTTP address must idle even when the broker URL is set — the matrix only requires no bind. Consume stays gated on `BFF_BROKER_URL`.

## Design Notes

The in-memory board and the SSE ring are the live connection, not a database. Nothing is written to PostgreSQL, Elasticsearch, or a file. gRPC aggregation of other services is outside this story; the signal already carries `client` from the event. Contatado and Adiar 1 h stay in story 7.

Map wire intents with the `ux.md` table (`reclamacao` → `Reclamação`, and the other seven). `churn` is true when `churn_risk` is at least 0.5. `human` is true when `wants_human` is at least 0.5. `fallback` is `degraded`. Frustration on the card is the float truncated to an integer from 0 through 3.

The ring holds 64 events. Publishing past that drops the oldest id. A cursor older than the ring uses the unknown-cursor row.

## Verification

**Commands:**
- `gofmt -l .` -- expected: empty
- `go mod tidy && git diff --exit-code go.mod go.sum` -- expected: clean
- `go vet ./...` -- expected: exit 0
- `go build ./...` -- expected: exit 0
- `mkdir -p .gotmp && GOTMPDIR="$PWD/.gotmp" go test -race -shuffle=on ./...` -- expected: pass

## Auto Run Result

**Summary:** The BFF serves the seed queue at `GET /v1/queue` and pushes new alerts and triaged messages on `GET /v1/queue/stream` as SSE events named `signal`. The board and the catch-up ring stay in memory. There is no database.

**Files:**
- `internal/bff/board.go` — seed `s01`–`s17`, idempotent apply, SSE ring
- `internal/bff/http.go` — versioned JSON and the SSE stream
- `internal/bff/board_test.go` and `internal/bff/http_test.go` — matrix rows
- `cmd/bff/main.go` — log `service=bff`, listen only with `BFF_HTTP_ADDR`, consume only with `BFF_BROKER_URL`
- `.env.example` — local listen address and broker URL

**Review:** Two medium groups were patched: `ChurnConf` now follows `churn_risk`, and the message-card plus `text/event-stream` assertions were added. The 64-event ring drop was deferred. Rejected findings were either untrue (`ago` is not on `STREAM`, marshal of a signal does not fail) or low items whose fix would add a guard, CORS, or a dead-letter topology.

**Follow-up review:** true. Patched medium groups: 2. High patches: 0. Unverified risk: dropping the oldest SSE id after 64 events is not executed by `go test`.

**Verification:** `gofmt -l .` empty. `go mod tidy` left `go.mod` and `go.sum` clean. `go vet ./...` and `go build ./...` exited 0. `GOTMPDIR=$PWD/.gotmp go test -race -shuffle=on -count=1 ./...` passed after the review patches.

**Residual risk:** A live AMQP consume is not dialed by `go test`. The ring-drop path is untested.
