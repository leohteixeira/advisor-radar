---
title: 'Accept purchases and raise the suitability-mismatch alert'
type: 'feature'
created: '2026-09-29'
status: 'done'
review_loop_iteration: 1
baseline_revision: '88a121f1a8dc4dedbc597df74a16eda3c491e281'
followup_review_recommended: true
context:
  - '{project-root}/AGENTS.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/architecture.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/ux.md'
  - '{project-root}/specs/http/bff.md'
  - '{project-root}/specs/adr/0010-individual-positions.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/stories/8-personalize-the-home-by-client-moment.md'
warnings: []
deferred:
  - |
    Purchase units are priced at SeedDay; story 13 replaces SeedDay with the stored sim_day (already in its spec).
  - |
    Command-key reuse with a different payload is not detected (pre-existing story 3 deferral).
  - |
    Timeline aplicacao rows carry no product detail for the client history; story 11 maps "Compra · {{product}}".
  - |
    advisory.account.event.recorded has no dead-letter queue; permanent errors are nacked without requeue and dropped (pre-existing topology, now also for unknown customer/profile and invalid purchases).
  - |
    Live alert cards (all kinds, perfil included) live in the BFF in-memory board; advisory ListQueue reads only the seeded queue_signals table. Persisting live alerts for the queue is a separate change.
---

<intent-contract>

## Intent

**Problem:** A client cannot buy a product. account-sim has no `aplicacao` command, the BFF has no purchase route, and advisory cannot flag a purchase above the investor profile. The CAP-4 backend and the team-queue `perfil` card therefore do not exist.

**Approach:** Add the `aplicacao` command end to end:

- account-sim's `Purchase` RPC moves cash into a position at the fixed catalog price and publishes `account.event.recorded` `kind: aplicacao` at `schema_version` 3 through the outbox, idempotently.
- The BFF adds `POST /v1/client-pov/customers/{id}/purchases`, with the phase-2 limits and the Bastidores flow.
- Advisory follows `aplicacao` in the book and raises an alert-only `perfil` card, "Compra acima do perfil de investidor", when the product's risk exceeds the profile's max risk.
- The team queue renders kind `perfil`.

## Boundaries & Constraints

**Always:** Load `golang-how-to` first and apply the Go skills in `/workspace/repos/advisor-radar/CLAUDE.md`.

**Event contract**

- `internal/event` accepts `schema_version` 3.
- A v3 `account.event.recorded` payload holds `kind`, `amount`, `before`, `after` (integer USD cents, as in v2), plus `product_id`, `asset_class`, and `risk` for `aplicacao`.
- Every consumer (advisory, timeline-indexer, BFF Bastidores/stream if it decodes payloads) accepts versions 1, 2, and 3 and scales v3 like v2 before a rule runs.
- The phase-2 commands keep publishing v2.
- Document v3 wherever the event schema is documented (`specs/` event docs and README event table if present).

**account-sim**

- `account/v1` gains `Purchase(customer_id, product_id, amount_cents, idempotency_key, command_id)` returning the same `CommandReply` (`event_id`, `replay`).
- It follows the existing command path: advisory-lock serialization, key reuse, outbox, and the same status mapping.
- Validation runs in this order:
  1. An unknown product is `InvalidArgument`.
  2. An amount ≤ 0, above `MaxAmountCents`, or below the product's `minimum_cents` is `InvalidArgument`.
  3. An amount above cash is `FailedPrecondition`, which the BFF maps to `422 insufficient`.
- One transaction does all of the following:
  - cash −= amount;
  - the position `(customer, product)` gets `units_cents` += `amount / factor(product, day)` and `applied_cents` += amount, created when absent;
  - one outbox row is written.

  In this story the factor is 1. Keep the division path so story 13 works, and round units half-up.
- Patrimony is unchanged: `before` equals `after`, which equals patrimony.
- `amount` is the purchase amount. `product_id`, `asset_class`, and `risk` come from the catalog row.
- Both stores support it: the memory store and pgx. `ResetPOV`/reseed restores the positions.

**BFF**

- `POST /v1/client-pov/customers/{id}/purchases` takes JSON `{product_id, amount_cents}` and requires `Idempotency-Key`.
- It shares the phase-2 per-customer limits: 10 per 60 s and 40 per 24 h, a replay is free, and budget follows the story 3 rules.
- It answers `202 {"event_id"}`. The error bodies and statuses are the ones the other POV commands use:
  - `422 {"error":"insufficient"}` when the amount is above cash;
  - `422 {"error":"invalid"}` for a bad body, product, or amount;
  - `400` for a bad id;
  - `502` when the upstream fails.
- Counters gain `actions.purchase` if the counters endpoint lists per-action counts.
- The Bastidores hub shows the `aplicacao` event like other account events.
- Document the route in `specs/http/bff.md` next to the other POV commands, with the phase-2 table format.

