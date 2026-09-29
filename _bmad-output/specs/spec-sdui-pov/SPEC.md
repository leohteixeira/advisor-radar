---
id: SPEC-sdui-pov
companions:
  - architecture.md
  - architecture-diagrams.md
  - ux.md
  - ../spec-client-pov/SPEC.md
  - ../spec-client-pov/architecture.md
  - ../spec-client-pov/ux.md
  - ../spec-advisor-radar/SPEC.md
  - ../spec-advisor-radar/architecture.md
  - ../../../docs/design/sdui-full-pov/README.md
  - ../../../docs/design/sdui-full-pov/project/OrlaApp.dc.html
  - ../../../docs/design/sdui-full-pov/project/Catalogo.dc.html
  - ../../../docs/design/sdui-full-pov/project/Main.dc.html
  - ../../../docs/design/sdui-full-pov/project/Selecao-SDUI-mobile.dc.html
sources:
  - ../../../../../docs/advisor-radar/advisor_radar_sdui_brief.md
---

> **Canonical contract.** This SPEC and the files in `companions:` are the complete, preservation-validated contract for what to build, test, and validate. Source documents listed in frontmatter are for traceability — consult them only if you need narrative rationale or prose color this contract intentionally omits.

# Advisor Radar phase 3: Server-Driven UI in the client app

## Why

An **opportunity** and a **pain**. The target squad builds Server-Driven UI (SDUI) in Go with internal gRPC, and the demo shows none of it. Meanwhile the client app contradicts the repository rule that business logic stays in the backend: `ClientAppScreen.tsx` (about 1,040 lines) hardcodes copy, allocation percentages, currency formatting, the over-cash withdrawal check, and Bastidores labels that duplicate the BFF. Every client sees the same home, so phase-1 and phase-2 events move the advisor queue but change nothing on the client side. Investir, Carteira, and Perfil are placeholders. BFF POV responses are `map[string]any`, and `activity` and `messages` are always empty. The code also diverges from ADR 0008: the BFF runs account-sim logic in process. Phase 3 lets the backend compose each screen for the client's moment. It makes the three tabs real, including a purchase that reaches the advisor queue.

## Capabilities

- **CAP-1**
  - **intent:** The client app gets each of its four screens from the BFF as a page of sections and components, already filled for that client, and only renders them.
  - **success:** `GET /v1/client-pov/customers/{id}/screens/{slug}` returns the typed envelope in `architecture.md` for `home`, `investir`, `carteira`, and `perfil`. Opening a screen makes one HTTP request. Web maps the 15 types in `ux.md` to components and does no percentage, currency, date, validation, or SLA logic. Changing copy, section order, or a variant of an existing type needs no web change. `ClientPov.test.tsx` passes before and after the home migration.

- **CAP-2**
  - **intent:** The home shows each client a moment chosen from their own situation, and the moment changes live when that situation changes.
  - **success:** After reseed, Fernanda opens on `segment_upgrade_near`, Thiago on `idle_cash`, and Mariana on `portfolio_review`. A USD 10,000 deposit by Fernanda switches her home to `segment_upgraded`. A complaint by Mariana switches her home to `case_open` with the Singular SLA. A purchase by Thiago removes `idle_cash`. Advancing to the shock day switches Mariana to `portfolio_drop`. The moment comes from advisory over gRPC; the BFF picks the variant by the priority in `architecture.md`.

- **CAP-3**
  - **intent:** One broken component or one slow data source never takes a screen down.
  - **success:** Tests cover each case: an unknown `type` renders nothing and is reported; a React component that throws is caught by its own boundary; a Go `Build` error drops only that component; each `Snapshot` source failing or exceeding the screen deadline makes its sections fall back to the default variant, or be omitted when they depend only on that source. The screen still answers `200` and renders the rest.

