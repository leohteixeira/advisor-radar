---
title: 'Show signal detail and the customer timeline'
type: 'feature'
created: '2026-09-27'
status: 'done'
review_loop_iteration: 0
baseline_revision: 'e564663779dca3e3be33507f0df5bc5d9b15d444'
followup_review_recommended: true
context: []
warnings:
  - multiple-goals
  - oversized
deferred: []
---

<intent-contract>

## Intent

**Problem:** Opening a signal or a case shows only a name and a quote, and the advisor cannot read or search one customer's timeline.

**Approach:** Message, alert, and case details replace that thin view. The timeline indexer keeps the customer 360, and the BFF serves it. A text query returns the matching entries. The browser still calls only the BFF.

## Boundaries & Constraints

**Always:** Details are a message, an alert, or a case. The 360 for `c01` shows account events, messages, cases, and notes. Chips are single-select: Todos, Conta, Mensagens, Casos, Notas. Search is a case-insensitive substring of title, text, and meta. A read of the indexer is gRPC. The BFF stores nothing. A second delivery of the same `event_id` does not add a second row. With `TIMELINE_GRPC_ADDR` unset, the indexer logs `service=timeline-indexer` and does not bind. With the BFF target unset, `GET /v1/customers/{id}/timeline` still returns the `c01` seed. UI copy is Portuguese. Touch targets are at least 44px. `go test` must not dial 5435, 5673, 8400, 8420, or 9201. Wrap errors with `%w`. Pass `context.Context` into I/O. Do not log and return the same error.

**Never:** Do not add a database to the BFF. Do not query Elasticsearch from the BFF. Do not build the analyst review queue, the manager panel, or `/demo`. Do not add a note-write API. Do not read or write `.env`. Do not log customer text. Do not edit `internal/triage` or `internal/outbox`. Do not remove Elasticsearch from Compose.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Message | Open s01 | The quote, Reclamação, Frustrado, Mensagem com risco, and Precisa de revisão | No error expected |
| Fallback | Open s06 | Classificação simplificada | No error expected |
| Alert | Open s11 | Saque relevante and the reason text | No error expected |
| Case | Open k1042 | Em atendimento on the four-step rail. k1031 stays off the board | No error expected |
| Timeline | `GET /v1/customers/c01/timeline` | Seven seed rows, including the Orlando note, the DARF case, a saque, and a message | No error expected |
| Search | `q=Orlando` on c01 | The note remains. The saque row does not | No error expected |
| Chip | `kind=notas` on c01 | Only the note | No error expected |
| Empty | Customer `c99` | `{"items":[]}` | No error expected |
| Idempotent | The same `event_id` delivered twice | One timeline row | No error expected |
| No listen | `TIMELINE_GRPC_ADDR` unset | Log `service=timeline-indexer` and do not bind | Wait for cancel |

</intent-contract>

## Code Map

- `docs/design/mock-data.js` lines 63–81 and 126–136 — s01, s06, s11, and `TIMELINES.c01` (seven rows: mensagem, mensagem, nota, saque, caso, aporte, telefone).
- `internal/bff/cases.go` — k1042 is state 1 (Em atendimento), client `c02`. k1031 is Resolvido and already filtered off the board.
- `web/src/components/DetailModal.tsx` — thin detail. Replace it with message, alert, and case detail, plus a control that opens the 360.
- `web/src/screens/QueueScreen.tsx` — open, desktop dialog, phone replace. Keep the queue behavior from story 7.
- `cmd/timeline-indexer/main.go` — today only `proc.Run`. Listen and consume only when the env vars are set.
- `internal/bff/http.go` — add `GET /v1/customers/{id}/timeline`. Do not store the rows.
- `internal/event/event.go` — envelope fields `event_id`, `occurred_at`, `customer_id`, `schema_version`. The name is the routing key.
- `compose.yaml` — Elasticsearch is already on 9201. Do not edit it except to leave it in place.
- `internal/proc/commands_test.go` — timeline-indexer must still exit 0 on SIGTERM when its URLs are unset.

## Tasks & Acceptance

