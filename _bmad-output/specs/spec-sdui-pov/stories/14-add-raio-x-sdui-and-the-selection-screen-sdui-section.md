---
title: 'Add Raio-X SDUI and the selection-screen SDUI section'
type: 'feature'
created: '2026-09-29'
status: 'done'
review_loop_iteration: 0
baseline_revision: 'ffdbcc9a6bfe12a5a0dfe69fc58fc9617665f78c'
followup_review_recommended: false
context:
  - '{project-root}/AGENTS.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/ux.md'
  - '{project-root}/specs/http/bff.md'
  - '{project-root}/docs/design/sdui-full-pov/README.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/stories/6-render-sdui-screens-in-web-and-migrate-the-home.md'
warnings: []
deferred:
  - summary: >-
      At 390 px the selection-screen walkthrough "Pausar" control overflows, making the page 438 px wide.
    evidence: |-
      Pre-existing before this story (seen in the visual check); carried into story 18, which owns layout fidelity.
    location: >-
      web/src/screens/SelectionScreen.tsx
    severity: low
---

<intent-contract>

## Intent

**Problem:** The demo cannot show that the screen is server-driven. The client app cannot reveal the sections, types, variants, and omitted sections behind the screen. The selection screen at `/advisor-radar` says nothing about Server-Driven UI.

**Approach:**
- Add a web-only Raio-X demo tool to the client app's simulation strip. It outlines each section, shows a banner with the request and envelope metadata, and draws omitted sections as dashed placeholders.
- Add the selection-screen changes from `ux.md` "Selection screen" at desktop and 390 px:
  - the anchors and the "Novo" chip;
  - the "Novo nesta versão" band;
  - the fifth pillar and the added "compra acima do perfil" text;
  - an interactive `#sdui` section that fetches each seed client's live home envelope.

## Boundaries & Constraints

**Always:**
- **Raio-X toggle.** The strip button is "Raio-X SDUI" ("Raio-X" on phone). It is a toggle with `aria-pressed`, and the state lives in the client app only (a `localStorage` convenience is fine, wrapped in try/catch).
- **Raio-X on.** Every rendered section is outlined and labelled `{{id}} · {{type}} · {{variant}}`, using the first component's type and variant.
- **Banner.** It shows `GET /v1/client-pov/customers/{{id}}/screens/{{slug}}` and "slug {{slug}} · revision {{revision}} · schema {{schema_version}} · {{count}} seções". `count` is the number of rendered sections. For each distinct omitted `reason` that names a source, it adds "{{reason}} fora do ar" (e.g. "timeline fora do ar"). A `build_error` reason reads "erro ao montar {{id}}".
- **Omitted sections.** Each one renders only under Raio-X, as a dashed placeholder "`{{id}} · {{type}} · omitido`", at the position given by the catalog order. Web has no catalog, so append placeholders after the rendered sections in `omitted` order. Match `Home-Thiago-RaioX`, `Home-Desktop-RaioX`, and `Home-Falha-Isolada` as closely as that allows, and record any deviation.
- **Raio-X off.** Nothing about Raio-X is in the DOM.
- **Where it runs.** Raio-X works on every SDUI screen: home and whatever screens exist.
- **Selection screen** (`web/src/screens/SelectionScreen.tsx`), copy and layout from `Main.dc.html` and `Selecao-SDUI-mobile.dc.html`:
  - The header gains "Arquitetura" and "Server-Driven UI" anchors, the latter with a "Novo" chip.
  - The band "Novo nesta versão · Server-Driven UI" sits under the view cards, with its two texts and a "Ver como funciona" link to `#sdui`.
  - A fifth pillar "Telas pelo servidor" is added, and the alert pillar gains "compra acima do perfil".
  - `#sdui` follows `#arquitetura`, with the heading "O backend monta a tela de cada cliente".
  - Client tabs: Fernanda · Essencial, Thiago · Advance, Mariana · Singular. The segment labels come from the POV list endpoint where available, otherwise from design copy.
  - The selected client's live `GET …/screens/home` response is shown with a "200 · 1 requisição" line and pretty-printed JSON, trimmed with a disclosure if long.
  - The panel "Momento (advisory, gRPC)" shows the `moment` component's variant.
  - "A regra que escolheu a variante" shows the documented priority row for that variant, as a static copy map in web keyed by variant name that mirrors the `specs/http/bff.md` priority table. This is documentation copy, not a computation.
  - The rendered home uses the same `SduiScreen` in a phone frame.
  - Then the five steps and four principles with the `Main.dc.html` copy.
  - When the BFF is unreachable, the section shows a neutral note, and the rest of the selection page is unaffected.
