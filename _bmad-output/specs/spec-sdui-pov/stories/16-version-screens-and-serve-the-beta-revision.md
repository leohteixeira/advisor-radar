---
title: 'Version screens and serve the beta revision'
type: 'feature'
created: '2026-09-29'
status: 'done'
review_loop_iteration: 0
baseline_revision: '8385498'
followup_review_recommended: false
context:
  - '{project-root}/AGENTS.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/architecture.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/ux.md'
  - '{project-root}/specs/http/bff.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/stories/12-build-the-perfil-screen-with-preferences-and-beta.md'
warnings: []
deferred: []
---

<intent-contract>

## Intent

**Problem:**
- The BFF ignores `X-SDUI-Schema`, so a client that cannot render the current `schema_version` gets a screen it will misdraw.
- The beta flag stored in story 12 changes nothing: every client gets home `v1`.

**Approach:**
- Implement `X-SDUI-Schema` exactly as `specs/http/bff.md` already documents it: `400` for a malformed header, `406 {"supported":{"min":1,"max":1}}` when the range excludes the served version.
- Web sends the header and shows the update notice on `406`.
- The BFF serves home revision `v2` to beta clients, inserting the `highlights` section (`product_rail`, profile variant, same builder as Investir) right after `moment`. Everyone else keeps `v1`, and a failed preferences read falls back to `v1`.
- Every screen carries `slug` and `revision`, and tests pin that.

## Boundaries & Constraints

**Always:**
- **Skills.** Load `golang-how-to` first and apply the Go skills in `/workspace/repos/advisor-radar/CLAUDE.md`.
- **Header parsing.** Parse the header in `internal/screen` or `internal/bff` as a small pure function with a table test:
  - accepted forms are `N` and `N-M`;
  - integers must satisfy `1 ≤ min ≤ max`;
  - whitespace is trimmed;
  - anything else is `400 {"error":"invalid_schema_range"}`.
- **Header handling.**
  - With no header, serve the current version, which is 1.
  - When the range excludes the current version, answer `406` with `{"supported":{"min":1,"max":1}}` and `Cache-Control: no-store`.
  - Header checks run before any Snapshot fetch.
- **Catalog revisions.** The catalog holds revisions per screen: home `v1` and `v2`; investir, carteira and perfil `v1`.
  - Revision choice is a function of the Snapshot's preferences: `beta` true → home `v2`, otherwise `v1`. A preferences error → `v1`.
  - The preferences source from story 12 joins the home Snapshot.
  - `v2` home is `moment`, `highlights`, `wealth`, `actions`, `advisor`, `activity`.
  - `highlights` reuses the Investir rail builder, and its failure policy is the same as on Investir: profile or catalog failure omits it.
- **Contract docs.** Update `specs/http/bff.md`:
  - remove "not implemented yet";
  - describe the `v2` home;
  - note that the revision is chosen per request.
- **Web.**
  - Send `X-SDUI-Schema: 1` on every screen request (constant `SUPPORTED_SCHEMA`).
  - On `406`, the screen area shows "Atualize o app para ver esta tela." with a reload button that reloads the page. The simulation strip and tabs stay.
  - Web also rejects a `200` envelope whose `schema_version` is outside its supported range, and treats it the same way (update notice). This closes story 6's unchecked `schema_version`.
  - The `v2` home renders `product_rail` through the registry. No new type.
  - Tests cover the header, the `406` notice, the reload button, an out-of-range `200`, and the `v2` envelope, keeping 100% coverage of `src/sdui/**`.
- **Go tests.**
  - The header parsing table.
  - `400`, `406`, and no-header responses through the HTTP handler.
  - `v1` versus `v2` per seed client with the beta flag.
  - The preferences-failure fallback to `v1`.
  - `slug` and `revision` present on every screen.
- **Live check.** With the local stack, switch Fernanda's beta flag on in Perfil. Her home should show "revision v2" in Raio-X and the highlights rail after the moment.

**Never:**
- No schema version 2 envelope.
- Do not serve a lower version than the current one.
- Carteira, Investir and Perfil stay `v1`.
- Web does not decide the revision.
- Unit tests do not dial 5435/5673/8400/8420/9201.
- Do not read or edit `.env` / `.env.*`.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| No header | — | `200` schema 1 | — |
| `X-SDUI-Schema: 1-2` | includes 1 | `200` | — |
| `X-SDUI-Schema: 2` | excludes 1 | `406 {"supported":{"min":1,"max":1}}` | web update notice |
| `X-SDUI-Schema: 2-1` / `abc` / `0` | invalid | `400` | — |
| Beta Fernanda | beta true | home `revision v2`, sections moment, highlights(profile_conservador), wealth, actions, advisor, activity | — |
| Beta, preferences down | GetPreferences fails | `v1` | not propagated |
| Non-beta | beta false | `v1` | — |
| Investir for beta client | — | `v1` | — |

</intent-contract>

## Code Map

- `internal/screen/{engine,catalog,snapshot}.go`, `catalog.json` -- revisions, preferences source, rail reuse.
- `internal/bff/screen.go` -- header handling, status codes.
- `specs/http/bff.md` -- `X-SDUI-Schema` paragraph, revisions.
- `web/src/sdui/{api,SduiScreen}.ts(x)`, `web/src/screens/ClientAppScreen.tsx` -- header, notice.

## Tasks & Acceptance

**Execution:**
- BFF header parsing and handling, revision selection, the `v2` home, and tests.
- Web header, the update notice, the schema-range check, and tests.
- Contract docs.

**Acceptance Criteria:**
- Given the local stack, when a request carries `X-SDUI-Schema: 2`, then the BFF answers `406` with the supported range, and web shows the update notice with the strip and tabs intact.
- Given Fernanda in beta, when her home is requested, then it is revision `v2` with `highlights` after `moment`.