**Execution:**
- `internal/timeline/index.go` — memory index, seed `c01`, search, chip filter, idempotent apply of `account.event.recorded`, `message.triaged`, `alert.raised`, `case.opened`, and `case.status.changed`
- `internal/timeline/index_test.go` — the matrix rows for search, chip, empty customer, and the second delivery
- `internal/timeline/elastic.go` — HTTP adapter used only when `ELASTICSEARCH_URL` is set. Cover the request with `httptest`, not port 9201
- `proto/timeline/v1/timeline.proto` — `Search` request (`customer_id`, `query`, `kind`) and response items. Commit the generated Go. `protoc` is not on the image
- `cmd/timeline-indexer/main.go` — gRPC on `TIMELINE_GRPC_ADDR` only when set. Broker only when `TIMELINE_BROKER_URL` is set. Elasticsearch only when `ELASTICSEARCH_URL` is set. Otherwise the memory seed
- `internal/bff/timeline.go` — client interface. Unset `TIMELINE_GRPC_TARGET` serves the same seed. Set target calls gRPC and does not keep a copy
- `internal/bff/http_test.go` — Orlando search, notas chip, unknown customer
- `web/src/screens/QueueScreen.test.tsx` — s01 review and risk, s06 simplified, s11 reason, k1042 rail, c01 search hides the saque
- `.env.example` — `TIMELINE_GRPC_ADDR=0.0.0.0:8420`, `TIMELINE_GRPC_TARGET=127.0.0.1:8420`, `TIMELINE_BROKER_URL=amqp://localdev:localdev@127.0.0.1:5673/`, `ELASTICSEARCH_URL=http://127.0.0.1:9201`

**Acceptance Criteria:**
- Given the seed board, when the advisor opens s01, s11, and k1042, then each detail matches the matrix row for that kind.
- Given `c01`, when the advisor searches Orlando, then the note is shown and the saque row is not.
- Given `TIMELINE_GRPC_ADDR` is unset, when the process receives SIGTERM, then it exits 0 and has logged `service=timeline-indexer`.

## Spec Change Log

## Review Triage Log

### 2026-09-27 — Review pass
- verdicts: 35 findings — high 0, medium 18, low 12, false 5, maybe-false 0
- findings:
  - `[medium]` `[patch]` an Elasticsearch failure after apply made the redelivery a no-op — `Forget` drops the event id before the requeue nack.
  - `[low]` `[reject]` Search reads memory while Elasticsearch is only written — the tasks make the memory index the read path and the HTTP adapter a PUT. Loading the index back would be a new query.
  - `[low]` `[reject]` the consumer has no dead-letter queue — the same shape as the earlier consumers. A new dead-letter topology is a new surface.
  - `[medium]` `[patch]` message and alert detail dropped Contatado — the line is back when `contacted_at` is set.
  - `[medium]` `[patch]` a failed timeline fetch left the previous rows up — the error path now clears the list.
  - `[low]` `[reject]` search has no debounce — a timer is new behavior. The seed query is one short field.
  - `[low]` `[reject]` the customer id is not escaped in the URL — seed ids are `c01`. Escaping is a new guard.
  - `[medium]` `[patch]` Abrir caso on desktop left the detail on the removed signal — that selection is cleared.
  - `[low]` `[patch]` the dialog aria-label became the generic word Detalhe — it is the customer name again.
  - `[false]` `[reject]` `message.received` is not consumed — the tasks index `message.triaged`. Indexing both would show the message twice.
  - `[low]` `[reject]` ages stay in minutes — hour and day labels are a new format. The seed field is already minutes.
  - `[low]` `[reject]` the subtitle says Assessora — the copy is cosmetic for this demo's advisors.
  - `[low]` `[reject]` the empty line can flash before the first response — a loading state is new. The list fills as soon as the seed returns.
  - `[false]` `[reject]` gRPC search uses `%v` inside `status.Errorf` — that status is the process boundary. `%w` stays on the Go errors inside the service.
  - `[low]` `[reject]` the new routes have no OpenTelemetry spans — this story's contract does not add a trace. Spans would be a new surface.
  - `[medium]` `[patch]` the 360 chips had no screen test — Notas is clicked and the saque row is gone.
  - `[medium]` `[patch]` IndexDoc failure skipped the retry — same `Forget` before nack as the consumer row.
  - `[medium]` `[patch]` desktop Abrir caso kept the old detail — same selection clear.
  - `[medium]` `[patch]` a failed fetch kept stale rows — same clear as the timeline component.
  - `[low]` `[reject]` an event id with a slash would break the Elasticsearch path — seed ids do not contain one. Path escaping is a new guard.
  - `[medium]` `[patch]` Search ignored a canceled context — it returns `ctx.Err()` before the scan.
  - `[medium]` `[patch]` detail no longer showed Contatado — same restored line.
  - `[medium]` `[patch]` only `account.event.recorded` was applied in tests — message, alert, and both case events now assert kind and a chip hit.
  - `[medium]` `[patch]` the BFF gRPC client was never called — a bufconn test checks Orlando and notas through `GRPCTimeline`.
  - `[medium]` `[patch]` Conta, Mensagens, and Casos could return nothing and stay green — Conta must include saque and aporte, and the other two chips must be non-empty and exclusive.
  - `[medium]` `[patch]` the screen never clicked a chip — same Notas click as the UI row.
  - `[medium]` `[patch]` a timeline Search error was not a 502 — a failing fake client now requires 502.
  - `[medium]` `[patch]` the rewritten detail dropped Contatado — same restored line.
  - `[low]` `[reject]` the screen fixture has three timeline rows and the HTTP seed has seven — the seven-row row is the GET test. The screen test checks the Orlando search.
  - `[medium]` `[patch]` the chip control was not clicked — same Notas screen test.
  - `[low]` `[reject]` idempotency is not proven on a live AMQP delivery — two applies on the memory index leave one row. `go test` must not dial 5673.
  - `[false]` `[reject]` an unset `TIMELINE_GRPC_ADDR` has no test in this diff — `internal/proc/commands_test.go` starts timeline-indexer with no env and requires `service=timeline-indexer` plus exit 0. That package passed.
  - `[false]` `[reject]` the 360 is a fourth selection type — the approach is the customer timeline. It is not a fourth signal kind.
  - `[false]` `[reject]` the unset target serves the seed inside the BFF process — the matrix requires that GET to return the `c01` seed. The rows are not a database.
  - `[medium]` `[patch]` the timeline search input was 38px — it uses `var(--ar-touch)`.

