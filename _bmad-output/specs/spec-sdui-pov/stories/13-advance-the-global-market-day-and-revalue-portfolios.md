---
title: 'Advance the global market day and revalue portfolios'
type: 'feature'
created: '2026-09-29'
status: 'done'
review_loop_iteration: 0
baseline_revision: '4eefc3b'
followup_review_recommended: true
context:
  - '{project-root}/AGENTS.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/architecture.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/ux.md'
  - '{project-root}/specs/http/bff.md'
  - '{project-root}/specs/adr/0010-individual-positions.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/stories/4-store-per-product-positions-and-the-product-catalog.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/stories/8-personalize-the-home-by-client-moment.md'
warnings: []
deferred:
  - summary: >-
      Every advance indexes a reavaliacao row per account, flat ones included, and the BFF timeline search has no limit.
    evidence: |-
      Flat rows are filtered only in visibleActivity; home and Carteira read a history that grows with each advance. A search limit or index-time filter would settle it.
    location: >-
      internal/timeline/index.go, internal/bff/timeline.go
    severity: low
  - summary: >-
      Matrix rows are verified per service with stubbed neighbours; no test drives an account-sim outbox row through advisory and the indexer.
    evidence: |-
      Cross-service agreement rests on shared fixture numbers; the live run in the implementation checked the chain once.
    severity: medium
---

<intent-contract>

## Intent

**Problem:** Market values never move. There is no simulated day, no revaluation event, no drop alert from a revaluation, and neither the day-change pill nor the `portfolio_drop` moment exists. CAP-6 and the portfolio-drop part of CAP-2 are therefore missing.

**Approach:** Add advance-day end to end:
- account-sim keeps one global `sim_day` and prices positions through the pure `Value(product, units, day)` with the scripted shock: Cobalto −53.5% from day 3 on.
- An idempotent `AdvanceDay` command writes, in one transaction, the new day and one `reavaliacao` outbox row per account.
- Advisory follows `reavaliacao`, raises its drop alert on a negative one above 15%, and reports the `portfolio_drop` fact.
- The BFF adds the advance route, the `with_day_change` variants, and `portfolio_drop`.
- Web adds the simulation-strip control.

## Boundaries & Constraints

**Always:** Load `golang-how-to` first and apply the Go skills in `/workspace/repos/advisor-radar/CLAUDE.md`.

**account-sim**

- **Pricing.**
  - `priceFactor("cobalto", day)` is 1 for day < 3 and 465/1000 for day ≥ 3. Every other product is 1.
  - `SeedDay` is replaced by the stored day.
  - Keep the half-away-from-zero rounding and test it with the real factor (story 4 deferral). Cobalto 7200000 units gives 3348000 on day 3; Thiago's 204000 gives 94860.
- **Day storage.**
  - Migration `006_pov_sim_day.sql`: a singleton `pov_sim(id BOOLEAN PRIMARY KEY DEFAULT true CHECK (id), sim_day INT NOT NULL)`, seeded to 0.
  - Reseed resets it to 0.
  - Values are computed at read time from units and the day, so positions need no rewrite. State this in ADR 0010 as a note.
- **`account/v1` changes.**
  - `AdvanceDay(idempotency_key, command_id)` → `{sim_day, event_ids, replay}`.
  - `Account` gains `sim_day` and `day_change_cents` (patrimony at `sim_day` minus patrimony at `sim_day − 1`, 0 on day 0).
  - Regenerate the proto.
- **AdvanceDay transaction.** It takes a global advisory lock (a namespace distinct from the per-customer one) and orders it before the per-customer locks, so there is no deadlock with deposits. Inside it:
  - `sim_day += 1`;
  - for every POV account, one outbox row `account.event.recorded` at `schema_version` 3, with payload:
    - `kind: reavaliacao`;
    - `amount` = after − before, signed, in cents;
    - `before` and `after` = patrimony at the old and new day;
    - `sim_day`;
    - `product_id` = the position with the largest absolute change, empty when every change is 0;
    - `product_change_bp`: that product's day change in basis points, e.g. −5350;
  - the event id is a name-based UUID of `reavaliacao:{customer_id}:{sim_day}`.
- **Replay.** A replayed idempotency key advances nothing, publishes nothing, and returns the original reply. Key reuse follows the existing command-key rules.
- **Existing commands.** Deposits, withdrawals, and purchases value positions at the stored day. Their v2/v3 `before`/`after` are patrimony at that day.

**BFF**

- `POST /v1/client-pov/simulation/advance-day` requires `Idempotency-Key` and has its own global limit of 20 per 10 minutes, separate from the per-customer limits.
  - `202 {sim_day, event_id}`. Use the first event id, or document the list if more are needed.
  - `429` with the phase-2 body shape when over the limit.
  - `502` when upstream fails.