- **CAP-4**
  - **intent:** A client can browse fictional products suited to their investor profile and buy one, and a purchase above the profile reaches the advisor queue.
  - **success:** Investir shows the profile highlights and the renda fixa, ETF, and ações lists from the server. The coded purchase form caps the amount at cash and answers `202` with `event_id`, the protocol, and Bastidores. The purchase moves cash into a position at a fixed price and publishes `account.event.recorded` `kind: aplicacao`. The same `Idempotency-Key` yields one event. An amount above cash is `422` with no event. A product above the profile shows a BFF-supplied badge and warning, does not block, and makes advisory raise a suitability-mismatch alert (card kind `perfil`, no automatic case). As Fernanda, a risk-5 purchase reaches the queue in under 2 s at p95.

- **CAP-5**
  - **intent:** A client can see total wealth, allocation, each position, and their movement history.
  - **success:** Carteira shows patrimony, allocation by class with backend percentages, per-class position lists with applied value, current value, and return, and a history from the timeline over gRPC. The same history fills home `activity`, which is no longer empty after an action.

- **CAP-6**
  - **intent:** A visitor can advance one shared fictional market day, and every portfolio is revalued the same deterministic way.
  - **success:** `POST /v1/client-pov/simulation/advance-day` advances the global day once per `Idempotency-Key`. It publishes one `reavaliacao` per account for that day, and a replay publishes nothing new. Positions follow a pure function of product and day, so equal holdings move equally. Day 0 equals the phase-2 values, and reseed returns to day 0. On day 3, the shock day, Mariana's loss crosses 15% of patrimony and advisory raises the drop alert. Any other holder alerts only past their own threshold.

- **CAP-7**
  - **intent:** A client can see their investor profile, fictional registration data, advisor, preferences, and beta enrollment.
  - **success:** Perfil marks the client's level on the conservador, moderado, and arrojado scale, with each level's description and max risk and the last assessment date. The profile is read-only and comes from the advisory book. Registration data uses `@example.com` e-mail and a masked phone. The contact channel (chat or e-mail) and the beta toggle persist in account-sim and are read over gRPC. The theme stays local to the browser.

- **CAP-8**
  - **intent:** Each screen is versioned, and clients in the beta segment receive a newer revision before everyone else.
  - **success:** Every response carries `schema_version`, `slug`, and `revision`. When the `X-SDUI-Schema` range excludes the server's version, the BFF answers `406` with the supported range. Turning beta on in Perfil serves home revision `v2`, which adds the profile highlights after the moment, on the next load. Turning it off restores `v1`.

- **CAP-9**
  - **intent:** An evaluator can see from telemetry how a screen was built and which variant each client saw.
  - **success:** One span per screen build and one per section carry `sdui.slug`, `sdui.revision`, `sdui.section`, and `sdui.variant`. Snapshot calls are child spans. `sdui_variant_served_total{slug,section,variant}`, `sdui_component_dropped_total{slug,type,reason}`, build latency per screen, purchases by class, and suitability-mismatch alerts are exported.

- **CAP-10**
  - **intent:** A content operator can add or change a section variant through an LLM skill without touching web code.
  - **success:** `.claude/skills/add-sdui-variant/SKILL.md` takes section, moment, and copy. It adds the catalog entry, the `Variant` rule, and its tests, and updates `specs/http/bff.md`. In the demo, a variant requested for Mariana appears after restarting only the BFF, without a web build.

- **CAP-11**
  - **intent:** A visitor learns on the selection screen that the client app is server-driven, and can inspect it live inside the app.
  - **success:** `/advisor-radar` shows the "Novo · Server-Driven UI" band, the Arquitetura and Server-Driven UI anchors, the fifth pillar, and the `#sdui` section described in `ux.md`, on desktop and at 390 px. The client app's simulation strip shows the simulated day, "Avançar um dia", and "Raio-X SDUI", which outlines each section with `id · type · variant`.

- **CAP-12**
  - **intent:** An evaluator can read the phase-3 decisions without the code and find that the running code follows the written ADRs.
  - **success:** ADR 0009 (SDUI in the BFF) and ADR 0010 (individual positions, revoking phase-2 decision 9) exist. ADR 0008 carries a note that the code now follows it. account-sim serves `account/v1` gRPC for deposit, withdrawal, message, complaint, and the account query. `cmd/bff/povmem.go` is gone, and POV responses are typed DTOs. The bff and account-sim POV tests pass before and after. `specs/http/bff.md` documents screens and components, and the README carries the phase-3 demo script.

