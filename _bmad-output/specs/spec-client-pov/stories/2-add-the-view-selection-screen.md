---
title: 'Add the view-selection screen'
type: 'feature'
created: '2026-09-28'
status: 'done'
review_loop_iteration: 0
baseline_revision: '2a13c9edfe3848e405eeb8c8adbd726b663f0c7c'
followup_review_recommended: false
context:
  - '{project-root}/_bmad-output/specs/spec-client-pov/ux.md'
warnings: []
deferred: []
---

<intent-contract>

## Intent

**Problem:** `/advisor-radar` redirected straight to the queue, so a visitor never saw the two views, the three pillars, or how one complaint crosses the system.

**Approach:** Render the selection screen at the index route. The team card opens `/advisor-radar/fila`. The client card points at `/advisor-radar/client-pov`. The screen states the three pillars and plays the eight-step walkthrough, which pauses and stays still under `prefers-reduced-motion`.

## Boundaries & Constraints

**Always:** Portuguese copy from `ux.md` and the adopted artboards. Desktop at 900px and above uses the desktop sentences, chips, graph, and step jumpers. Below that, the mobile sentences and the ordered list. Step tags follow `ux.md`, including `POST /v1/client-pov/customers/{id}/complaints`. Auto-advance is 2.6 seconds. The pause control is at least 44px. Selection stays on the light theme and outside the team shell.

**Never:** The client list, the client app, BFF routes, or a change to how fila, revisão, painel, and the 360 view behave.

</intent-contract>

## Auto Run Result

Status: done

Summary: `/advisor-radar` renders the selection screen. The team card opens the existing queue. The walkthrough pauses, jumps by step on desktop, and does not advance when reduced motion is requested.

Files changed:
- `web/src/screens/SelectionScreen.tsx` — selection screen
- `web/src/selection/steps.ts` — eight steps
- `web/src/App.tsx` — index route
- `web/src/styles/app.css` — selection layout
- `web/src/main.tsx` — tokens path after the design files moved
- `web/src/screens/SelectionScreen.test.tsx` — mobile, desktop, pause, reduced motion, and the queue link

Verification: `pnpm test` passed (36 tests). `tsc --noEmit` passed. In the browser, desktop showed both cards, the pillars, and the walkthrough; pause held step 3; a 390px viewport showed the mobile copy; reduced motion started on Reproduzir; the team card landed on `/advisor-radar/fila` with the persona navigation.

Residual risk: `/advisor-radar/client-pov` is linked and has no screen yet. That list is story 6.
