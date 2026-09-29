---
title: 'Build the Carteira screen with movement history'
type: 'feature'
created: '2026-09-29'
status: 'done'
review_loop_iteration: 0
baseline_revision: '552b755'
followup_review_recommended: true
context:
  - '{project-root}/AGENTS.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/ux.md'
  - '{project-root}/specs/http/bff.md'
  - '{project-root}/specs/adr/0010-individual-positions.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/stories/10-build-the-investir-screen-and-the-coded-purchase-form.md'
warnings: []
deferred:
  - summary: >-
      No HTTP test asserts the raw product_id and amount_cents keys on GET /v1/customers/{id}/timeline.
    evidence: |-
      Existing tests decode into bff.TimelineEntry and index no aplicacao row; no consumer reads the fields from that route yet.
    location: >-
      internal/bff/http_test.go
    severity: low
  - summary: >-
      Web Carteira fixtures are hand-built copies of the Go envelopes.
    evidence: |-
      Carried from stories 6 and 10; a golden export from Go would settle it.
    location: >-
      web/src/test/sduiFixtures.ts
    severity: medium
---

<intent-contract>

## Intent

**Problem:** The Carteira tab is a placeholder. The client cannot see total patrimony, allocation, per-product positions, or movement history, even though account-sim holds positions (story 4) and the timeline holds movements.

**Approach:** The BFF serves `GET …/screens/carteira` (`v1`), 100% SDUI, with these sections:
- `summary` (`portfolio_summary` `default`);
- `allocation` (`allocation_breakdown`);
- one `position_list` per class that has positions;
- `history` (`activity_list` `history`).

Every value and percentage is formatted in Go. Home `activity` and Carteira `history` share one timeline mapping. Web registers the three new types and renders the Carteira route, faithful to `Carteira-Mariana` and `Carteira-Desktop`.

## Boundaries & Constraints

**Always:**

- Load `golang-how-to` first and apply the Go skills in `/workspace/repos/advisor-radar/CLAUDE.md`.

- **BFF `summary`**
  - Section `summary`, `portfolio_summary` `default`.
  - Props: `total_label` "Patrimônio total", `total` (patrimony), and `stats`:
    - "Valor aplicado": the sum of `applied_cents` over the positions.
    - "Rentabilidade": signed money and a signed percentage of (sum of position values − applied) / applied, e.g. "+US$ 19.400,00 (+11,5%)", with `tone` `pos`, `neg`, or `neutral` at zero.
    - "Caixa".
    - "Dia simulado": the current simulated day from the account source. It is 0 until story 13 adds the global day. Keep it as a Snapshot field that story 13 fills.
  - `with_day_change` is not served in this story.

- **BFF `allocation`**
  - `allocation_breakdown` `default`, `title` "Alocação".
  - `rows` covers Ações, ETFs, Renda fixa, and Caixa (all four, including zero). Each row has `class`, `label`, `value`, `share` (largest remainder, sums to 100), and `bar_width`.

- **BFF position lists**
  - Sections in the order `positions_stocks`, `positions_etf`, `positions_fixed_income`, each only when the class has a position.
  - Variants `stocks` / `etf` / `fixed_income`. `title` is "Ações" / "ETFs" / "Renda fixa", with `subtotal` and `applied_label` "Aplicado".
  - `items` are `[{product_id, name, applied, value, return, return_tone}]`, ordered by value descending then id. `return` is the signed percentage with one decimal (e.g. "+20,0%"). Names come from the catalog source (`ListProducts`).

- **BFF `history`**
  - `activity_list` `history`, `title` "Movimentações".
  - Up to 20 client-facing timeline rows, newest first. Use the same mapping as home `activity`: extract one shared function, so the home keeps its limit of 5.
  - With no rows, `items: []` and `empty_text` "Nenhuma movimentação ainda.".
  - Rows of kind `aplicacao` (story 9) read "Compra · {{product}}" with meta "{{ago}}" and value "−{{amount}}".
  - The `reavaliacao` row text arrives in story 13. Keep the mapping extensible.

- **BFF heading and failure policy**
  - Heading: "Carteira" / "Valores de mercado no dia simulado {{day}}".
  - Account failure omits `summary`, `allocation`, and positions.
  - Catalog failure omits the position lists only (names unknown). Do not fall back to product ids.
  - Timeline failure omits `history`.
  - `carteira` now answers `200`.
  - Update `specs/http/bff.md`, including the `activity_list` `history` props if they changed.
  - One test per variant.
  - Seed-client screen tests: Mariana has 5 positions in 3 classes; Thiago has stocks and ETF only, so no `positions_fixed_income`; Fernanda has all 3 classes.

