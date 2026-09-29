---
title: 'Render SDUI screens in web and migrate the home'
type: 'feature'
created: '2026-09-29'
status: 'done'
review_loop_iteration: 0
baseline_revision: '257cf651722d66265a348540bcf39edeeb21adfc'
followup_review_recommended: false
context:
  - '{project-root}/AGENTS.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/ux.md'
  - '{project-root}/specs/http/bff.md'
  - '{project-root}/specs/adr/0009-server-driven-ui-in-the-bff.md'
  - '{project-root}/docs/design/sdui-full-pov/README.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/stories/5-build-the-bff-screen-engine-and-serve-the-home.md'
warnings: []
deferred:
  - summary: >-
      Remove the phase-2 home fallback in web once the purchase story lands.
    evidence: |-
      When the screen request fails or answers a non-envelope body, ClientAppScreen renders the phase-2 home from GET /v1/client-pov/customers/{id} so ClientPov.test.tsx keeps passing unchanged; the shell strip, sidebar, and the deposit/withdraw/message/complaint panels also still read that route.
    location: >-
      web/src/screens/ClientAppScreen.tsx
    severity: low
  - summary: >-
      The eye toggle does not mask money that the BFF writes inside moment copy (title and body).
    evidence: |-
      idle_cash says "US$ 60.520,00 parados…" and "89% do seu patrimônio"; the contract has no way to mark sensitive copy. Story 8 serves idle_cash and can add masked alternates.
    location: >-
      web/src/sdui/components/MomentCard.tsx
    severity: low
  - summary: >-
      Web SDUI fixtures are hand-built; nothing checks them against the Go envelopes.
    evidence: |-
      web/src/test/sduiFixtures.ts mirrors internal/screen output by hand; a golden envelope produced by Go tests and read by web tests would catch contract drift.
    location: >-
      web/src/test/sduiFixtures.ts
    severity: medium
  - summary: >-
      Artboard fidelity is verified by manual screenshots, not by an automated visual-regression suite.
    evidence: |-
      Screens were compared by hand with cached headless Chromium at 390 and 1440 px; light theme not screenshotted.
    location: >-
      web/src/styles/app.css
    severity: low
---

<intent-contract>

## Intent

**Problem:** The BFF now serves the home as an SDUI envelope, but web still hardcodes the home in `ClientAppScreen.tsx` (copy, percentages, currency formatting), so nothing the server composes reaches the client.

**Approach:** Add `web/src/sdui/` — envelope types, a type registry, one error boundary per component, unknown-type reporting, and an action dispatcher — and render the client app home from `GET /v1/client-pov/customers/{id}/screens/home`, pixel-faithful to the adopted artboards (`Home-Fernanda`, `Home-Thiago`, `Home-Mariana`, `Home-Carregando`, `Home-Falha-Isolada`, `OrlaApp.dc.html`), with `ClientPov.test.tsx` passing unchanged before and after, and 100% test coverage of `web/src/sdui/`.

## Boundaries & Constraints

**Always:** pnpm only. Types in `web/src/sdui/types.ts` mirror `specs/http/bff.md` exactly (`schema_version`, `slug`, `revision`, `title`, `subtitle?`, `sections[{id, components[{type, variant, props}]}]`, `omitted[{id, type, reason}]`, actions `navigate|panel|note|link`). The registry maps a `type` to a React component; this story registers the five home types (`moment_card`, `wealth_summary`, `action_grid`, `advisor_card`, `activity_list`) and the registry is the only place a type is added. An unknown `type` renders nothing and calls `console.error` once with the type; a component that throws is caught by its own boundary (renders nothing, `console.error`), and sibling components and sections still render. The action dispatcher: `navigate` → `/client-pov/{id}` for `home` or `/client-pov/{id}/{target}` (routes for other slugs may still show the phase-2 placeholder until their stories); `panel` `deposit|withdraw|message|complaint` → opens the existing coded panels; `panel` `purchase` hidden until story 10; `note` → shows its text in place; `link` → only `https:` with `rel="noopener noreferrer"`; unknown type/target → control hidden + `console.error`. Web computes no percentage, currency, date, validation, or SLA: every visible value on the SDUI home is a BFF prop; `bar_width` drives widths; `tone` maps to color classes (unknown → neutral); `class` maps allocation colors. The eye toggle ("Esconder valores"/"Mostrar valores") masks money props as "US$ ••••••" in web only. Section array order is render order; one column on phone, the drawn two-column grid on desktop (≥ 900 px), spans as in `OrlaApp.dc.html`. Loading state: skeleton of the section shapes with `role="status"` "Montando sua tela…" and the note from `ux.md` "States". Screen heading uses the envelope `title`/`subtitle`. Omitted sections render nothing (Raio-X comes in story 14). Visual fidelity: port the artboard styling (Orla identity, Manrope, gold `#f2b544`, dark default and light theme, 44 px targets, `prefers-reduced-motion`) into the existing `pov-app` stylesheet; component markup and spacing follow the artboard components. `ClientPov.test.tsx` is not edited and passes: when the screen request fails or returns a body that is not a valid envelope (missing `schema_version` or `sections`), the home falls back to the phase-2 home built from `GET /v1/client-pov/customers/{id}`; this fallback is marked for removal in story 10 (comment + deferred note). The panels keep reading cash and messages from the phase-2 endpoint as today. Coverage: add `@vitest/coverage-v8` pinned to the installed vitest version, enable coverage in `vite.config.ts` for `src/sdui/**` with 100% lines, branches, functions, and statements thresholds, and make `pnpm --dir web test` enforce it (`vitest run --coverage`). New tests cover every registered type and each of its variants from the contract, unknown type, throwing component, each action type and unknown action, masking, loading, fallback to phase-2, and the three seed homes (Fernanda, Thiago, Mariana envelopes shaped like the BFF output of story 5).

