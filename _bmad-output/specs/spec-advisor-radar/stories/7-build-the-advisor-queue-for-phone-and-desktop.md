---
title: 'Build the advisor queue for phone and desktop'
type: 'feature'
created: '2026-09-27'
status: 'done'
review_loop_iteration: 0
baseline_revision: 'c065d9aeb321d453b36af2b38968b874a248532e'
followup_review_recommended: true
context: []
warnings:
  - oversized
deferred:
  - summary: >-
      The PostgreSQL signal_action table is not executed by go test.
    evidence: |-
      Action tests use MemoryActionStore and httptest. ADVISORY_DATABASE_URL stays unset so the suite does not dial port 5435. A wrong column in actions_pgx.go would stay green.
    location: >-
      internal/advisory/actions_pgx.go
    severity: medium
---

<intent-contract>

## Intent

**Problem:** The advisor cannot see the live queue on a phone or as four desktop columns, and Contatado and Adiar 1 h disappear on reload.

**Approach:** A React queue reads only the BFF. Below 900px the board scrolls sideways and a card replaces it. At 900px and above the same board is four columns and the card opens in a modal. Advisory stores the two actions in PostgreSQL. The BFF reads and writes them and stores nothing.

## Boundaries & Constraints

**Always:** The browser calls only the BFF. Columns are Novos sinais, Aberto, Em atendimento, and Aguardando cliente. New signals are messages and alerts that are not snoozed. The three seed cases k1042, k1038, and k1031 sit in the case columns by their state. A new `signal` SSE event stays in an incoming buffer until the advisor taps the pill. `Last-Event-ID` is sent on reconnect. Contatado sets `contacted_at`. Adiar 1 h sets `snoozed_until` to one hour after the action and hides the card until then. Undo deletes that row. A reload shows the same action. The advisory row is keyed by signal id. The BFF reaches advisory over HTTP at `ADVISORY_HTTP_URL` and does not keep the action. With `ADVISORY_HTTP_ADDR` unset, advisory does not bind. With `BFF_HTTP_ADDR` unset, the BFF still only logs and waits. UI copy is Portuguese. Tokens come from `docs/design/handoff/tokens.css`. Touch targets are at least 44px. `go test` must not dial 5435, 5673, or 8400. Wrap errors with `%w`. Pass `context.Context` into I/O.

**Never:** Do not add a database to the BFF. Do not add gRPC. Do not build the analyst review queue, the manager panel, the customer 360 timeline, or `/demo`. Do not read or write `.env`. Do not log customer text. Do not edit `internal/triage`, `internal/triagepipe`, `internal/cases`, or `internal/outbox`.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Board | Seed queue and the three cases | Four column titles. s01 is under Novos sinais. k1042 is Em atendimento, k1038 is Aguardando cliente, k1031 is Resolvido and not on the board | No error expected |
| Phone | Viewport below 900px | One horizontal board. Opening s01 replaces the board | No error expected |
| Desktop | Viewport at 900px | Four columns. Opening s01 keeps the board and shows a dialog | No error expected |
| Live | One SSE `signal` while the board is open | The card is not inserted. The pill shows 1. Tapping the pill inserts it | No error expected |
| Snooze | Adiar 1 h on s01 at 15:00 UTC | `snoozed_until` is 16:00 UTC. s01 leaves the board. Reload still hides it | No error expected |
| Contact | Contatado on s02 | `contacted_at` is set. The card stays and shows Contatado | No error expected |
| Undo | Undo after Contatado | The row is gone. Reload shows no action | No error expected |
| Advisory down | Actions URL unset on the BFF | `POST` returns 503. The board still loads | Returned error |
| No listen | `ADVISORY_HTTP_ADDR` unset | Advisory logs `service=advisory` and does not bind a new HTTP port | Wait for cancel |

</intent-contract>

## Code Map