- **Tests.**
  - Update `SelectionScreen.test.tsx` expectations only where copy intentionally changed. Every pre-existing assertion that still describes shipped behaviour must stay.
  - New tests cover:
    - the Raio-X toggle, labels, banner, omitted placeholders, and the "fora do ar" text;
    - the `#sdui` tabs switching clients, the live fetch, and the BFF-down note;
    - `aria-pressed`.
  - Keep 100% coverage of `src/sdui/**`. Raio-X code under `src/sdui/` is covered.
- **Visual check.** Compare at 390 px and desktop against `Home-Thiago-RaioX`, `Home-Desktop-RaioX`, `Home-Falha-Isolada`, `Main.dc.html`, and `Selecao-SDUI-mobile.dc.html`, and record the result.
- pnpm only. Portuguese UI copy.

**Never:**
- No BFF or Go change.
- Raio-X is not shown to the team-side routes.
- Web does not compute which variant should win.
- No npm, yarn, or bun.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Raio-X on, Thiago | 5 sections | 5 outlines with `id · type · variant`; banner "slug home · revision v1 · schema 1 · 5 seções" | — |
| Timeline down | `omitted: activity/timeline` | dashed "activity · activity_list · omitido"; banner "timeline fora do ar" | — |
| Raio-X off | default | no outline, banner, or placeholder in DOM | — |
| Selection `#sdui` | pick Mariana | fetches her home; shows variant and its rule; renders home | — |
| BFF down on selection | fetch fails | neutral note; rest of page intact | — |
| 390 px | phone | layout of `Selecao-SDUI-mobile` | — |

</intent-contract>

## Code Map

- `web/src/screens/ClientAppScreen.tsx` -- simulation strip.
- `web/src/sdui/SduiScreen.tsx` -- section rendering, spans.
- `web/src/screens/SelectionScreen.tsx` (+ test) -- selection page.
- `docs/design/sdui-full-pov/project/{Main,Selecao-SDUI-mobile,Home-Thiago-RaioX,Home-Desktop-RaioX,Home-Falha-Isolada,OrlaApp}.dc.html` -- artboards and copy.

## Tasks & Acceptance

**Execution:**
- Raio-X in `web/src/sdui/` plus the strip toggle, with tests.
- Selection-screen changes and the `#sdui` section, with tests.

**Acceptance Criteria:**
- Given the local stack, when the visitor turns on Raio-X on Thiago's home, then every section shows its `id · type · variant` and the banner matches the envelope. With the timeline down, the omitted placeholder and "timeline fora do ar" appear.
- Given the change, when `pnpm --dir web test` and `pnpm --dir web build` run, then all pass with 100% `src/sdui/**` coverage.

## Verification

**Commands:**
- `pnpm --dir web test && pnpm --dir web build` -- expected: pass; coverage thresholds met.

## Review Triage Log

### 2026-09-29 — Review pass
- verdicts: 32 findings — high 0, medium 0, low 29, false 3, maybe-false 0
- findings:
  - `[low]` `[patch]` verification-gap: roster timeout and signal forwarding untested — fake-timer test added.
  - `[low]` `[patch]` verification-gap: late screen response after a tab switch untested — deferred-response test added.
  - `[low]` `[patch]` verification-gap: disclosure reset on client switch untested — test added.
  - `[low]` `[patch]` verification-gap other: Raio-X toggle shown where it has no effect — grouped with the toggle fix.
  - `[false]` `[reject]` intent: Raio-X wired only on home — home is the only SDUI screen until stories 10–12; the renderer support is generic.
  - `[low]` `[patch]` intent: no recorded visual check or deviation note — recorded in the Auto Run Result.
  - `[low]` `[patch]` intent: "200 · 1 requisição" static — grouped with the real-status fix.
  - `[low]` `[reject]` intent: copy asserted against its own constants — the constants are the design copy; a cross-file check against `.dc.html` adds tooling for no user-facing gain.
  - `[low]` `[patch]` intent: rule map not tied to `bff.md` — grouped with the derived count; a web-to-Go check is not added.
  - `[false]` `[reject]` intent: placeholder repeats the label with body copy — matches the `Home-Falha-Isolada` artboard.
  - `[low]` `[patch]` blind: `moments`/`profile` reasons not mapped — mapped to advisory.
  - `[low]` `[patch]` blind: banner URL unencoded — encoded like the request.
  - `[low]` `[patch]` blind: Raio-X toggle has no effect on fallback/loading/non-SDUI tabs — shown only with an SDUI screen.
  - `[low]` `[patch]` blind: missing client reported as BFF down — distinct note.
  - `[false]` `[reject]` blind: "Momento (advisory, gRPC)" label contradicts the rule source — the label is `ux.md` copy; the rule panel names the fact owner.
  - `[low]` `[patch]` blind: omitted `moment` shown as an undocumented variant — says the section was omitted with its reason.
  - `[low]` `[patch]` blind: tabs match clients by first name — matched by seed customer id.
  - `[low]` `[patch]` blind: rule count is a separate constant — derived from the map.
  - `[low]` `[reject]` blind: principles describe spans and beta not yet shipped — stories 16 and 17 of this epic ship them; the copy is the design's.
  - `[low]` `[patch]` blind: status line hard-coded — renders the real status.
  - `[low]` `[patch]` blind: no retry after the roster fails — re-fetch on tab click while down.
  - `[low]` `[patch]` blind: no recorded deviations or visual result — recorded in the Auto Run Result.
  - `[low]` `[patch]` blind: stale-response handling untested — grouped with the deferred-response test.
  - `[low]` `[patch]` blind: dead `.selection__pillars` grid rule — deleted.
  - `[low]` `[patch]` edge: `moments`/`profile` mapping — grouped.
  - `[low]` `[patch]` edge: dangling separator with an empty variant — joins non-empty parts.
  - `[low]` `[patch]` edge: banner encoding — grouped.
  - `[low]` `[patch]` edge: POV item without a string name crashes render — grouped with the id match and item guard.
  - `[low]` `[patch]` edge: duplicate first names — grouped with the id match.
  - `[low]` `[patch]` edge: roster stays down — grouped with the retry.
  - `[low]` `[patch]` edge: empty variant chip — `variant || '—'`.
  - `[low]` `[reject]` edge: a section with no components is labelled by id only — nothing else exists to label; intended.