**Never:** No BFF or Go change. No Investir/Carteira/Perfil screens, purchase form, Raio-X, simulation-strip additions, or `X-SDUI-Schema` (stories 10–16). Do not edit `web/src/screens/ClientPov.test.tsx`. Do not use npm, yarn, or bun. No business formatting in web (no `toLocaleString` on SDUI values, no percentage math).

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Thiago home | valid envelope, 5 sections | heading "Olá, Thiago"; sections in array order; wealth shows "US$ 68.000,00" and shares from props | No error expected |
| Unknown type | a component `type: "hero_banner"` | nothing rendered in its slot; other components render | `console.error` once |
| Throwing component | a registered component throws on bad props | its slot empty; siblings render | boundary logs |
| Omitted section | `omitted: [{id:"activity"…}]`, no activity section | nothing rendered for it | — |
| Action navigate | moment action `navigate investir` | routes to `/client-pov/{id}/investir` | — |
| Action panel | Depositar | existing deposit panel opens | — |
| Unknown action / non-https link | `{type:"teleport"}` / `http:` href | control hidden | `console.error` |
| Loading | request pending | `role="status"` "Montando sua tela…" skeleton | — |
| Fallback | screen 502 or non-envelope 200 | phase-2 home from `/customers/{id}` | — |
| Masking | eye toggle on | money props show "US$ ••••••" | — |

</intent-contract>

## Code Map

- `web/src/screens/ClientAppScreen.tsx` (1,042 lines) -- phase-2 client app: panels (`MoneyPanel`, complaint, message), `HomePhone`/`HomeDesktop` (`:796`, `:867`), `Wealth`/`Actions`/`Activity` (`:955-1042`), theme storage, `useWide` (`:94`). Keep panels and shell; replace the home body with the SDUI renderer; keep phase-2 home components only for the fallback.
- `web/src/api/pov.ts` -- phase-2 fetchers; add `fetchScreen(customerID, slug)` (or put it under `web/src/sdui/api.ts`).
- `web/src/api/base.ts` -- `apiPath`.
- `web/src/screens/ClientPov.test.tsx` -- read-only; mocks `fetch` returning phase-2 JSON for every GET, and asserts "Olá, Fernanda", "Nenhuma movimentação nos últimos 30 dias.", deposit/withdraw/complaint panels, desktop theme and message, 404 error state, and "Investir não entra nesta simulação".
- `web/src/styles/app.css`, `web/src/styles/tokens.css` -- existing `pov-app` styles and Orla tokens.
- `web/vite.config.ts` -- vitest config (jsdom, `src/**/*.test.{ts,tsx}`); add coverage.
- `docs/design/sdui-full-pov/project/OrlaApp.dc.html` -- the single interactive client app; `homeSections` (`:749-802`), its render template, palette, icons, and section chips; `Home-*.dc.html` import it with fixed props.
- `internal/screen/catalog.json`, `internal/bff/screen_test.go` (story 5) -- real envelope shapes and props to build test fixtures from.

## Tasks & Acceptance

