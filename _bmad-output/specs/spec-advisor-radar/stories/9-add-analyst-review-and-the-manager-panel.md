---
title: 'Add analyst review and the manager panel'
type: 'feature'
created: '2026-09-27'
status: 'done'
review_loop_iteration: 0
baseline_revision: '0e6739f1da287f9f3fbeaa97c27b0cfa498e9807'
followup_review_recommended: false
context: []
warnings:
  - multiple-goals
deferred:
  - summary: >-
      The header has no light/dark theme toggle.
    evidence: |-
      ux.md asks for a theme toggle. AppShell had none before this story; this change only replaced the static Fila label with persona controls. This story's intent is the review queue and the manager panel.
    location: >-
      web/src/components/AppShell.tsx
    severity: low
---

<intent-contract>

## Intent

**Problem:** The analyst cannot correct a message whose intent probability is below 0.85, and the manager cannot see backlog, SLA risk, first contact, or the review rate.

**Approach:** A persona switch opens the review queue or the manager panel. Both read only the BFF. Correcting an intent updates the label and confirms the feedback. The panel shows the four measures for the demo day.

## Boundaries & Constraints

**Always:** Membership is intent probability below 0.85, including 0.82 and a heuristic row. The seven seed rows r01–r07 are the queue. Choosing an intent updates that row's label and confirms the feedback. The manager panel shows backlog by advisor, cases with SLA at risk, average minutes to first contact, and the review rate. Personas are Fila, Revisão de triagem, and Painel da assessoria. There is no login. The browser calls only the BFF. The BFF stores nothing in a database. UI copy is Portuguese. Touch targets are at least 44px. `go test` must not dial 5435, 5673, 8400, 8420, or 9201. Wrap errors with `%w`. Pass `context.Context` into I/O.

**Never:** Do not add `message.reclassify.requested`. Do not build `/demo`, a trace, or a k6 run. Do not add a database to the BFF. Do not read or write `.env`. Do not log customer text. Do not edit `internal/triage` keyword tables or `internal/outbox`. Do not remove the advisor queue.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Queue | `GET /v1/review` | r01–r07. r05 shows Reclamação at 0.82. A row whose top probability is 0.85 is absent | No error expected |
| Correct | `PUT /v1/review/r05` with intent Operacional | The row label is Operacional. A second GET still shows it. The UI confirms the feedback | No error expected |
| Bad intent | Intent `foo` | 400. The row is unchanged | Returned error |
| Backlog | `GET /v1/manager` | Ana Paula Ribeiro has open 17, risk 4, overdue 1 | No error expected |
| SLA risk | Same payload | k1042 is listed with 46 minutes left. k1044 is listed and overdue | No error expected |
| First contact | Same payload | Average first contact is 14 minutes | No error expected |
| Review rate | Same payload | The review share is 18. The fallback share is 6 | No error expected |
| Persona | Switch to Revisão de triagem, then back to Fila | The review queue replaces the board, then the advisor queue returns | No error expected |

</intent-contract>

## Code Map

- `docs/design/mock-data.js` lines 94–102 — `REVIEW` r01–r07. r05 is 0.82. r06 is the fallback row.
- `docs/design/mock-data.js` lines 139–158 — `MANAGER`: four advisors, `avgFirstContactMin` 14, `reviewPct` 18, `fallbackPct` 6, `atRisk` includes k1042 (46) and k1044 (-8).
- `web/src/components/AppShell.tsx` — the header says Fila and is not a switch yet.
- `web/src/App.tsx` — renders only `QueueScreen`.
- `internal/bff/http.go` — add `GET /v1/review`, `PUT /v1/review/{id}`, and `GET /v1/manager`. Do not store the rows in PostgreSQL.
- `ux.md` "Case, 360, review, manager" — distribution on the row, corrected intent, the four manager measures.

## Tasks & Acceptance

