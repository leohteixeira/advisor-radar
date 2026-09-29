---
title: 'Build the Investir screen and the coded purchase form'
type: 'feature'
created: '2026-09-29'
status: 'done'
review_loop_iteration: 0
baseline_revision: 'e93842a'
followup_review_recommended: true
context:
  - '{project-root}/AGENTS.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/ux.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/architecture.md'
  - '{project-root}/specs/http/bff.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/stories/6-render-sdui-screens-in-web-and-migrate-the-home.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/stories/9-accept-purchases-and-raise-the-suitability-mismatch-alert.md'
warnings: []
deferred:
  - summary: >-
      Web SDUI fixtures are hand-built copies of the Go envelopes; nothing checks them against real BFF output.
    evidence: |-
      Carried from story 6; product() in web/src/test/sduiFixtures.ts recomputes above_profile. A golden-file export from Go would settle it.
    location: >-
      web/src/test/sduiFixtures.ts
    severity: medium
  - summary: >-
      The shared phase-2 dollars() parser reads "250.50" as 25050 dollars.
    evidence: |-
      Pre-existing helper shared by all POV panels; pt-BR uses the dot as a thousands separator, so the reading is consistent but ambiguous.
    location: >-
      web/src/screens/ClientAppScreen.tsx dollars()
    severity: low
---

<intent-contract>

## Intent

**Problem:** The client app has no Investir screen. The purchase route from story 9 has no UI. The home still falls back to the hardcoded phase-2 home. The CAP-4 demo is therefore not possible: Thiago buying "Tudo" or US$ 30,000 so that `idle_cash` disappears.

**Approach:**
- The BFF serves `GET …/screens/investir` (`v1`) with the sections `cash`, `highlights`, `fixed_income`, `etfs` and `stocks`, and the above-profile badge and warning.
- Web registers `invest_summary`, `product_rail` and `product_list`, renders the Investir route from the envelope, and adds the coded purchase form and the "Compra enviada" confirmation with Bastidores from `ux.md`.
- The phase-2 home fallback is removed, and `ClientPov.test.tsx` is updated to the SDUI app.

## Boundaries & Constraints

**Always:**

- Load `golang-how-to` first and apply the Go skills in `/workspace/repos/advisor-radar/CLAUDE.md` for the Go part.

- **BFF (`internal/screen`)**
  - Investir `v1` sections, in this order:
    1. `cash`: `invest_summary` `default`, with `cash_label`, `cash`, `cash_cents`, and `profile_chip` "Perfil {{profile}}".
    2. `highlights`: `product_rail` with variant `profile_{profile}`. `title` is "Para o seu perfil {{profile}}", `subtitle` is "Escolhidos pelo backend a partir do seu perfil de investidor.", and `products` is the deterministic pick: conservador → `tbill`, `corp`; moderado → `acoesg`, `corp`; arrojado → `cobalto`, `acoesg`.
    3. `fixed_income`: `product_list` `fixed_income` (`tbill`, `corp`), title "Renda fixa".
    4. `etfs`: `product_list` `etf` (`renda`, `acoesg`), title "ETFs".
    5. `stocks`: `product_list` `stocks` (`farol`, `cobalto`), title "Ações".
  - List membership comes from the account-sim catalog's `asset_class`, ordered by risk and then id.
  - Product props follow `specs/http/bff.md` "Product":
    - `product_id`, `name`, `class_label`, `risk`, `risk_label`, `return_label`, `minimum`, `minimum_cents`;
    - `above_profile` = risk > the `max_risk` from advisory `GetInvestorProfile` (story 8);
    - `badge` "Acima do seu perfil" and `warning` "Este produto tem risco {{risk}}. Seu perfil é {{profile}}, que vai até risco {{max}}. Você pode investir mesmo assim, e a sua assessora será avisada.", both set only when above profile;
    - `action` `{type: panel, label: "Investir", target: purchase, product_id}`.
  - The catalog comes from account-sim `ListProducts` as a new Snapshot source with its own error.
  - Failure policy:
    - catalog failure → the three lists and `highlights` are omitted;
    - profile failure → `highlights` is omitted, `cash` has no chip, and list products carry `above_profile: false` with no badge;
    - account failure → `cash` is omitted.
  - Heading: "Investir" / "Produtos fictícios · preço fixo da simulação".
  - Copy lives in `catalog.json`, and there is one test per variant.
  - `investir` now answers `200`. `carteira` and `perfil` stay `404`.
  - Update `specs/http/bff.md`.