## Design Notes

The indexer owns the 360. The BFF calls `Search` over gRPC when `TIMELINE_GRPC_TARGET` is set and otherwise returns the in-process seed. That seed is the same seven `TIMELINES.c01` rows, not a database. Account-event kinds `saque`, `aporte`, `queda`, `segmento`, and `contato` belong to the Conta chip. `telefone` is visible only on Todos. Notes are the seed row; this story does not create one.

gRPC status codes come from `google.golang.org/grpc/status`. The server stops with `GracefulStop` and a timeout fallback. Tests use `bufconn`, not a published port. Do not store `context.Context` in a struct.

## Verification

**Commands:**
- `gofmt -l .` -- expected: empty
- `go mod tidy && git diff --exit-code go.mod go.sum` -- expected: clean
- `go vet ./...` -- expected: exit 0
- `go build ./...` -- expected: exit 0
- `mkdir -p .gotmp && GOTMPDIR="$PWD/.gotmp" go test -race -shuffle=on ./...` -- expected: pass
- `pnpm --dir web test` -- expected: exit 0
- `pnpm --dir web build` -- expected: exit 0

## Auto Run Result

**Summary:** Opening a message, an alert, or a case shows that detail. The customer 360 for Mariana lists the seven seed rows, and a text query returns the matches. The indexer keeps the index. The BFF serves it and stores nothing.

**Files:**
- `internal/timeline/` — memory index, seed `c01`, idempotent apply, Elasticsearch PUT adapter, gRPC search
- `proto/timeline/v1/` and `gen/timeline/v1/` — committed `Search` stubs
- `cmd/timeline-indexer/main.go` — listen, consume, and Elasticsearch only when the URLs are set
- `internal/bff/timeline.go` and `internal/bff/http.go` — `GET /v1/customers/{id}/timeline`
- `web/src/components/DetailModal.tsx` and `web/src/components/Timeline360.tsx` — message, alert, case, and the 360
- `.env.example` — timeline gRPC, broker, and Elasticsearch URLs

**Review:** Eleven medium groups were patched: the Elasticsearch retry forgets the event id, Contatado is back on the detail, a failed fetch clears the list, Abrir caso clears that selection, chip tests cover Conta, Mensagens, Casos, and the Notas click, Search honors a canceled context, the other event names are applied in tests, the BFF gRPC client is covered with bufconn, a Search error is 502, and the search field is 44px. The dialog name was restored with the detail. Rejected findings were either untrue (`message.received` would duplicate the triaged row, the unset address is already in `commands_test`) or low items whose fix would add a dead-letter queue, a debounce timer, or a load-from-Elasticsearch query.

**Follow-up review:** true. Patched medium groups: 11. High patches: 0. Unverified risk: `Search` does not read Elasticsearch back, so restarting the indexer drops every row except the `c01` seed even after a successful PUT.

**Verification:** `gofmt -l .` empty. `go mod tidy` left the module files ready to commit. `go vet ./...` and `go build ./...` exited 0. `GOTMPDIR=$PWD/.gotmp go test -race -shuffle=on -count=1 ./...` passed. `pnpm --dir web test` (12 tests) and `pnpm --dir web build` exited 0. In the browser, s01 opened onto the quote, the 360 listed seven rows, Notas left the Orlando note, and k1042 showed Em atendimento on the four-step rail while the board stayed.

**Residual risk:** A live AMQP consume and a live Elasticsearch read are not dialed by `go test`.
