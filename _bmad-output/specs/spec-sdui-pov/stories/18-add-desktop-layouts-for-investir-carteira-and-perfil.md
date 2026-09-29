---
title: 'Add desktop layouts for Investir, Carteira, and Perfil'
type: 'feature'
created: '2026-09-29'
status: 'done'
review_loop_iteration: 0
baseline_revision: '2193a27'
followup_review_recommended: true
context:
  - '{project-root}/AGENTS.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/ux.md'
  - '{project-root}/docs/design/sdui-full-pov/README.md'
warnings: []
deferred: []
---

<intent-contract>

## Intent

**Problem:** Investir, Carteira, and Perfil were built phone-first. On desktop (≥ 900 px) they must match `Investir-Desktop`, `Carteira-Desktop`, and `Perfil-Desktop`: the two-column grid with the drawn spans, the two-column highlights rail, and the three-column suitability levels.

**Approach:** A web-only layout change. Lay out the same server sections in the desktop grid, with per-screen section spans taken from `OrlaApp.dc.html` (`investSections`, `walletSections`, `profileSections` `span` values), and adapt the component internals that change on desktop. There is no contract or BFF change.

## Boundaries & Constraints

**Always:**
- **Spans.** Section spans live in the existing web span table keyed by slug and section id (story 6 `WIDE_SECTIONS`):
  - Investir: `cash` and `highlights` span 2; the lists span 1.
  - Carteira: every section spans 1, laid out as `walletSections` draws them.
  - Perfil: `header`, `suitability`, and `advisor` span 2; `registration` and `preferences` span 1.
- **Component internals on desktop.**
  - The `product_rail` grid has 2 columns.
  - `profile_scale` levels use 3 columns.
  - Position lists and allocation follow the artboard spacing.
- **Where desktop comes from.** Desktop behaviour keys off the existing `data-layout="desktop"` / `useWide` mechanism, not a new breakpoint.
- **Everything else unchanged.**
  - The purchase form on desktop opens in the existing side-panel aside, as the other coded panels do.
  - Raio-X outlines keep working in the desktop grid.
  - The phone layout does not change.
- **Tests.** Add desktop tests for each of the three screens asserting `data-span` values and the rail and levels column classes or attributes. Keep 100% coverage of `src/sdui/**`.
- **Visual check.** Compare at 1440 px against the three desktop artboards and at 390 px for regressions, using the cached headless Chromium, and record the result.
- Fix the pre-existing 390 px overflow on the selection page (the walkthrough "Pausar"/"Reproduzir" control makes the page 438 px wide; story 14 deferral) so no page scrolls horizontally at 390 px.
- pnpm only.

**Never:**
- No BFF, Go, or contract change.
- No new component type.
- Web computes no values.
- No npm, yarn, or bun.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Investir desktop | ≥ 900 px | cash + rail full width, rail 2 columns, three lists in the grid | — |
| Carteira desktop | ≥ 900 px | summary/allocation side by side, positions and history in the grid as drawn | — |
| Perfil desktop | ≥ 900 px | header and suitability full width, levels 3 columns, registration and preferences side by side | — |
| Phone | 390 px | unchanged from stories 10–12 | — |
| Omitted section | desktop | no hole left behind (empty sections hidden) | — |

</intent-contract>

## Code Map

- `web/src/sdui/SduiScreen.tsx` -- span table.
- `web/src/sdui/components/*` -- rail, scale, lists.
- `web/src/styles/app.css` -- desktop grid rules.
- `docs/design/sdui-full-pov/project/{Investir-Desktop,Carteira-Desktop,Perfil-Desktop,OrlaApp}.dc.html`.

## Tasks & Acceptance

**Execution:**
- Span table entries, desktop CSS, component desktop variants, and tests.

**Acceptance Criteria:**
- Given a desktop viewport, when each of the three screens renders, then its layout matches the desktop artboard, with no change to the BFF response.

## Verification

**Commands:**
- `pnpm --dir web test && pnpm --dir web build` -- expected: pass; coverage thresholds met.
- `git diff --stat <baseline> -- internal cmd proto specs` -- expected: empty.

## Review Triage Log