- `internal/bff/http.go` — `GET /v1/queue` and `GET /v1/queue/stream` already exist. Add action routes and merge advisory actions into the queue. Add `GET /v1/cases` for the three seed cases.
- `internal/bff/board.go` — seed signals s01–s17. Do not persist actions here.
- `cmd/bff/main.go` — call advisory only when `ADVISORY_HTTP_URL` is set.
- `cmd/advisory/main.go` — log `service=advisory` before env checks. Listen on `ADVISORY_HTTP_ADDR` only when set. Action SQL only when `ADVISORY_DATABASE_URL` is set.
- `internal/advisory/apply.go` — do not change alert rules. Add a small actions store beside it.
- `migrations/advisory/001_book.sql` — append `signal_action`, do not rewrite the earlier tables.
- `docs/design/mock-data.js` lines 63–123 — signal fields and cases k1042, k1038, k1031.
- `docs/design/handoff/tokens.css` — import once in the React root.
- `ux.md` "Screens" and "Queue and live updates" — 900px, columns, pill, SSE name `signal`, the two actions, undo.
- `.github/workflows/ci.yml` — `pnpm --dir web install --frozen-lockfile`, test, and build. The lockfile must exist.
- `internal/proc/commands_test.go` — advisory and bff must still exit 0 on SIGTERM when their URLs are unset.

## Tasks & Acceptance

**Execution:**
- `internal/advisory/actions.go` — contact, snooze one hour, undo, list, behind a store interface and an injected clock
- `internal/advisory/actions_test.go` — the snooze, contact, and undo rows with a fake store
- `internal/advisory/actions_http.go` — `PUT /v1/actions/{id}` with body `contact` or `snooze`, `DELETE /v1/actions/{id}`, `GET /v1/actions`
- `migrations/advisory/001_book.sql` — `signal_action(signal_id primary key, contacted_at, snoozed_until)`
- `cmd/advisory/main.go` — optional HTTP listen
- `internal/bff/actions.go` — client interface; unset URL returns an error the handler maps to 503
- `internal/bff/http.go` — merge actions, hide snoozed cards, proxy the three action calls, `GET /v1/cases`
- `internal/bff/http_test.go` — snooze hidden, 503 when the client is unset, cases in columns via JSON
- `web/` — Vite, React, TypeScript, Vitest. Queue at `/`. Four columns, horizontal scroll below 900px, dialog at 900px, pill, Contatado, Adiar 1 h, Desfazer. `pnpm-lock.yaml` committed
- `.env.example` — `ADVISORY_HTTP_ADDR=0.0.0.0:8410` and `ADVISORY_HTTP_URL=http://127.0.0.1:8410`

**Acceptance Criteria:**
- Given the seed board on a 390px viewport, when the advisor opens s01, then the board is gone and the signal is shown.
- Given the same board at 1100px, when the advisor opens s01, then the four columns stay and a dialog shows the signal.
- Given Contatado on s02, when the page is reloaded, then s02 still shows Contatado.
- Given `web/`, when `pnpm --dir web test` and `pnpm --dir web build` run, then both exit 0.

## Spec Change Log

## Review Triage Log