**Execution:**
- `web/src/sdui/types.ts`, `web/src/sdui/api.ts` -- envelope types and screen fetch with envelope validation.
- `web/src/sdui/registry.tsx`, `web/src/sdui/Boundary.tsx`, `web/src/sdui/SduiScreen.tsx` -- registry, per-component boundary, unknown-type reporting, section rendering, loading skeleton.
- `web/src/sdui/actions.ts(x)` -- dispatcher for the four action types.
- `web/src/sdui/components/*.tsx` -- `MomentCard`, `WealthSummary`, `ActionGrid`, `AdvisorCard`, `ActivityList` matching the artboards.
- `web/src/screens/ClientAppScreen.tsx` -- home body from `SduiScreen`, phase-2 fallback, masking, panels wired to `panel` actions.
- `web/src/styles/app.css` -- SDUI component styles ported from the artboards, both themes, phone and desktop.
- `web/src/sdui/*.test.tsx` -- tests listed in Always; 100% coverage of `src/sdui/**`.
- `web/package.json`, `web/pnpm-lock.yaml`, `web/vite.config.ts` -- coverage dependency, thresholds, `test` script with `--coverage`.

**Acceptance Criteria:**
- Given the BFF from story 5 and account-sim running locally, when a visitor opens `/advisor-radar/client-pov/{Thiago}`, then one `GET …/screens/home` renders the home and it visually matches `Home-Thiago.dc.html` at 390 px (manual screenshot comparison).
- Given the change, when `pnpm --dir web test` runs, then all suites pass, including unchanged `ClientPov.test.tsx`, and coverage of `src/sdui/**` is 100% on all four metrics.

## Design Notes

Visual check (manual, recommended): screenshot the running app and the artboard with the cached headless Chromium (`~/.cache/ms-playwright/chromium_headless_shell-*/`), e.g. via `pnpm dlx playwright@<version matching the cache> screenshot --viewport-size=390,844 <url> out.png` into the scratchpad, then compare the PNGs side by side. Artboards open as local files.

## Verification

**Commands:**
- `pnpm --dir web install --frozen-lockfile` -- expected: success after the lockfile update.
- `pnpm --dir web test` -- expected: all pass; coverage thresholds met for `src/sdui/**`.
- `pnpm --dir web build` -- expected: success (tsc + vite).
- `git diff --stat HEAD -- web/src/screens/ClientPov.test.tsx` -- expected: empty.

## Review Triage Log

### 2026-09-29 — Review pass
- verdicts: 32 findings — high 0, medium 2, low 26, false 4, maybe-false 0
- findings:
  - `[low]` `[patch]` edge: envelope slug inherited key (`constructor`) makes `spanOf` throw outside any Boundary — envelope slug must equal the requested slug, `spanOf` uses `Object.hasOwn`.
  - `[low]` `[patch]` edge: a different valid slug renders home with another screen's spans — same fix (slug mismatch → `InvalidScreenError`).
  - `[low]` `[reject]` edge: returning from a panel re-fetches and a later failure swaps SDUI home to fallback — re-fetch is intended (reflects the action); the swap needs a failed BFF mid-session; the misleading comment is patched instead.
  - `[medium]` `[patch]` edge: a hung screen request keeps the skeleton forever — `fetchScreen` takes an AbortSignal with a 3 s client timeout plus cleanup abort; timeout falls back.
  - `[low]` `[patch]` edge: raw id in `Navigate`/`pickNav` — paths built with `encodeURIComponent`.
  - `[low]` `[patch]` edge: SDUI navigate on desktop leaves an open coded panel open — SDUI navigate goes through the close-panel path.
  - `[low]` `[reject]` edge: a null item makes the Boundary drop the whole grid — that is the designed failure isolation; the BFF never sends null items.
  - `[low]` `[patch]` blind: unknown slug crash — grouped with the slug fix.
  - `[low]` `[reject]` blind: web accepts any numeric `schema_version` — version negotiation (`X-SDUI-Schema`, 406) is story 16 by intent.
  - `[false]` `[reject]` blind: loading note is developer text — the intent requires the `ux.md` "States" note verbatim and `Home-Carregando` draws it.
  - `[low]` `[defer]` blind: masking leaves money inside moment `title`/`body` copy — needs a contract marker for sensitive copy; deferred.
  - `[low]` `[patch]` blind: inconsistent id encoding — grouped with the encoding fix.
  - `[low]` `[patch]` blind: `/client-pov/:id/home` is a second URL for home — redirects to the canonical path.
  - `[false]` `[reject]` blind: `/investir` renders the home with a note — the intent allows the placeholder until the Investir story.
  - `[low]` `[patch]` blind: malformed components kept with empty type, omitted rows get "undefined" — dropped in `api.ts`.
  - `[low]` `[reject]` blind: props are cast, not validated per type — would add per-type guards for BFF-owned data; Boundary already isolates throws.
  - `[low]` `[patch]` blind: empty sections leave blank grid cells — `.sdui-section:empty { display: none }`.
  - `[low]` `[reject]` blind: desktop loading layout shift — cosmetic, fix is a new desktop skeleton layout.
  - `[low]` `[patch]` blind: test gaps (desktop, invalid JSON body) — grouped with the verification-gap patches; screen-ok-but-phase-2-fails and unmount-in-flight rejected as unlikely.
  - `[low]` `[reject]` blind: coverage scope excludes `ClientAppScreen.tsx` — the intent scopes the 100% gate to `src/sdui/**`; the host is covered by `ClientHome.test.tsx` app-level cases.
  - `[medium]` `[defer]` blind: hand-built fixtures have no drift check against the Go envelopes — a shared golden envelope needs Go and web wiring; deferred.
  - `[low]` `[patch]` blind: `PhoneBar` picks its icon by comparing UI text — passes the `light` boolean.
  - `[low]` `[reject]` blind: `ActionGrid` ignores `action.label` — the contract's item label is the visible text; divergence is unlikely.
  - `[low]` `[reject]` blind: web routes and timeout undocumented in `bff.md` — web routes are not the BFF contract.
  - `[low]` `[defer]` intent: pixel fidelity is not checked automatically — manual screenshot comparison was done at 390 px and 1440 px; an automated visual-regression suite is deferred.
  - `[false]` `[reject]` intent: `ClientPov.test.tsx` exercises only the fallback — that is the reading the intent pairs with the no-edit rule; the SDUI path is covered by `ClientHome.test.tsx`.
  - `[false]` `[reject]` intent: `specs/http/bff.md` edited despite "No BFF change" — the edit is a documentation note about the web fallback, not BFF code.
  - `[low]` `[patch]` verification-gap: no test re-fetches the screen after returning from a panel — added.
  - `[low]` `[patch]` verification-gap: no desktop SDUI home test — added.
  - `[low]` `[patch]` verification-gap other: comment wrongly claims the home never switches — corrected.
  - `[low]` `[patch]` verification-gap other: a non-JSON 200 throws `SyntaxError` — mapped to `InvalidScreenError` with a test.
  - `[low]` `[reject]` intent: variants covered by props, not variant-specific markup — the registry renders by type; variants differ by props from the BFF.