**advisory**

- The book follows `aplicacao`: AUM = `after`, so segment is unchanged since patrimony is unchanged.
- It also follows `reavaliacao` generically: it updates AUM from `after` and runs the segment rule. Only the drop rule on `reavaliacao` waits for story 13.
- New rule `perfil`: rule key `suitability`, kind `perfil`, text "Compra acima do perfil de investidor". It fires on `aplicacao` when `risk > MaxRisk(profile)`, using the story 8 table and the book's profile.
  - It is one small rule type, like the others.
  - The alert payload carries the product and amount fields that the existing alert/queue cards use, so the card body can say what was bought. Follow the existing card payload shape.
  - It opens no case: cases does not consume `alert.raised` for `perfil`. Verify this and keep it true.
- The team queue (`ListQueue`) returns kind `perfil` cards.

**web (team side only)**

- Add `perfil` to `AlertType`, with the label "Compra acima do perfil" (or the ux.md text) and an icon path in `queueVisual.ts` consistent with the other kinds.
- Add or update the tests that render a `perfil` card.
- Keep 100% coverage of `src/sdui/**` from story 6. Do not touch `src/sdui` in this story.

**General**

- Wrap errors with `%w`; propagate `context.Context`. Info logs carry no amounts.

**Never:**
- No Investir screen, purchase form, `product_rail`, or badge (story 10).
- No advance-day or drop on `reavaliacao` (story 13).
- No automatic case for `perfil`.
- No change to phase-2 command payloads (they stay v2).
- Unit tests do not dial 5435/5673/8400/8420/9201.
- Do not read or edit `.env` / `.env.*`. Never a `utils` package.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Thiago buys US$ 30,000 of acoesg | cash 6052000 | `202`; cash 3052000; acoesg units +3000000; event v3 `aplicacao` amount 3000000, before = after = 6800000, product_id acoesg, asset_class etfs, risk 3 | No error expected |
| Replay | same `Idempotency-Key` + same payload | `202`, same `event_id`, one event, no budget spent | — |
| Over cash | amount > cash | `422 insufficient`, no event, no position change | refusal counted |
| Below minimum / unknown product / ≤ 0 | — | `422 invalid` | — |
| Fernanda (conservador, max 2) buys cobalto (risk 5) | — | purchase succeeds; advisory raises `perfil` alert "Compra acima do perfil de investidor"; queue shows the card; no case | — |
| Within profile | Thiago (arrojado) buys cobalto | no `perfil` alert | — |
| Consumers | v1, v2, v3 events | advisory and timeline-indexer accept all three | unknown version → permanent |
| Rate limit | 11th command in 60 s | `429` as phase 2 | — |

</intent-contract>

## Code Map

- `internal/sim/{account,burst,value,memory,pgx,grpc}.go` -- commands (`build`), `AccountPayload` (`burst.go:22`, `Dollars(schemaVersion)`), positions, catalog, stores, gRPC.
- `proto/account/v1/account.proto`, `scripts/gen-proto.sh`.
- `internal/event/*.go` -- envelope validation, `SchemaVersionMVP`/`SchemaVersionCents`; add v3.
- `internal/bff/{pov,account,http}.go` -- POV command handlers, limits, counters, `grpcPOV`; `specs/http/bff.md` POV table.
- `internal/advisory/{rules,apply,pgx,grpc}.go` -- `EvaluateAccount` (`rules.go:45`), apply's `Dollars` scaling (`apply.go:82`), book AUM update, queue cards; the story 8 profile and max-risk table.
- `internal/timeline/*` -- indexing of account events by kind (`aplicacao` title/text like the others).
- `internal/cases` -- confirm no `alert.raised` → case path for `perfil`.
- `web/src/domain/{types,queueVisual}.ts` and their tests -- `perfil` card.

## Tasks & Acceptance

**Execution:**
- Event v3 support plus consumer scaling, with tests.
- account-sim `Purchase` in the domain, both stores, and gRPC, with tests for every matrix row, including a gated pgx test.
- The BFF route with limits, Bastidores, docs, and tests.
- The advisory `perfil` rule, book follow, and queue card, with rule table tests and apply tests.
- Timeline indexes `aplicacao`.
- The web queue card kind, with tests.

**Acceptance Criteria:**
- Given the local stack, when Fernanda buys Cobalto through `POST …/purchases`, then the purchase answers `202`, her cash drops, and a `perfil` card "Compra acima do perfil de investidor" appears in the team queue with no case opened.
- Given the change, when the Go suite, the gated pgx tests, and `pnpm --dir web test` run, then all pass.

## Verification