**Execution:**
- `internal/bff/review.go` — seed r01–r07, drop any row at or above 0.85, correct an intent in memory, reject an unknown intent
- `internal/bff/manager.go` — the demo-day snapshot: backlog, at-risk cases, first contact 14, review 18, fallback 6
- `internal/bff/http_test.go` — the matrix rows for the queue, the correction, the bad intent, and the four manager measures
- `web/src/components/AppShell.tsx` — three persona controls
- `web/src/screens/ReviewScreen.tsx` — the seven rows, the distribution, and a correction that confirms the feedback
- `web/src/screens/ManagerScreen.tsx` — backlog, SLA risk, 14 minutes, and 18 percent
- `web/src/screens/ReviewScreen.test.tsx` — r05 becomes Operacional and the confirmation is shown; switching back to Fila shows the queue

**Acceptance Criteria:**
- Given the review queue, when the analyst sets r05 to Operacional, then the label updates and the feedback is confirmed.
- Given the manager panel, when it loads, then Ana Paula Ribeiro shows 17 open, k1042 shows 46 minutes, first contact is 14, and the review rate is 18.
- Given Fila, when the persona returns from Revisão de triagem, then the advisor queue is shown again.

## Spec Change Log

## Review Triage Log

### 2026-09-27 — Review pass
- verdicts: 28 findings — high 0, medium 2, low 11, false 15, maybe-false 0
- findings:
  - `[medium]` `[patch]` A failed correction replaced the review queue with the load-error screen — `onCorrect` now keeps the cards and shows "Não foi possível corrigir. Tente de novo."
  - `[low]` `[defer]` The header has no light/dark theme toggle — pre-existing; AppShell never had one, and this story only replaced the static Fila label.
  - `[false]` `[reject]` New review and manager CSS ignores `prefers-reduced-motion` — those rules add no animation or transition.
  - `[low]` `[patch]` `INTENT_LABELS` was exported and unused — deleted. `correctReview` still accepts `Intent | string`, matching `Signal.intent`; unknown labels already return 400.
  - `[false]` `[reject]` The nil check in `listReview` is dead — `Items` always returns a slice from `make`, so the response is unchanged.
  - `[false]` `[reject]` `Correct` lacks direct unit tests — success, a bad intent, and an unknown id are covered by the HTTP tests.
  - `[low]` `[patch]` The second GET after a correction did not assert `corrected` — the test now requires it to stay true.
  - `[low]` `[patch]` A failed correction had no UI regression test — the test now keeps the card and expects the failure toast.
  - `[low]` `[patch]` The App test never opened Painel da assessoria — it now opens that persona, sees Ana Paula Ribeiro, and returns to Fila.
  - `[false]` `[reject]` The fallback note is always "modelo externo saudável" — the demo snapshot is the healthy day, and the intent asks for the 6% share, not a degraded variant.
  - `[false]` `[reject]` A worse first-contact day would render a double minus — the only snapshot is 19 then 14, so that input never reaches the panel.
  - `[low]` `[patch]` Confirmar's accessible name was only "Confirmar" — it is now "Confirmar {intent}". `aria-current="page"` stays on the active persona control.
  - `[false]` `[reject]` The first-contact delta has no worse-than-yesterday branch — same fixed snapshot as above.
  - `[low]` `[reject]` Two clicks can send two corrections before the buttons disable — the second PUT writes the same label, and a ref guard would be a new guard.
  - `[false]` `[reject]` Backlog bar segments can exceed 100% — in the seed, risk plus overdue never exceeds open.
  - `[false]` `[reject]` A JSON encode error leaves a success status — that happens when the client is already gone, and the handler stops, as the other endpoints do.
  - `[medium]` `[patch]` A rejected correction replaced the loaded queue — same fix as the first row.
  - `[low]` `[patch]` The manager persona path was not exercised through `App` — the App test now clicks Painel da assessoria.
  - `[low]` `[patch]` `PUT /v1/review/r99` was untested — it expects 404 and leaves r05 unchanged.
  - `[low]` `[patch]` Confirmar was never clicked — the test clicks it on r05 and expects the Confirmado feedback.
  - `[low]` `[patch]` The manager UI test did not assert the fallback share — it now expects 6%.
  - `[false]` `[reject]` The UI tests do not load all seven review rows — the queue matrix row is `GET /v1/review`, covered by the HTTP test.
  - `[false]` `[reject]` Correction is split between HTTP and a mocked UI — both surfaces are tested, and the browser confirmed the live PUT.
  - `[false]` `[reject]` A bad intent is not tested in the UI — the matrix row is the HTTP 400.
  - `[false]` `[reject]` The manager UI test uses a mock — the HTTP test checks the seed, and the browser loaded the live panel.
  - `[false]` `[reject]` The main UI correction test does not require the heuristic badge — the HTTP test asserts r06 is the fallback row.
  - `[false]` `[reject]` Portuguese copy and 44px targets are not asserted by tests — the copy is Portuguese and the new buttons use `--ar-touch`.
  - `[false]` `[reject]` In-memory review methods do not take `context.Context` — they do no I/O; the handlers check the request context.