- `GET /v1/client-pov/simulation` → `{sim_day}` for the strip. Document both routes in `specs/http/bff.md`.
- **Screen variants.**
  - Home `wealth_summary` `with_day_change` matches when `day_change_cents ≠ 0`. It adds `day_change` "{{signed money}} ({{signed pct}}) no dia {{day}}" and `day_change_tone` `neg` or `pos`. The pct is `day_change / patrimony_before` with one decimal and U+2212 for negatives.
  - Carteira `portfolio_summary` `with_day_change` has the same rule. "Dia simulado" shows the real day. The Carteira subtitle uses the day.
  - Home moment `portfolio_drop` has priority 1 and matches the advisory fact.
    - Copy follows ux.md: "{{first}}, sua carteira caiu {{drop_pct}} hoje" and "{{product}} recuou {{product_pct}} no dia simulado {{day}}. A {{advisor}} já foi avisada e vai falar com você."
    - Action `navigate carteira` ("Ver carteira"), tone `neg`.
- **Timeline history.** A `reavaliacao` row with amount ≠ 0 reads "Reavaliação diária" with meta "dia simulado {{day}} · {{product}} {{pct}}". A zero-change row is not shown on the client side.

**Advisory**

- The book follows `reavaliacao`: AUM = `after`, and the segment rule runs.
- The drop rule fires on `reavaliacao` with `amount < 0` and `|amount| / before > 0.15`. It raises the existing `queda` alert kind with its rule text. The phase-1 `asset_drop` path is unchanged.
- The latest `reavaliacao` per customer is stored (`sim_day`, `amount`, `before`, `product_id`, `product_change_bp`).
- `GetMomentFacts.portfolio_drop` is true when that latest revaluation is for the current day, as reported by the stored latest day, and its loss is above 15% of `before`. Add `drop_bp`, `drop_product_id`, `drop_product_bp`, and `drop_day` fields.
- The BFF formats them and resolves the product name from the catalog.

**Web**

- The simulation strip shows "Dia simulado {{day}}" from `GET …/simulation`, plus a button "Avançar um dia" ("+1 dia" on phone).
- The button posts with a fresh key, disables while pending, updates the day, and re-fetches the current screen.
- On `429` or failure it shows the existing strip-level error text pattern.
- Register nothing new. Existing components render the new props. Add tests for the `day_change` pill props and the tone mapping, and keep 100% coverage of `src/sdui/**`.

**Seed docs**

- Document in the README demo notes that the shock hits every Cobalto holder, and that non-demo accounts may raise drop alerts.

**General**

- Wrap errors with `%w`. Propagate `context.Context`. Info logs carry no values.

**Never:**
- No randomness or wall clock in pricing.
- No position rewrite on advance.
- No Raio-X (story 14), `X-SDUI-Schema` or beta `v2` (story 16), or OTel (story 17).
- Unit tests do not dial 5435/5673/8400/8420/9201.
- Do not read or edit `.env` / `.env.*`.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Day 0 → 1 | reseeded | `sim_day` 1; one `reavaliacao` per POV account with amount 0; no alert; homes unchanged, no pill | — |
| Day 2 → 3 | shock | Mariana amount −3852000, before 24830000 (−15.5%) → `queda` alert; Thiago −109140 (−1.6%) → no alert; Fernanda 0 | — |
| Mariana home day 3 | drop fact | `portfolio_drop` "Mariana, sua carteira caiu 15,5% hoje", "Cobalto Semicondutores recuou 53,5% no dia simulado 3…", wealth pill "−US$ 38.520,00 (−15,5%) no dia 3" | — |
| Day 3 → 4 | flat | Mariana back to `portfolio_review` (or `case_open` if a case is open); no pill | — |
| Replay | same key | no advance, no events, same reply | — |
| Rate limit | 21st advance in 10 min | `429` | — |
| Concurrent | advance + deposit | serialized; deposit before/after reflect the day it ran on | — |
| Reseed | after advances | day 0, values back to seed | — |

</intent-contract>

## Code Map

- `internal/sim/{value,account,memory,pgx,grpc}.go`, `migrations/account_sim/`, `seeds/account_sim/`, `proto/account/v1/account.proto` -- pricing, day, AdvanceDay, account fields.
- `internal/bff/{pov,http,account}.go` -- routes, global limiter, adapter; `specs/http/bff.md`.
- `internal/screen/*`, `catalog.json` -- `with_day_change`, `portfolio_drop`, history row mapping.
- `internal/advisory/{rules,apply,pgx,grpc}.go`, `migrations/advisory/` -- drop rule on `reavaliacao`, latest revaluation, moment fact.
- `internal/timeline/*` -- indexing of `reavaliacao`.
- `web/src/screens/ClientAppScreen.tsx` (simulation strip), `web/src/sdui/*`.
- `docs/design/sdui-full-pov/project/{Home-Mariana-Queda,OrlaApp}.dc.html`.