- **Web**
  - Register `portfolio_summary`, `allocation_breakdown`, and `position_list`. `activity_list` already exists; add its `history` rendering (title and empty text) if it is not already covered.
  - The `/client-pov/{id}/carteira` route renders the envelope.
  - The eye toggle masks money props in the new components.
  - Layout follows `Carteira-Mariana` (phone) and `Carteira-Desktop`, using the spans from `OrlaApp.dc.html` `walletSections`.
  - Coverage of `src/sdui/**` stays at 100%. `ClientPov.test.tsx` keeps passing; update its tab note expectation if Carteira no longer shows it.
  - Visual check at 390 px and on desktop against the artboards, recorded in the result.

**Never:**
- No day change (`with_day_change`) or revaluation (story 13).
- No Perfil screen, `X-SDUI-Schema`, or Raio-X.
- Web computes no sum, percentage, or return.
- Unit tests do not dial 5435/5673/8400/8420/9201.
- Do not read or edit `.env` / `.env.*`.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Mariana | day 0 | total "US$ 248.300,00"; applied "US$ 168.900,00"; Rentabilidade "+US$ 19.400,00 (+11,5%)"; positions in 3 classes; allocation shares sum 100 | — |
| Thiago | no fixed income | no `positions_fixed_income` section; allocation Renda fixa row 0% | — |
| After Thiago's "Tudo" purchase | acoesg grows | ETF subtotal includes the purchase; history shows "Compra · Maré Ações Globais ETF" | — |
| No history | timeline empty | `history` with `items: []` and "Nenhuma movimentação ainda." | — |
| Timeline down | Search fails | `history` omitted, rest `200` | — |
| Catalog down | ListProducts fails | position lists omitted; summary and allocation stay | — |
| Account down | GetAccount fails (not NotFound) | only `history`; others in `omitted` | — |

</intent-contract>

## Code Map

- `internal/screen/{snapshot,catalog,engine,home,format}.go`, `internal/screen/catalog.json` -- sources (account positions, catalog from story 10, timeline), formatter, variants; extract the shared activity mapping from `home.go`.
- `internal/bff/screen.go` -- slug set.
- `web/src/sdui/{registry,types}` and `web/src/sdui/components/*` -- register the new types.
- `web/src/screens/ClientAppScreen.tsx` -- tab routes.
- `docs/design/sdui-full-pov/project/{OrlaApp,Carteira-Mariana,Carteira-Desktop}.dc.html` -- `walletSections` at `OrlaApp.dc.html:834`.

## Tasks & Acceptance

**Execution:**
- BFF: Carteira sections, the shared activity mapping, the failure policy, docs, and tests.
- Web: three components plus the history rendering, the route, and tests at 100% coverage.

**Acceptance Criteria:**
- Given the local stack, when Mariana opens Carteira, then she sees summary, allocation, three position lists, and history with backend-formatted values matching `Carteira-Mariana` at day 0.
- Given the change, when the Go suite, `pnpm --dir web test`, and `pnpm --dir web build` run, then all pass with 100% `src/sdui/**` coverage.

## Verification

**Commands:**
- `gofmt -l . && go vet ./... && go build ./...` -- expected: clean.
- `mkdir -p .gotmp && GOTMPDIR=$PWD/.gotmp go test -race -shuffle=on ./...` -- expected: pass.
- `go mod tidy && git diff --exit-code go.mod go.sum` -- expected: clean.
- `pnpm --dir web test && pnpm --dir web build` -- expected: pass; coverage thresholds met.

## Spec Change Log

- 2026-09-29 — Orchestrator decisions at merge (no intent change):
  - Timeline rows gain the additive fields `product_id` (10) and `amount_cents` (11), so history can name and price a purchase. This is additive to the timeline contract.
  - Stat gains an optional `money` flag so the eye toggle masks money stats.
  - Activity variants read the catalog as an optional source (`uses`), fetched but never required, so home and history keep working when the catalog is down.

## Review Triage Log