## Verification

**Commands:**
- `gofmt -l . && go vet ./... && go build ./...` -- expected: clean.
- `mkdir -p .gotmp && GOTMPDIR=$PWD/.gotmp go test -race -shuffle=on ./...` -- expected: pass.
- `go mod tidy && git diff --exit-code go.mod go.sum` -- expected: clean.
- `pnpm --dir web test && pnpm --dir web build` -- expected: pass; coverage thresholds met.

## Spec Change Log

- 2026-09-29 — Orchestrator merge: home `v2` mirrors story 13's home `v1` variants (`portfolio_drop` first in `moment`, `with_day_change` in `wealth`), so `v2` stays `v1` plus `highlights`.

## Review Triage Log

### 2026-09-29 — Review pass
- verdicts: 19 findings — high 0, medium 1, low 17, false 1, maybe-false 0
- findings:
  - `[low]` `patch` Header checked before slug: an unknown slug answers 406/400 instead of 404 — precedence documented and pinned by a test.
  - `[low]` `patch` The showcase reports the BFF as down on UnsupportedSchemaError — distinct unsupported state.
  - `[low]` `patch` The default-variant invariant exemption is broadened to any highlights section — scoped.
  - `[low]` `patch` Early-ending spans lose sdui.revision — default revision set at span start.
  - `[low]` `reject` Intent: the header grammar accepts `01` and rejects `1 - 2` — both fit "digits only, whitespace trimmed" and are documented.
  - `[low]` `reject` Intent: web treats a non-numeric schema_version as invalid, not unsupported — a non-numeric version is not an envelope.
  - `[false]` `reject` Intent: the live check is not shown — it was performed (see Auto Run Result).
  - `[medium]` `patch` Every home request requires GetPreferences, so an outage records failures on 100% of homes — preferences made an optional source.
  - `[low]` `patch` (dup of 4) Revision attribute on early spans — same fix.
  - `[low]` `reject` Metrics have no revision label — the label set is fixed by architecture.md; the served log line carries the revision.
  - `[low]` `reject` 400/406 refusals are not counted — the metric set is fixed by story 17's contract.
  - `[low]` `patch` (dup of 1) Header versus slug precedence — same fix.
  - `[low]` `patch` (dup of 2) Showcase unsupported state — same fix.
  - `[low]` `patch` (dup of 3) Invariant exemption — same fix.
  - `[low]` `patch` The "invalid beta revision" case needs no beta logic — tied to beta_revision.
  - `[low]` `patch` Preferences-down test misses a non-beta client — added.
  - `[low]` `patch` Stale and ragged doc comments — fixed.
  - `[low]` `patch` v2 app-level coverage is thin and one test name overstates it — renamed; ClientApp v2 Raio-X test added.
  - `[low]` `reject` v2 duplicates v1 in catalog.json — TestCatalog_HomeBetaRevision pins v2 = v1 + highlights, so any drift fails the build.

## Auto Run Result

**Summary:** The BFF implements `X-SDUI-Schema` as documented. It accepts `N` or `N-M` (digits only, trimmed, `1 ≤ min ≤ max`).
- A malformed or repeated header gets `400 {"error":"invalid_schema_range"}`.
- A range that excludes the served version gets `406 {"supported":{"min":1,"max":1}}`.
- Both answers carry `Cache-Control: no-store`.
- The checks run in this order: id, then header, then slug, all before any source is read.

The catalog gains home `v2` and `beta_revision`. The engine picks the revision per request from the stored beta flag:
- a beta client gets `v2`, which is `v1` with `highlights` (`product_rail`, the same builder as Investir) right after `moment`;
- everyone else gets `v1`;
- a failed preferences read falls back to `v1` silently; preferences are an optional source, and the error appears only on its span.

Every screen carries `slug` and `revision`, and the span always carries `sdui.revision`. Web sends `X-SDUI-Schema: 1`. On a `406`, or a `200` with another `schema_version`, it shows "Atualize o app para ver esta tela." with "Recarregar"; the selection showcase shows the same note.

**Files:**
- `internal/bff/{schema,screen}.go`
- `internal/screen/{revision,engine,catalog}.go`, `catalog.json` + tests
- `web/src/sdui/{api,SduiUpdate}`, `web/src/screens/ClientAppScreen.tsx`, `web/src/selection/SduiShowcase.tsx`
- tests, fixtures
- `specs/http/bff.md`

**Review:** 19 findings. 12 patched (1 medium, 11 low), 0 deferred, 7 rejected, with the reasons in the triage log.

**Followup review recommended:** no. Only one medium entry was patched, and no high.

**Verification:**
- gofmt, vet and build are clean.
- `go test -race -shuffle=on ./...` passes.
- `go mod tidy` is clean.
- web 357/357 passes with 100% coverage of `src/sdui/**`, and `pnpm --dir web build` passes.

**Live check:** before the review patches, with services on throwaway databases and headless Chromium. Screenshots are in the session scratchpad under `s16/`.
- `curl` answers:
  - `200` with no header and with `1-2`;
  - `406` with the documented body for `2`;
  - `400` for `2-1`, `abc` and `0`.
- Every app screen request carried `X-SDUI-Schema: 1`.
- After Fernanda switched beta on in Perfil, her home Raio-X read "slug home · revision v2 · schema 1 · 6 seções", with `highlights · product_rail · profile_conservador` after the moment.
- A forced `406` showed the update notice, with the strip and tabs intact.

**Residual risks:**
- The home `v2` rail's purchase form has no cash line or cap, because the home carries no `invest_summary`. The BFF still refuses above cash with `422`.
- `v2` duplicates `v1` in the catalog; drift is guarded by a test.