### 2026-09-27 — Review pass
- verdicts: 39 findings — high 0, medium 19, low 13, false 7, maybe-false 0
- findings:
  - `[false]` `[reject]` `lastEventId` is stored and never read — the SSE writer emits `id:` and the browser `EventSource` sends `Last-Event-ID` on its own reconnect. The ref does not have to.
  - `[medium]` `[patch]` tokens were a copy under `web/src/styles/` — `main.tsx` now imports `docs/design/handoff/tokens.css`.
  - `[medium]` `[patch]` card actions and Desfazer were 36px and 32px — those controls use `var(--ar-touch)`. The open control stays `min-height: auto` because it is the card body, which is already taller than 44px.
  - `[false]` `[reject]` publish and consume channels lost their `Close` defers — shutdown still calls `amqpCleanup`, which closes the connection, and that closes the channels with it.
  - `[false]` `[reject]` a process restart drops Contatado when no database URL is set — the matrix reload is a page reload. The code map uses SQL only when `ADVISORY_DATABASE_URL` is set, so `go test` does not dial Postgres. The browser reload kept the row while the process lived.
  - `[medium]` `[patch]` Desfazer cleared the toast when DELETE failed — the catch now leaves the toast in place.
  - `[low]` `[reject]` the matrix says POST and the handler is PUT — the execution tasks specify PUT, and the only client sends PUT. Changing the matrix would edit this spec.
  - `[medium]` `[patch]` advisory action HTTP had no test — `actions_http_test.go` covers contact, snooze, delete, and a bad action.
  - `[low]` `[reject]` the phone detail has no Contatado or Adiar — the phone row only replaces the board. Those buttons stay on the card before it is opened.
  - `[low]` `[reject]` the toast says "contatada" and does not dismiss itself — the copy is cosmetic, and a four-second timer is a new behavior the matrix does not require.
  - `[low]` `[reject]` Abrir caso is only a local `k-local-*` row — the matrix does not require that write, and the design notes keep case state out of PostgreSQL for this story.
  - `[low]` `[reject]` port 8410 is not in Compose — `.env.example` is the config surface this story adds.
  - `[low]` `[reject]` concurrent Contact and Snooze can lose a column — a row lock is a new guard, and the demo is one advisor.
  - `[medium]` `[patch]` Contact left an active snooze, so the card stayed hidden — Contact now clears `SnoozedUntil`, and the store test contacts `s01` after the snooze.
  - `[low]` `[reject]` `GET /v1/queue` returns 502 when a configured advisory list fails — the matrix 503 row is the unset URL, which still loads the board. Swallowing a configured client error would hide that advisory is down.
  - `[low]` `[reject]` the action body is read without a limit — a size cap is a new guard on a body of one short JSON field.
  - `[false]` `[reject]` a signal can be dropped if it arrives while the pill is tapped — the click and the `EventSource` callback are separate tasks, and the click flushes the buffer it closed over.
  - `[medium]` `[patch]` Desfazer cleared the toast on failure — same catch as the screen handler above.
  - `[medium]` `[patch]` `load()` put a pill signal back on the board and dropped it from `seenIds` — pill ids stay off the board and inside `seenIds`, with a screen test that taps the pill once.
  - `[medium]` `[patch]` action controls were under 44px — same `var(--ar-touch)` change as the stylesheet row above.
  - `[medium]` `[patch]` `GET /v1/queue` never asserted `contacted_at` — `TestHTTP_ContactedAtOnQueue` requires the merged timestamp.
  - `[medium]` `[patch]` an expired snooze was never shown again — a past `SnoozedUntil` stays in the items.
  - `[medium]` `[patch]` successful PUT and DELETE never recorded the client call — contact, snooze, and undo expect 204 and a recorded call.
  - `[medium]` `[patch]` the Contatado screen test invented `contacted_at` — the BFF merge test is what fails if that copy is removed. The screen stub stays a stand-in for the browser.
  - `[medium]` `[patch]` `NewActionsHandler` had no httptest — same `actions_http_test.go` as the advisory HTTP row.
  - `[medium]` `[patch]` Adiar and Desfazer had no screen test — the screen test removes `s01` and restores it after DELETE.
  - `[false]` `[reject]` the stored `lastEventId` does not drive reconnect — same browser `EventSource` behavior as the first row. Story 6 already asserts the BFF `id:` frame.
  - `[low]` `[reject]` case advance has no test — the matrix does not include advance. `TestHTTP_CasesInColumns` covers the three seed columns.
  - `[low]` `[reject]` no single test mounts the whole seed board — the BFF case test, the screen open tests, and the browser pass cover the columns separately.
  - `[low]` `[reject]` phone and desktop tests inject `isDesktop` instead of a viewport — the browser checked 763px and 1100px. The CSS still uses `(min-width: 900px)`.
  - `[low]` `[reject]` the UI never asserts `Last-Event-ID` — the browser sends that header from the SSE `id:` field. Adding a custom reconnect would be a new mechanism.
  - `[medium]` `[defer]` snooze reload is not proven against PostgreSQL — the screen test now hides and restores the card. The SQL row is the deferred item below.
  - `[medium]` `[patch]` undo reload had no screen test — Desfazer in `QueueScreen.test.tsx` calls DELETE and the reloaded queue shows `s01`.
  - `[low]` `[reject]` advisory down is specified as POST — same matrix word as the PUT row. The tasks and the client use PUT.
  - `[false]` `[reject]` an unset `ADVISORY_HTTP_ADDR` has no test in this diff — `internal/proc/commands_test.go` already starts advisory with no env and requires `service=advisory` plus exit 0.
  - `[medium]` `[patch]` tokens were copied — same handoff import as the stylesheet root row.
  - `[medium]` `[patch]` touch targets were 36px — same `var(--ar-touch)` change.
  - `[medium]` `[defer]` action tests never open `signal_action` in PostgreSQL — recorded in `deferred`. `go test` must not dial 5435.
  - `[false]` `[reject]` case advance and Abrir caso sit outside the matrix — they match the design notes. They do not add a BFF database or gRPC.