### 2026-09-29 — Review pass
- verdicts: 27 findings — high 0, medium 2, low 22, false 1, maybe-false 2
- findings:
  - `[low]` `patch` Heading validation misses `$`-rooted variables and template nodes — collected or rejected, with tests.
  - `[low]` `patch` Title sources computed then discarded and recomputed — titleNeeds stored in screenDef; the self-guarding rule documented.
  - `[false]` `reject` A failed optional source leaves no trace — its sdui.snapshot.catalog span records the error status.
  - `[maybe-false]` `reject` The optional catalog fetch delays home up to the deadline — the catalog is served by account-sim, which home already requires; would only be low.
  - `[low]` `reject` Two negative-money conventions in bff.md — the `with_day_change` example is story 13's to align, and its implementer has been told.
  - `[low]` `patch` bff.md "home and Investir render" omits Carteira — added.
  - `[low]` `patch` bff.md lists `with_day_change` without saying it is not served — note added.
  - `[low]` `reject` A position with an unknown asset class counts in the summary but no list — the catalog classes are a fixed enum.
  - `[low]` `reject` Catalog health decides whether an empty-class section appears — the spec's catalog failure policy omits all position lists.
  - `[medium]` `patch` History shows aporte/saque rows with no amount — signed values per the artboard.
  - `[low]` `reject` reavaliacao rows will flood activity lists — story 13's spec defines the reavaliacao row and hides zero-change rows.
  - `[low]` `patch` No web test masks a history value — fixture row with a value added.
  - `[low]` `patch` No Carteira Raio-X or error-state web test — added.
  - `[medium]` `patch` The BFF end-to-end test hand-builds the indexer payload — uses the outbox row account-sim wrote.
  - `[low]` `reject` Masking percentages inconsistently — `money` masks money stats; position returns are percentages by design.
  - `[low]` `reject` ChangePercent allocates big.Int — negligible per-screen cost.
  - `[low]` `patch` The amount_cents zero semantics comment is incomplete — comment aligned with bff.md.
  - `[low]` `patch` A test ignores the json.Marshal error — checked.
  - `[low]` `patch` (dup of 1) `$`/template bypass — same fix.
  - `[low]` `patch` with/range bodies collect fields relative to the rebound dot — skipped inside bodies.
  - `[low]` `patch` (dup of 2) Title has no source guard — same fix; titles guard themselves.
  - `[low]` `reject` (dup of 8) Unknown asset class — same reason.
  - `[low]` `defer` The timeline HTTP route's new keys are untested — no consumer yet; recorded.
  - `[low]` `patch` (dup of 1) Verification-gap other finding: `$` and bare-dot headings — same fix.
  - `[low]` `reject` Intent: seed HTTP tests use an empty timeline — history is covered in screen fixtures and by the patched BFF end-to-end test.
  - `[maybe-false]` `reject` Intent: no desktop-viewport test for Carteira — story 18 owns desktop layouts; would only be low.
  - `[medium]` `defer` Intent: web fixtures are hand-built — carried deferral, recorded.

## Auto Run Result

**Summary:**
- **BFF.** It serves `GET …/screens/carteira` (`v1`) with the sections `summary` (`portfolio_summary`), `allocation` (`allocation_breakdown`), `positions_stocks`, `positions_etf` and `positions_fixed_income`, and `history` (`activity_list` `history`).
  - Each position section appears only when the class holds a position.
  - Every value is formatted in Go: `SignedMoney`, `ChangePercent` and `SignTone`, with largest-remainder shares.
  - Home activity and Carteira history share one timeline mapping (5 rows and 20 rows):
    - purchases read "Compra · {{product}}" "−US$ …";
    - aporte reads "+US$ …" `pos`;
    - saque reads "−US$ …".
  - Timeline rows carry `product_id` and `amount_cents`.
  - Heading templates declare their sources. Engine variants gain optional sources (`uses`), so the catalog names rows without being required.
- **Web.** It registers `portfolio_summary`, `allocation_breakdown` and `position_list` and renders the Carteira route. The eye toggle masks money.

**Files:**
- `internal/screen/{carteira,activity,catalog,engine,format,home,snapshot}.go`, `catalog.json` + tests
- `internal/bff/{screen,timeline}.go` + `screen_carteira_test.go`
- `internal/timeline/{index,grpc}.go`
- `proto/timeline/v1` + gen
- `web/src/sdui/components/{PortfolioSummary,AllocationBreakdown,PositionList}.tsx`, `registry`, `types`
- `web/src/screens/ClientAppScreen.tsx`
- tests and fixtures
- `specs/http/bff.md`

**Review:** 27 findings. 14 patched (2 medium, 12 low). 3 deferred, 2 of them recorded in frontmatter. 10 rejected, with the reasons in the triage log.

**Followup review recommended:** yes. Two medium entries were patched: history amounts, and the end-to-end payload taken from the real outbox row.

**Verification:**
- gofmt, vet and build are clean.
- `go test -race -shuffle=on ./...` passes.
- `go mod tidy` is clean.
- gen-proto changes only `timeline/v1`.
- web 282/282 passes with 100% coverage of `src/sdui/**`, and `pnpm --dir web build` passes.

**Visual check:** headless Chromium against account-sim, advisory, an in-memory timeline-indexer and the BFF, before the review patches. Screenshots are in the session scratchpad under `s11/`.
- Mariana at 390 px, clear and masked, and on desktop, plus Thiago and Fernanda at 390 px.
- Layout matches `Carteira-Mariana` and `Carteira-Desktop`, apart from story 13's day-change pill and day values.
- There is no horizontal scroll and there are no console errors.

**Residual risks:**
- Web fixtures are hand-built.
- The timeline HTTP route's new keys are untested.
- `reavaliacao` rows are still mapped by the default path until story 13.