## Tasks & Acceptance

**Execution:**
- account-sim: pricing, day, and AdvanceDay, with memory and pgx tests for every matrix row.
- advisory: rule, storage, and fact, with tests.
- BFF: routes, variants, and history mapping, with tests and docs.
- web: the strip control, with tests.
- README note.

**Acceptance Criteria:**
- Given the reseeded local stack, when the operator advances three days, then Mariana's home shows `portfolio_drop` with the pill, the team queue shows her `queda` alert, and Thiago gets no alert.
- Given the change, when the Go suite, the gated pgx tests, `pnpm --dir web test`, and `pnpm --dir web build` run, then all pass.

## Verification

**Commands:**
- `sh scripts/gen-proto.sh && git status --porcelain gen/` -- expected: account (and advisory) only.
- `gofmt -l . && go vet ./... && go build ./...` -- expected: clean.
- `mkdir -p .gotmp && GOTMPDIR=$PWD/.gotmp go test -race -shuffle=on ./...` -- expected: pass.
- `ACCOUNT_SIM_TEST_DATABASE_URL='postgres://localdev:localdev@127.0.0.1:5435/account_sim?sslmode=disable' GOTMPDIR=$PWD/.gotmp go test -race -count=1 ./internal/sim/` -- expected: pass.
- `go mod tidy && git diff --exit-code go.mod go.sum` -- expected: clean.
- `pnpm --dir web test && pnpm --dir web build` -- expected: pass; coverage thresholds met.

## Spec Change Log

- 2026-09-29 — Orchestrator decisions accepted at merge (no intent change):
  - `pov_sim` carries an `epoch` UUID, and reavaliacao ids are name-based in that epoch namespace, so replaying days after a reseed publishes fresh ids.
  - `pov_advance` stores the advance replies.
  - `GetSimulation` RPC added for the strip.
  - Timeline fields 12 and 13 added.
  - `portfolio_drop` is skipped when the catalog cannot name the product or advisory has no advisor.
  - A 429 on advance-day gets its own copy ("Espere até 10 minutos"), since the window is 10 minutes.

## Review Triage Log

### 2026-09-29 — Review pass
- verdicts: 27 findings — high 0, medium 6, low 19, false 1, maybe-false 1
- findings:
  - `[medium]` `patch` A reseed keeps advance keys answerable with the old epoch — pov_advance and Memory.advances cleared on reset and seed.
  - `[medium]` `patch` An older reavaliacao overwrites a newer one, and reseed rows are indistinguishable — epoch carried in the payload; guarded upsert.
  - `[low]` `patch` The 15% threshold is implemented twice with different arithmetic — one integer-cents predicate.
  - `[low]` `reject` "Queda acima de 15% em 5 dias úteis" text on a daily check — the spec says to reuse the existing queda rule text.
  - `[low]` `reject` The day-change pill values same-day purchases at the previous day's price — matches the spec's definition (holdings valued at sim_day and sim_day−1).
  - `[medium]` `patch` Retrying a failed advance sends a fresh key and can advance twice — key kept until a definite answer.
  - `[low]` `patch` The strip day goes stale when another viewer advances — re-read on reload.
  - `[low]` `patch` postAdvanceDay does not validate the reply — validated.
  - `[low]` `patch` The replay/retry budget branches are untested; fakeSim.calls is unasserted — tests added.
  - `[low]` `reject` advanceLimiter duplicates povLimiter; `known` grows for the process lifetime — same pattern as the existing POV limiter; demo-scale process.
  - `[low]` `reject` command_id not logged on success — existing command logging pattern.
  - `[low]` `patch` Drop copy says "recuou" when the top mover rose — the moment requires a falling product.
  - `[low]` `patch` Out-of-range revaluation fields retry instead of failing permanently — validated as ErrInvalidRevaluation.
  - `[low]` `patch` A missing pov_sim row silently prices at day 0 in GetAccount — returns an error.
  - `[low]` `defer` Flat revaluations are indexed, and the timeline search is unbounded — recorded.
  - `[medium]` `patch` (dup of 2) Stale day overwrite — same fix.
  - `[medium]` `patch` (dup of 1) Keys survive reseed — same fix.
  - `[low]` `reject` (dup of 10) Unbounded `known` map — same reason.
  - `[medium]` `patch` The single reload right after the 202 comes before advisory consumes the revaluation, so the drop moment is missing — delayed second re-fetch.
  - `[low]` `patch` (dup of 8) The 202 body is unvalidated — same fix.
  - `[low]` `patch` (dup of 7) The strip day is stale — same fix.
  - `[low]` `patch` (dup of 14) COALESCE on the missing row — same fix.
  - `[low]` `patch` (dup of 12) The product rose — same fix.
  - `[maybe-false]` `patch` (dup of 19) Claim: the demo shows the drop card after three advances — same fix.
  - `[low]` `patch` (dup of 9) Verification gap: the replay budget branches — same fix.
  - `[false]` `reject` Intent R6: new 429 copy is undocumented — recorded in this change log.
  - `[medium]` `defer` Intent R8: the matrix is verified per service, not end to end — recorded; the live run covered the chain.