**Commands:**
- `sh scripts/gen-proto.sh && git status --porcelain gen/` -- expected: only account/v1 changed.
- `gofmt -l . && go vet ./... && go build ./...` -- expected: clean.
- `mkdir -p .gotmp && GOTMPDIR=$PWD/.gotmp go test -race -shuffle=on ./...` -- expected: pass.
- `ACCOUNT_SIM_TEST_DATABASE_URL='postgres://localdev:localdev@127.0.0.1:5435/account_sim?sslmode=disable' GOTMPDIR=$PWD/.gotmp go test -race -count=1 ./internal/sim/` -- expected: pass.
- `go mod tidy && git diff --exit-code go.mod go.sum` -- expected: clean.
- `pnpm --dir web test && pnpm --dir web build` -- expected: pass.

## Review Triage Log

| # | Source | Finding | Verdict | Route |
|---|--------|---------|---------|-------|
| 1 | Blind, Edge, VerifGap | Unknown customer/profile on aplicacao is requeued forever (classifyApplyError) | medium | patch |
| 2 | Edge | pgx UpdateBook does not wrap ErrUnknownCustomer for a missing row | medium | patch |
| 3 | Blind, Edge | v3 aplicacao with risk 0/out of range passes silently; v1/v2 aplicacao runs the rule | medium | patch |
| 4 | Blind | Live perfil cards exist only in the BFF board; ListQueue reads queue_signals | low | defer (existing pattern for all live kinds) |
| 5 | Blind, Edge | Catalog fetched on every queue/cases/SSE render, stalls when account-sim is down | medium | patch |
| 6 | Blind | Purchase units priced at SeedDay | low | defer (story 13) |
| 7 | Blind | Key reuse with a different payload undetected | low | defer (story 3) |
| 8 | Blind | Bad body answers 422 on purchase vs 400 elsewhere | false | reject (spec requires 422 invalid) |
| 9 | Blind | command_id barely logged | low | reject (existing log shape) |
| 10 | Blind, VerifGap | bff.md says the purchase panel works; web hides it until story 10 | low | patch |
| 11 | Blind | Timeline aplicacao rows lack product detail | low | defer (story 11) |
| 12 | Blind | asset_class dropped in the BFF | false | reject (card does not show it) |
| 13 | Blind | No guard test that perfil opens no case | low | patch |
| 14 | Blind | Web fixture gives Fernanda segment Advance | low | patch |
| 15 | Edge | perfilReason renders "em , risco 0." without product/risk | low | patch |
| 16 | Edge | sim.ErrProduct returned directly maps to 502 | low | patch |
| 17 | Edge | Home activity has no icon for aplicacao | low | patch |
| 18 | VerifGap | Alert payload raw keys never checked against the BFF board's keys | medium | patch |
| 19 | Intent | No end-to-end test of the purchase flow | low | reject (unit + gated pgx cover each hop; e2e out of MVP) |
| 20 | Intent | Limits shared across mixed commands not tested | low | reject (same limiter instance, covered by existing limiter tests) |

## Auto Run Result

**Summary:** Purchases work end to end:
- account-sim's `Purchase` RPC does validation, cash → position, and a v3 `aplicacao` event through the outbox, idempotently, in both stores.
- The BFF adds `POST /v1/client-pov/customers/{id}/purchases`, which shares the phase-2 limits and appears in Bastidores.
- Advisory follows `aplicacao`/`reavaliacao` in the book and raises the alert-only `perfil` card "Compra acima do perfil de investidor" when risk > max risk.
- The timeline indexes `aplicacao`.
- The web queue renders kind `perfil`.

**Files:**
- `proto/account/v1` + gen
- `internal/event`
- `internal/sim/{account,burst,value,memory,pgx,grpc}.go`
- `internal/bff/{account,pov,http,display,board,filter}.go`
- `internal/advisory/{profile,rules,apply,pgx}.go`
- `internal/screen/home.go`
- `internal/timeline/index.go`
- `cmd/advisory/main.go`, `cmd/cases/main.go`
- `web/src/domain/{types,queueVisual}.ts`
- tests
- `specs/http/bff.md`, README

**Review:** 20 findings.
- 11 patched (5 medium, 6 low); 4 of the medium patches cover 5 of the 6 medium findings, and one medium (live cards held only by the BFF board) was deferred.
- 5 deferred.
- 4 rejected.

**Followup review recommended:** yes. At least two medium findings were patched: consumer error classification, UpdateBook, purchase validation, the catalog cache, and the payload key contract.

**Verification:** all passed.
- gofmt, vet and build are clean.
- `go test -race -shuffle=on ./...` passes.
- The gated pgx tests for `internal/sim` and `internal/advisory` pass.
- `go mod tidy` is clean.
- gen-proto changes only `account/v1`.
- `pnpm --dir web test` passes 192/192 with 100% coverage of `src/sdui/**`, and `pnpm --dir web build` passes.

**Residual risks:**
- Permanent advisory errors are dropped without a DLQ.
- Live alert cards are held only by the BFF board.
- Purchase pricing is at SeedDay until story 13.