## Constraints

- Phase-1 and phase-2 rules hold. There is one database per service, an outbox on every publisher, and an inbox on every consumer. The BFF stores nothing. POV commands need `Idempotency-Key` and a rate limit. Web talks only to the BFF. Advisor, analyst, and manager screens keep their behavior; they only gain the suitability-mismatch alert.
- The BFF composes screens. Advisory owns segmentation, investor profile, and client moments; the BFF never decides a moment, it only picks the variant for the moment it receives.
- The catalog is a versioned file embedded in the BFF with `go:embed`. It is read-only config: no CMS and no runtime writes. The `add-sdui-variant` skill is the editing tool.
- Components are semantic domain types; there are no `row`, `column`, or `text` primitives. The server controls section order and point props (tone, emphasis). Style and breakpoint layout stay in web. A new `type` needs web code and a contract entry; a new variant of an existing type does not.
- Values arrive ready to display (`"US$ 48.210,00"`); forms also receive integer cents. Numbers and dates are formatted in Go before `text/template`. An invalid template fails a test, not production.
- Every section has a default variant. The first matching variant wins, and the default matches always.
- There is no new event type. `aplicacao` and `reavaliacao` ride on `account.event.recorded` at `schema_version` 3, and consumers accept 1, 2, and 3. Patrimony is positions at market value plus cash, in the app, book, rules, and moments. account-sim is the source of truth, and advisory follows it through events.
- The simulated day is global. It starts at 0 with the phase-2 values, advances only by command, and returns to 0 on reseed. Advance-day has its own global limit of 20 per 10 minutes. There is no cron and no wall clock in returns. One transaction writes the new day, the revalued positions, and one `reavaliacao` outbox row per account, keyed `reavaliacao:{customer_id}:{sim_day}`.
- A product above the investor profile warns and never blocks the purchase. The resulting alert opens no case by itself. The investor profile is read-only with no questionnaire. Preferences and beta are account-sim state read over gRPC; they publish no event.
- Phase A comes first. The `account/v1` gRPC server and the removal of the in-process simulator land before purchase and preferences, which are born on the gRPC path. The BFF-to-account-sim connection carries deadline propagation and retry from Phase A; retry is safe because every command carries `Idempotency-Key`. Tracing interceptors come with CAP-9.
- Targets are one HTTP request per screen, a screen endpoint p95 under 150 ms locally, purchase-to-alert p95 under 2 s, and zero duplicate purchases per `Idempotency-Key`.
- Info logs omit rendered copy, registration data, and full account values. UI copy is Portuguese. Code, catalog keys, ADRs, and repository docs are English. All data is fictional: no real product, ticker, brand, or institution. The public talk that inspired the model contributes no name, brand, or content.
- If time runs short, cut only in the order in `architecture.md`, and never cut the items it lists as uncuttable.

## Non-goals

- A CMS of any kind, or runtime catalog edits.
- Generic layout primitives, and server-driven theme or layout beyond tone and emphasis.
- Real quotes, price moves outside the simulated day, market sells, pending orders, cancellation, taxes, and real FX.
- The investor-profile questionnaire, and editing registration data.
- Pagination, infinite scroll, and reloading a single section.
- Random A/B splits. The only segment is beta, by choice in Perfil.
- Internationalization. Copy stays Portuguese.
- LLM product recommendations. Investir highlights are a deterministic rule per profile.
- Authentication and notifications beyond phase 2.

## Success signal

On a phone, the interviewer opens the `#sdui` section, then opens the app as Fernanda, Thiago, and Mariana and sees three different homes. "Raio-X SDUI" shows each section's `id · type · variant`. Thiago buys a fictional ETF and his `idle_cash` card disappears. Fernanda, a conservador, buys a risk-5 product through the warning, and the suitability-mismatch alert appears on the team queue in under 2 s. A variant for Mariana, requested through `add-sdui-variant`, shows after restarting only the BFF. Advancing to the shock day revalues Mariana's Carteira, raises the drop alert, and switches her home to `portfolio_drop`. The trace shows one span per section with its variant.