## Design Notes

Advisory's HTTP is only for these action rows. It is not gRPC and it is not a second public API for the browser. The BFF is the only origin the React app calls.

Case cards on the board are the design seed served by `GET /v1/cases`. Advancing a case on the board updates that in-memory seed for the session. The cases service remains the SLA owner. This story does not write case state to PostgreSQL.

Snooze uses the injected clock in tests and `time.Now` in the process. One hour is 60 minutes.

## Verification

**Commands:**
- `gofmt -l .` -- expected: empty
- `go mod tidy && git diff --exit-code go.mod go.sum` -- expected: clean
- `go vet ./...` -- expected: exit 0
- `go build ./...` -- expected: exit 0
- `mkdir -p .gotmp && GOTMPDIR="$PWD/.gotmp" go test -race -shuffle=on ./...` -- expected: pass
- `pnpm --dir web install --frozen-lockfile` -- expected: exit 0
- `pnpm --dir web test` -- expected: exit 0
- `pnpm --dir web build` -- expected: exit 0

## Auto Run Result

**Summary:** The React queue reads only the BFF. Below 900px the board scrolls sideways and a card replaces it. At 900px and above the same board is four columns and the card opens in a dialog. Contatado and Adiar 1 h are stored by advisory and merged back into `GET /v1/queue`. The BFF does not keep those rows.

**Files:**
- `web/` — Vite React queue, pill, Contatado, Adiar 1 h, and Desfazer
- `internal/bff/http.go`, `internal/bff/actions.go`, `internal/bff/cases.go` — merge actions, proxy PUT and DELETE, seed cases
- `internal/advisory/actions.go`, `internal/advisory/actions_http.go`, `internal/advisory/actions_pgx.go` — contact, snooze, undo, optional SQL
- `migrations/advisory/001_book.sql` — `signal_action`
- `cmd/advisory/main.go` and `cmd/bff/main.go` — listen and call advisory only when the URLs are set
- `.env.example` — `ADVISORY_HTTP_ADDR` and `ADVISORY_HTTP_URL`

**Review:** Ten medium groups were patched: the handoff token import, 44px action controls, Desfazer keeping its toast on failure, advisory HTTP tests, Contact clearing an active snooze, pill ids surviving `load()`, the `contacted_at` merge, an expired snooze staying visible, the 204 action proxy, and the Adiar/Desfazer screen tests. PostgreSQL `signal_action` was deferred. Rejected findings were either untrue (the browser already sends `Last-Event-ID`, closing the AMQP connection closes the channels, a page reload keeps the in-process row) or low items whose fix would add a lock, a body cap, a timer, or a second public verb.

**Follow-up review:** true. Patched medium groups: 10. High patches: 0. Unverified risk: `go test` never executes the PostgreSQL `signal_action` statements in `internal/advisory/actions_pgx.go`.

**Verification:** `gofmt -l .` empty. `go mod tidy` left `go.mod` and `go.sum` clean. `go vet ./...` and `go build ./...` exited 0. `GOTMPDIR=$PWD/.gotmp go test -race -shuffle=on -count=1 ./...` passed. `pnpm --dir web install --frozen-lockfile`, `pnpm --dir web test` (6 tests), and `pnpm --dir web build` exited 0. The browser at 763px replaced the board on open, and at 1100px kept four columns plus a dialog. Contatado on s02 survived a reload against the advisory memory process.

**Residual risk:** A live PostgreSQL `signal_action` write is not dialed by `go test`. Restarting advisory without `ADVISORY_DATABASE_URL` drops the in-memory actions.