## Auto Run Result

Status: done

- **Summary:** web renders the client home from `GET /v1/client-pov/customers/{id}/screens/home` through `web/src/sdui/`: envelope types and a validating fetch (slug check, non-JSON and malformed records rejected, 3 s client timeout plus abort), a registry of the five home types, one Boundary per component, unknown-type reporting, section spans from the artboards, the loading skeleton, and an action dispatcher (`navigate` through the close-panel path, `panel`, `note`, https-only `link`). The artboard styling is ported into `pov-app` for both themes, phone and desktop. The phase-2 home stays as a fallback marked for removal in story 10. The test gate enforces 100% coverage of `src/sdui/**`.
- **Files:**
  - `web/src/sdui/{types,api,registry,Boundary,SduiScreen,actions,context,style,icons}` and `web/src/sdui/components/*`: the renderer.
  - `web/src/screens/ClientAppScreen.tsx`: SDUI home, fallback, tab routes, encoded paths.
  - `web/src/App.tsx`: the `:tab?` route.
  - `web/src/styles/app.css`: ported artboard styles.
  - `web/src/sdui/*.test.*`, `web/src/test/{sdui.tsx,sduiFixtures.ts}`: tests and fixtures.
  - `web/package.json`, `web/pnpm-lock.yaml`, `web/vite.config.ts`: coverage dependency and thresholds.
  - `specs/http/bff.md`: the note on the web fallback.
- **Review:** 32 findings. 18 patched in 9 fixes (1 medium, 17 low). 3 deferred: masking of money inside moment copy, a golden-fixture drift check, and automated visual regression. The other 11 were rejected with evidence (4 false).
- **Follow-up review recommended:** false. Only one medium was patched (client timeout).
- **Verification:**
  - `pnpm --dir web install --frozen-lockfile`: ok.
  - `pnpm --dir web test`: 143/143, with 100% statements, branches, functions and lines on `src/sdui/**`.
  - `pnpm --dir web build`: ok.
  - `ClientPov.test.tsx`: unchanged.
  - Manual visual check: Thiago, Fernanda and Mariana at 390 px, plus loading and desktop at 1440 px, compared with headless Chromium against the artboards.
- **Residual risks:**
  - The shell still needs the phase-2 route, so the error page shows if it fails even when the screen request succeeded.
  - The light theme was not screenshotted.