## Auto Run Result

**Summary:**
- **account-sim.**
  - It keeps one global `sim_day` in the `pov_sim` singleton, with an epoch; migration 006 and seed 005 set it up.
  - Positions are priced through the pure `Value(product, units, day)`. Cobalto falls to 465/1000 from day 3, rounded half away from zero.
  - `AdvanceDay` is idempotent. It takes a global lock ordered before the per-customer locks, and it writes the new day plus one v3 `reavaliacao` per POV account, with a name-based id in the epoch namespace.
  - `GetSimulation` is new, and `Account` carries `sim_day` and `day_change_cents`.
- **advisory.** The book follows `reavaliacao`. The `queda` rule fires on a loss above 15% (exact integer cents). The latest revaluation is stored per customer, guarded by epoch and day (migration 004). `GetMomentFacts` carries the drop fields.
- **BFF.**
  - `POST /v1/client-pov/simulation/advance-day` has a global limit of 20 per 10 minutes, and `GET /v1/client-pov/simulation` reports the day.
  - It serves `with_day_change` on the home `wealth_summary` and the Carteira `portfolio_summary`, the `portfolio_drop` moment (priority 1, falling product only), and the "Reavaliação diária" history row.
- **Web.** The simulation strip shows "Dia simulado N" and an "Avançar um dia" / "+1 dia" button.
  - The idempotency key is kept until a definite answer.
  - The day is re-read on reload, and the screen is re-read again after a delay so the drop moment appears.
  - The day-change pill renders in `PortfolioSummary`.
- **Docs.** README demo note, `bff.md`, and ADR 0010 note.

**Files:**
- `internal/sim/{value,advance,account,memory,pgx,grpc,burst}.go`
- `migrations/account_sim/006_pov_sim_day.sql`, `seeds/account_sim/005_pov_sim_day.sql`
- `migrations/advisory/004_revaluation.sql`
- `internal/advisory/{rules,apply,moments,pgx,grpc}.go`
- `proto/{account,advisory,timeline}/v1` + gen
- `internal/timeline`
- `internal/bff/{simulation,account,http}.go`
- `internal/screen/{moment,home,carteira,activity,catalog}.go`, `catalog.json`
- `web/src/screens/ClientAppScreen.tsx`, `web/src/api/pov.ts`, `web/src/sdui/components/PortfolioSummary.tsx`
- tests
- `specs/http/bff.md`, ADR 0010, README

**Review:** 27 findings. 19 patched (4 distinct medium entries; the 7 medium verdicts include duplicates). 2 deferred: unbounded timeline growth, and the cross-service matrix. 6 rejected, with the reasons in the triage log.

**Followup review recommended:** yes. Several medium entries were patched: epoch-guarded revaluations, advance keys cleared on reseed, advance-key reuse on retry, and the delayed drop re-fetch.

**Verification:**
- gofmt, vet and build are clean.
- `go test -race -shuffle=on ./...` passes.
- The gated pgx tests for `internal/sim` and `internal/advisory` pass.
- `go mod tidy` is clean.
- gen-proto changes only `account`, `advisory` and `timeline`.
- web 341/341 passes with 100% coverage of `src/sdui/**`, and `pnpm --dir web build` passes.

**Live and visual check:** real services with throwaway databases, RabbitMQ and headless Chromium, before the review patches. Screenshots are in the session scratchpad under `s13/`.
- **Days 1–2:** flat for everyone.
- **Day 3:**
  - Mariana gets "Mariana, sua carteira caiu 15,5% hoje", the pill "−US$ 38.520,00 (−15,5%) no dia 3", Carteira day 3, a history row, and a live `queda` alert (−38.520).
  - Thiago gets only his pill (−1,6%) and no alert.
- **Day 4:** Mariana is back to `portfolio_review`.
- **Replay:** returns the same reply.
- **Rate limit:** the 21st advance gets `429`, and the strip shows the alert.

**Residual risks:**
- The matrix is verified per service, with shared fixture numbers.
- The global advance budget is per BFF process.
- The timeline grows by one row per account per day.