## Design Notes

The correction lives in the BFF process memory, the same way the seed cases do. It is not a triage write and not `message.reclassify.requested`. A reload shows the corrected label while that process is up. The manager numbers are the `MANAGER` seed for the demo day, not a live aggregate across Postgres.

Intent labels are the eight Portuguese names in `ux.md`. Anything else is 400.

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

The analyst can open Revisão de triagem, correct a low-confidence intent, and see the confirmation. The manager can open Painel da assessoria and read backlog, SLA risk, first contact, and the review rate. Both screens read only the BFF. The rows live in process memory.

### Files changed

- `internal/bff/review.go` — seed r01–r07, drop a row at 0.85, correct an intent in memory
- `internal/bff/manager.go` — demo-day backlog, SLA risk, 14 minutes, review 18, fallback 6
- `internal/bff/http.go` — `GET /v1/review`, `PUT /v1/review/{id}`, `GET /v1/manager`
- `internal/bff/review_test.go` — a live row at exactly 0.85 is absent
- `internal/bff/http_test.go` — queue, correction, bad intent, unknown id, manager snapshot
- `web/src/components/AppShell.tsx` — Fila, Revisão de triagem, Painel da assessoria
- `web/src/App.tsx` — switches the three screens
- `web/src/screens/ReviewScreen.tsx` — distribution, correction, confirmation, failure toast
- `web/src/screens/ManagerScreen.tsx` — the four measures plus the fallback share
- `web/src/screens/ReviewScreen.test.tsx` — correction, failure, persona switch, manager numbers
- `web/src/api/bff.ts`, `web/src/domain/types.ts`, `web/src/styles/app.css` — client, types, and layout

### Review

Patches: one medium group (a failed correction no longer clears the queue) and eight low test or naming fixes. Deferred: the theme toggle, which predates this story. Rejected findings are listed in the triage log; they describe unreachable seed states, dead branches, or checks that already exist on the HTTP surface.

Follow-up review: false. One medium group was patched. No unverified risk remains after the HTTP tests and the browser pass.

Patched counts: high 0, medium 1 group (2 findings), low 8 findings.

### Verification

- `gofmt -l .` empty
- `go mod tidy` left `go.mod` and `go.sum` clean
- `go vet ./...` and `go build ./...` exit 0
- `go test -race -shuffle=on ./...` passed
- `pnpm --dir web test` — 17 passed
- `pnpm --dir web build` passed
- Browser: Revisão de triagem listed the seven rows; setting the Reclamação card to Operacional showed "Corrigido para Operacional" and `GET /v1/review` kept r05 as Operacional with `corrected` true. Painel da assessoria showed Ana Paula Ribeiro, 17, 14, 18%, 6%, k1042, and k1044 vencido há 8 min. Fila restored Novos sinais.

### Residual risk

A process restart drops corrections. That is the same in-memory rule as the seed cases. The theme toggle remains deferred.