### 2026-09-29 — Review pass
- verdicts: 20 findings — high 0, medium 2, low 17, false 1, maybe-false 0
- findings:
  - `[low]` `reject` Crossing 900 px with the form open remounts it and loses the typed amount — requires resizing mid-purchase; the fix lifts form state into the app shell.
  - `[low]` `patch` The desktop purchase drawer lacks the other panels' close control — panel header with close added in the drawer.
  - `[false]` `reject` Position-list and allocation spacing not changed — measured against Carteira-Desktop: row pitch within 1 px, and a 12 px gap moved it further off; no change needed.
  - `[low]` `reject` Visual check not recorded — recorded under Auto Run Result.
  - `[low]` `patch` The "no hole" test removes only the last section — middle-section case and Carteira phone case added.
  - `[low]` `reject` Desktop span assertions are not desktop-specific — spans are layout-independent by design; column attributes are the desktop evidence.
  - `[low]` `patch` The Mariana Carteira desktop test uses Thiago's phase-2 data — Mariana payload used.
  - `[low]` `reject` The matchMedia stub is duplicated across tests — test-only duplication.
  - `[low]` `patch` The selection overflow fix targets the heading by DOM position — class added.
  - `[low]` `patch` The drawer `.pov-buy` rule is unscoped despite its comment — scoped to desktop.
  - `[low]` `reject` (dup of 1) Breakpoint remount — same reason.
  - `[medium]` `patch` Focus reaches the products behind the desktop drawer, and a second form opens over an in-flight purchase — main made inert while the drawer is open.
  - `[medium]` `patch` openPurchase while a form is open does not bump the form counter, so a late answer lands on the new form — counter bumped.
  - `[low]` `patch` Raio-X toggle shows during the desktop purchase — hidden.
  - `[low]` `patch` Phone purchase has no test guarding against an empty "Ação" drawer — test added.
  - `[low]` `reject` (dup of 1) Verification-gap other finding: resize remount — same reason.
  - `[low]` `reject` Stray web/tsconfig.tsbuildinfo appeared during a test run — removed; not part of the change.
  - `[low]` `reject` Intent: the tests assert DOM attributes rather than rendered CSS — jsdom applies no CSS; the visual check covers rendering.
  - `[low]` `reject` Intent: moving the desktop purchase form into the aside reverses story 10's placement — the spec states the form opens in the side-panel aside on desktop.
  - `[low]` `reject` Intent: a React context flag duplicates data-layout — it carries the same useWide value, so the columns can be tested.

## Auto Run Result

**Summary:** A web-only change. Investir, Carteira and Perfil use the desktop grid with the `OrlaApp.dc.html` spans. The SDUI context carries `wide` from the existing `useWide`, so:
- `product_rail` renders 1 column on phone and 2 on desktop;
- `profile_scale` levels render 1 column on phone and 3 on desktop.

On desktop the purchase form and "Compra enviada" open in the side-panel aside:
- the aside has the shared panel header and a "Fechar" button;
- the screen behind it is inert;
- the Raio-X toggle is hidden during a purchase.

The 390 px overflow on the selection page (the story 14 deferral) is fixed with a wrapping walkthrough header.

**Files:**
- `web/src/screens/{ClientAppScreen,PurchasePanel,SelectionScreen}.tsx`
- `web/src/sdui/{context.ts,components/ProductRail.tsx,components/ProfileScale.tsx}`
- `web/src/selection/SduiShowcase.tsx`
- `web/src/styles/app.css`
- tests (`ClientDesktop`, `ClientInvestir`, `investir`, `perfil`), fixtures

**Review:** 20 findings. 9 patched (2 medium, 7 low). 0 deferred. 11 rejected, with the reasons in the triage log. Position and allocation spacing were measured within 1 px of Carteira-Desktop, so no change was needed.

**Followup review recommended:** yes. Two medium entries were patched: the inert screen behind the drawer, and the form counter on reopen.

**Verification:** `pnpm --dir web test` passes 323/323 with 100% coverage of `src/sdui/**`, and `pnpm --dir web build` passes. There is no Go, BFF or contract change.

**Visual check:** headless Chromium against services at 2193a27, read-only. Shots are in the session scratchpad under `s18/`.
- **1440 px:** Investir (Thiago), Carteira (Mariana) and Perfil (Fernanda) match the desktop artboards: the same spans, the rail at 2 × 554 px, and levels at 3 × 355 px.
- **Desktop purchase drawer:** the "Fechar" button measures 44×44. Over 14 Tab presses focus never reached the screen behind the drawer.
- **390 px:** the three screens and the purchase form are unchanged. The selection page, client list, home, Investir, Carteira and Perfil all measure `scrollWidth` 390.

**Residual risks:**
- Resizing across 900 px with the form open resets the typed amount.
- The Investir product rows are about 5 px tighter than the artboard, the same on phone and desktop.