- **Web**
  - Register `invest_summary`, `product_rail` and `product_list`.
  - Components follow the `Investir-Thiago`, `Investir-Desktop` and `OrlaApp.dc.html` artboards: five risk bars filled up to `risk`, warning color when `above_profile`, and the rail one column on phone and two on desktop.
  - The `/client-pov/{id}/investir` route fetches and renders the `investir` envelope. The home route keeps fetching `home`. `carteira` and `perfil` keep the placeholder note until their stories.
  - A `panel` `purchase` action with `product_id` is now shown and opens the coded purchase form (`Compra-Fernanda-Aviso`, `Compra-Enviada`, `OrlaApp.dc.html:421-500`). A purchase action without `product_id` stays hidden and is reported.
  - The purchase form reads the product (name, class, risk, return, warning) and `cash_cents` from the current Investir envelope's props. Web computes nothing else.
  - Form layout:
    - Title "Investir em {{name}}", the class and risk bars, and "Preço fixo da simulação, sem cotação. Rentabilidade fictícia.".
    - A "Quanto investir" field with the chips "US$ 250", "US$ 1.000" and "Tudo" (= cash). The line "Disponível no caixa: {{cash}}" uses the BFF `cash` display string.
    - The amount is capped at `cash_cents`, as a form convenience only.
    - Above profile, the BFF `warning` is shown and nothing is blocked.
    - Buttons "Confirmar compra de {{amount}}" and "Cancelar".
  - Amount formatting in the form (the typed value and the button label) reuses the existing phase-2 `formatCents` helper. This is presentation of user input, not a business value.
  - Submitting sends `POST …/purchases` `{product_id, amount_cents}` with a fresh `Idempotency-Key`, the way the other POV panels do.
  - Errors map as follows:
    - `422 insufficient` → "Valor acima do caixa disponível.";
    - `422 invalid` → "Confira o valor e tente de novo.";
    - `429`, `5xx` and network failures behave like the existing panels.
  - The "Compra enviada" confirmation shows:
    - the lead "{{amount}} em {{name}}. O valor saiu do caixa e já aparece na sua carteira.";
    - the protocol (the first two groups of `event_id`, uppercased, as in phase 2);
    - Bastidores `account.event.recorded · kind aplicacao · schema 3` with the `event_id`;
    - the steps "Gravado na outbox do account-sim", "Publicado no RabbitMQ", then "Regra: compra acima do perfil" when above profile or "Avaliado pelas regras" otherwise, then "Na fila da assessoria";
    - the buttons "Ver na fila do time" (`/advisor-radar/fila`, i.e. the app's queue route) and "Voltar ao início".

    Returning home re-fetches the home screen.
  - **Remove the phase-2 home fallback.**
    - When the screen request fails, times out, or returns a non-envelope, the screen area shows an error state: `role="alert"` "Não foi possível montar sua tela." plus a "Tentar de novo" button that re-fetches. The shell, tabs, simulation strip and panels stay.
    - Delete the phase-2 home components that become unused, and resolve story 6's deferred item.
    - Update `web/src/screens/ClientPov.test.tsx` so its stubs serve SDUI envelopes (reusing `web/src/test/sduiFixtures.ts`). Keep every behavior it asserted: greeting, empty activity text, deposit/withdraw/complaint panels, desktop theme and message, the 404 error state, and the tab note (now only for carteira/perfil).
    - The panels keep reading phase-2 data as before.
  - Coverage of `src/sdui/**` stays at 100% on all four metrics. New tests cover:
    - each new type and variant, including an above-profile and a within-profile product;
    - the purchase form: chips, cap, warning, submit success, 422s, cancel;
    - the confirmation steps for both rule outcomes;
    - the Investir route;
    - the error state and retry;
    - Thiago's "Tudo" purchase, after which the home fixture no longer carries `idle_cash`, with a stubbed BFF.

- **General**
  - pnpm only. Shipped UI copy is Portuguese.
  - Visual check: screenshot `/client-pov/{Thiago}/investir` and the purchase form at 390 px and desktop against `Investir-Thiago`, `Investir-Desktop`, `Compra-Fernanda-Aviso` and `Compra-Enviada` with the cached headless Chromium, and record the result.

**Never:**
- No Carteira or Perfil screens, day change, beta `v2` home, Raio-X, or `X-SDUI-Schema` (stories 11–16).
- Web never computes above-profile, percentages, or business formatting of BFF values.
- No npm, yarn or bun.
- Unit tests do not dial 5435/5673/8400/8420/9201.
- Do not read or edit `.env` / `.env.*`.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Thiago Investir | arrojado, cash 6052000 | `cash` "US$ 60.520,00" + "Perfil arrojado"; highlights cobalto, acoesg; no badge anywhere | — |
| Fernanda Investir | conservador (max 2) | acoesg, farol, cobalto carry badge "Acima do seu perfil" + warning; bars in warning color | — |
| Purchase above profile | Fernanda buys cobalto US$ 250 | form shows warning, not blocked; `202`; confirmation with "Regra: compra acima do perfil" | — |
| Tudo | Thiago taps "Tudo" on acoesg | amount = cash; `202`; returning home shows no `idle_cash` (cash now 0 of 68000) | — |
| Over cash | typed amount > cash | capped at cash in the form; if the BFF still answers `422 insufficient`, message shown | — |
| Catalog down | ListProducts fails | `200` with only `cash`; lists and highlights in `omitted` | — |
| Profile down | GetInvestorProfile fails | highlights omitted; no badges; no chip | — |
| Screen fails | any screen request fails | error state with retry; no phase-2 home | — |
| Purchase action without product_id | malformed action | hidden + reported | `console.error` |

</intent-contract>

## Code Map

- `internal/screen/{snapshot,catalog,engine,home}.go`, `internal/screen/catalog.json` -- add Investir sections, the catalog source, and variants.
- `internal/bff/screen.go`, `internal/bff/account.go` -- the screen route slug set and a `ListProducts` adapter.
- `internal/sim/grpc.go`, `proto/account/v1` -- `ListProducts` (story 4).
- `web/src/sdui/{registry,actions,api,SduiScreen,types}` and `web/src/sdui/components/*` -- story 6 renderer.
- `web/src/screens/ClientAppScreen.tsx` -- panels (`MoneyPanel`, `send`, Bastidores steps), fallback home (remove), tab routing.
- `web/src/screens/ClientPov.test.tsx`, `web/src/test/{sdui.tsx,sduiFixtures.ts}` -- tests and fixtures.
- `docs/design/sdui-full-pov/project/{OrlaApp,Investir-Thiago,Investir-Desktop,Compra-Fernanda-Aviso,Compra-Enviada}.dc.html` -- artboards.

## Tasks & Acceptance

**Execution:**
- BFF: Investir sections, catalog source, failure policy, docs, tests.
- Web: three components, the Investir route, the purchase form and confirmation, removal of the fallback, updated `ClientPov.test.tsx`, and tests at 100% `src/sdui/**` coverage.

**Acceptance Criteria:**
- Given the local stack, when Thiago opens Investir and buys "Tudo" in acoesg, then the purchase answers `202`, the confirmation shows the protocol and Bastidores, and his home no longer shows `idle_cash`.
- Given the change, when the Go suite, `pnpm --dir web test`, and `pnpm --dir web build` run, then all pass with 100% `src/sdui/**` coverage.

## Verification

**Commands:**
- `gofmt -l . && go vet ./... && go build ./...` -- expected: clean.
- `mkdir -p .gotmp && GOTMPDIR=$PWD/.gotmp go test -race -shuffle=on ./...` -- expected: pass.
- `go mod tidy && git diff --exit-code go.mod go.sum` -- expected: clean.
- `pnpm --dir web test && pnpm --dir web build` -- expected: pass; coverage thresholds met.

## Review Triage Log

### 2026-09-29 — Review pass
- verdicts: 30 findings — high 0, medium 7, low 21, false 1, maybe-false 1
- findings:
  - `[medium]` `patch` Every screen fetches every source (home pays for the catalog, Investir for timeline/cases) — fetch only the plan's needed sources.
  - `[medium]` `patch` Purchase form ignores the product minimum — show the minimum and disable Confirm below it.
  - `[medium]` `patch` A retry after a network error/5xx gets a fresh key and can buy twice — one key per open form and amount.
  - `[medium]` `patch` A late purchase answer overrides the view the user moved to — result dropped when the panel is closed.
  - `[medium]` `patch` Within-profile purchase leaves "Na fila da assessoria" waiting forever — uses the BFF's "se uma regra disparar" label when not above profile.
  - `[low]` `defer` dollars() reads a dot as a thousands separator — pre-existing shared helper; recorded.
  - `[low]` `patch` Default-variant invariant test skips every product_rail — scoped to (investir, highlights).
  - `[low]` `reject` Highlight picks are not validated against max_risk — the spec fixes the picks; a catalog risk change would still badge the product.
  - `[low]` `reject` One bad product drops the whole section — build_error semantics are the engine's contract; the catalog is fixed seed data.
  - `[low]` `patch` No focus management or aria-describedby on the form — focus moves to the new heading; input described by cash and warning.
  - `[low]` `patch` Fernanda cobalto test indexes without a length check and ignores the action — length check and action assertion.
  - `[low]` `patch` bff.md garbled above_profile sentence and missing refusal list — rewritten.
  - `[low]` `patch` CSS selector depends on the aria-label copy — modifier class.
  - `[low]` `reject` highlightPicks is a mutable package map — never mutated; no caller writes it.
  - `[medium]` `patch` (dup of 4) Late 202 forces the bought panel — same fix.
  - `[medium]` `patch` (dup of 3) Retry with a new UUID buys twice — same fix.
  - `[medium]` `patch` (dup of 2) US$ 250 chip below the corp minimum — same fix.
  - `[low]` `patch` The "US$ 1.000" chip shows pressed when the cap is below it — "Tudo" pressed in that case.
  - `[low]` `patch` The value mask is not applied in the form and confirmation — masked money rendered.
  - `[low]` `patch` A failed shell re-read after the first load replaces the app with a terminal error — error only before the first load.
  - `[low]` `patch` Browser back/forward keeps the purchase panel over another tab — panel reset on tab change.
  - `[medium]` `patch` (dup of 1) Home fetches the catalog — same fix.
  - `[false]` `reject` Negative or non-integer cash_cents — the BFF formats cash_cents from account-sim integer cents, which the store keeps non-negative.
  - `[maybe-false]` `reject` Huge typed amount with cash omitted exceeds the safe integer range — account-sim rejects above MaxAmountCents with 422; would only be low.
  - `[low]` `patch` Confirm disabled while a POST is pending is untested — pending-POST test added.
  - `[low]` `patch` Tab change shows the skeleton, not the previous screen, untested — pending tab-change test added.
  - `[low]` `patch` openPurchase not-found branch untested — test added.
  - `[low]` `reject` Intent: the confirmation's rule step is predicted from above_profile rather than the rule stream — the spec defines it from the envelope flag.
  - `[medium]` `defer` Intent: web fixtures drift from the Go envelopes — carried story 6 deferral; recorded.
  - `[low]` `patch` Intent: the demo outcome is only proven against a stubbed BFF — live acceptance check with the real stack requested and recorded in the result.

## Auto Run Result

**Summary:**
- **BFF.** It serves `GET …/screens/investir` (`v1`) with the sections `cash`, `highlights`, `fixed_income`, `etfs` and `stocks`.
  - Product props, the "Acima do seu perfil" badge and the warning follow the spec, with copy in `catalog.json`.
  - The account-sim catalog is a new Snapshot source, and each screen now fetches only the sources its plan needs.
  - The failure policy is in place for catalog, profile and account failures. `carteira` and `perfil` still answer 404.
- **Web.**
  - It registers `invest_summary`, `product_rail` and `product_list` (with `RiskBars`) and renders the Investir route.
  - It adds the coded purchase form and the "Compra enviada" confirmation with Bastidores:
    - chips, the cash cap, and the minimum;
    - the above-profile warning, which does not block;
    - one Idempotency-Key per open form and amount;
    - masking and focus management.
  - The phase-2 home fallback is removed. A failed screen now shows "Não foi possível montar sua tela." with "Tentar de novo".

**Files:**
- `internal/screen/{investir,catalog,engine,snapshot,format,home,page}.go`, `catalog.json` + tests
- `internal/bff/{screen,account,pov,display}.go` + `screen_investir_test.go`
- `web/src/sdui/{registry,types,actions,context,purchase,SduiScreen}` and `components/{InvestSummary,ProductRail,ProductList,RiskBars}`
- `web/src/screens/{ClientAppScreen,PurchasePanel}.tsx`
- tests: `ClientPov`, `ClientInvestir`, `investir`, `ClientHome`, `ClientXray`, `actions`
- fixtures, `app.css`
- `specs/http/bff.md`
- story 6's deferred note, marked resolved

**Review:** 30 findings.
- 22 patched: 5 distinct medium issues, which account for 9 medium rows once duplicates are counted, plus 13 low.
- 2 deferred: fixture drift, carried from story 6, and the `dollars()` dot parsing.
- 6 rejected, with the reasons in the triage log.

**Followup review recommended:** yes. Five medium entries were patched, including idempotency-key reuse on retry, dropping late answers, and source selection per plan.

**Verification:**
- gofmt, vet and build are clean.
- `go test -race -shuffle=on ./...` passes.
- `go mod tidy` is clean.
- `pnpm --dir web test` passes 258/258 with 100% coverage of `src/sdui/**` on all four metrics.
- `pnpm --dir web build` passes; it warns that the chunk is over 500 kB.

**Live acceptance check:** account-sim, advisory and BFF were built from the change, with Postgres 5435 and a throwaway RabbitMQ.
- Thiago bought "Tudo" in acoesg (US$ 60.520,00) and got `202`.
- The confirmation showed the protocol `01A0EC3B-79FF` and the Bastidores line `account.event.recorded · kind aplicacao · schema 3`.
- The steps read: outbox feito, RabbitMQ feito, Avaliado feito, and "Na fila da assessoria, se uma regra disparar" aguardando, which is expected within his profile.
- His home then moved from `idle_cash` to `welcome`.
- The databases were reseeded afterwards.

**Visual check:** headless Chromium at 390 px and on desktop. Screenshots are in the session scratchpad under `s10/`.
- Investir for Thiago matches `Investir-Thiago` and `Investir-Desktop`.
- Fernanda's cobalto form matches `Compra-Fernanda-Aviso`, plus the added "Mínimo" line.
- The confirmation matches `Compra-Enviada`; its "Dia simulado" strip belongs to story 13.

**Residual risks:**
- Web fixtures are hand-built copies of the Go envelopes.
- The purchase form lives outside the `src/sdui/**` coverage gate; it is tested at app level.
- The rule step is predicted from `above_profile` as specified; when the profile source is down, it can disagree with advisory.