## Auto Run Result

Status: done

- **Summary:**
  - **Raio-X.** The strip toggle is labelled "Raio-X SDUI" on desktop and "Raio-X" on phone. It uses `aria-pressed`, is remembered per browser, and is shown only while an SDUI screen is on view.
    - Each section is outlined and labelled `id · type · variant`.
    - The banner shows the encoded request path and "slug · revision · schema · N seções", followed by "{{reason}} fora do ar" or "erro ao montar {{id}}".
    - Omitted sections appear as dashed placeholders.
    - None of this is rendered when the toggle is off.
  - **Selection screen.**
    - The header gains the anchors and the "Novo" chip.
    - The "Novo nesta versão" band is added.
    - A fifth pillar is added, and the alert pillar mentions "compra acima do perfil".
    - An interactive `#sdui` section follows, with seed-client tabs matched by customer id. For the selected client it shows the live home JSON with its real status, the moment variant, and the documented priority rule. It renders the home read-only in a phone frame, followed by the five steps and four principles. It has a timeout, retries after a failure, and shows neutral notes when a client is missing or the BFF is down.
- **Files:**
  - `web/src/sdui/Xray.tsx`: new.
  - `web/src/sdui/SduiScreen.tsx`: the `xray` prop.
  - `web/src/sdui/api.ts`: `fetchScreenResponse` with status.
  - `web/src/api/pov.ts`: abort signal.
  - `web/src/screens/ClientAppScreen.tsx`: strip toggle.
  - `web/src/screens/SelectionScreen.tsx`: anchors, band, pillar, `#sdui`.
  - `web/src/selection/{SduiShowcase.tsx,sdui.ts}`: the showcase and its copy.
  - `web/src/styles/app.css`: styles.
  - Tests: `web/src/sdui/{Xray,ClientXray,api}.test.*` and `web/src/screens/SelectionScreen.test.tsx`.
- **Review:** 32 findings. 25 patched (all low). 1 pre-existing deferral: the 390 px overflow. 7 rejected with evidence, 3 of them false.
- **Follow-up review recommended:** false. No medium or high finding was patched.
- **Verification:**
  - `pnpm --dir web test`: 189/189, with 100% statements, branches, functions and lines on `src/sdui/**`.
  - `pnpm --dir web build`: ok.
  - `ClientPov.test.tsx`: unchanged.
  - Visual check at 390 px and desktop against `Home-Thiago-RaioX`, `Home-Desktop-RaioX`, `Home-Falha-Isolada`, `Main.dc.html` and `Selecao-SDUI-mobile.dc.html`, using the cached headless Chromium, a live BFF, and a proxy for artboard-shaped envelopes.
- **Recorded deviations from the artboards:**
  1. The Raio-X toggle sits in the first strip row. The artboard's second row ("Dia simulado / +1 dia") is story 13.
  2. The toggle is shown only while an SDUI screen is on view.
  3. The fallback moment reads "welcome", not "welcome (padrão)".
  4. The banner shows the real, URL-encoded customer id.
  5. Placeholder text is generic per reason.
  6. On the phone selection page, the "Demo técnica" chip is dropped and "Arquitetura" is kept. The client tabs stack the name over the segment.
  7. Parts of `Main.dc.html` that the spec and `ux.md` do not ask for are unchanged: view-card illustrations, hero sentence, footer chip, walkthrough node copy.
- **Residual risks:**
  - The rule map is copied from `specs/http/bff.md` and can drift.
  - `fetchPOVClients` still rejects a list with a null item; the selection screen shows the neutral note.
  - The light theme was not screenshotted.
