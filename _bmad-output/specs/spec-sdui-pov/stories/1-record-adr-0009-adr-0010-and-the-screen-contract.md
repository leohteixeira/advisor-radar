---
title: 'Record ADR 0009, ADR 0010, and the screen contract'
type: 'chore'
created: '2026-09-28'
status: 'done'
review_loop_iteration: 0
baseline_revision: '87375217457e630891a2ef522b7481ddf25393ea'
followup_review_recommended: true
context:
  - '{project-root}/AGENTS.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/SPEC.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/architecture.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/ux.md'
warnings: []
deferred:
  - summary: >-
      Purchase amounts below a product's minimum have no documented refusal.
    evidence: |-
      bff.md documents only the over-cash 422; the purchase route arrives with stories 9 and 10, which must add the below-minimum answer.
    location: >-
      specs/http/bff.md
    severity: low
---

<intent-contract>

## Intent

**Problem:** Phase 3 moves the client app to Server-Driven UI and replaces the four class balances with per-product positions, but neither decision is recorded, and `specs/http/bff.md` has no screen contract that the BFF engine and the web renderer can both build against.

**Approach:** Write ADR 0009 (SDUI composed in the BFF from an embedded, read-only, versioned catalog; web renders by `type`) and ADR 0010 (individual per-product positions in account-sim, revoking phase-2 closed decision 9), and add a "Screens" section to `specs/http/bff.md` with the envelope, the `X-SDUI-Schema`/`406` rule, the four action types, and the 15 component types with their sections, variants, and props.

## Boundaries & Constraints

**Always:** English only. Follow the house ADR format of `specs/adr/0008-client-command-grpc-outbox.md` (title line, Status accepted, Date 2026-09-28, Context, Decision, Consequences, Reassessment trigger). ADR 0009 states: the BFF composes screens and stores nothing; the catalog is a versioned file embedded with `go:embed`, read-only config with no CMS and no runtime writes, and the `add-sdui-variant` skill is its editing tool; advisory owns moments and segmentation, the BFF only picks the variant for the moment it receives; components are semantic domain types (no `row`/`column`/`text` primitives); server controls section order and point props (tone, emphasis), web owns style and breakpoints; every section has a default variant, first match wins; failure policy (fallback to default, omit when only-source failed, `Build` error drops the component, response stays `200`); a new `type` needs web code and a contract entry, a new variant does not; values arrive display-formatted, forms also get integer cents. ADR 0010 states: account-sim stores positions per fictional product; the four phase-2 classes become an aggregate of positions plus cash; patrimony is positions at market value plus cash everywhere (app, book, rules, moments); the seed keeps every phase-2 class total so phase-1 rules fire at the same values; a position's value is a pure function of product and global simulated day; `aplicacao` and `reavaliacao` ride on `account.event.recorded` at `schema_version` 3 and consumers accept 1, 2, and 3; it explicitly revokes phase-2 closed decision 9 ("four classes, no individual asset"). The bff.md section documents `GET /v1/client-pov/customers/{id}/screens/{slug}` with `slug ∈ home|investir|carteira|perfil`, the envelope `{schema_version, slug, revision, sections:[{id, components:[{type, variant, props}]}]}`, section array order = render order, the header rule, the four action shapes (`navigate` target slug, `panel` one of deposit/withdraw/message/complaint/purchase, `note`, `link`), and a table row per type listing section ids, variants, and props — types, sections, and variants exactly as in `ux.md` "Component catalog (15 types)".

**Never:** No code, proto, migration, or web change. Do not add ADR 0008's "code now follows it" note (story 3 owns it). Do not document routes that later stories add (purchases, preferences, advance-day) as existing. Do not rewrite the existing team and POV route tables.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Current schema | GET screen, no `X-SDUI-Schema` | `200` envelope at the current `schema_version` | No error expected |
| Excluded schema | `X-SDUI-Schema` range excludes server version | `406` with the supported range in the body | web shows "Atualize o app para ver esta tela." |
| Unknown slug or customer | slug outside the four, or unknown id | `404` | documented in the contract |

</intent-contract>

## Code Map

- `specs/adr/0008-client-command-grpc-outbox.md` -- format template; read-only in this story.
- `specs/adr/0001…0007` -- numbering; the new files are `0009-server-driven-ui-in-the-bff.md` and `0010-individual-positions.md`.
- `specs/http/bff.md` -- 41 lines; team and POV route tables. Its intro line "OpenTelemetry is not instrumented" stays (CAP-9 changes it later). Append a `## Screens (phase 3)` section after the POV routes.
- `_bmad-output/specs/spec-sdui-pov/architecture.md` -- "Screen contract", "Composition engine", "Home moment priority", "Events", "Seed" sections are the source of truth.
- `_bmad-output/specs/spec-sdui-pov/ux.md` -- "Component catalog (15 types)", "Home moments", per-screen sections: source for sections, variants, and prop names.
- `docs/design/sdui-full-pov/project/OrlaApp.dc.html` -- the design's section data (`id`, `type`, `variant`, fields) to derive prop names from.
- `/workspace/docs/advisor-radar/advisor_radar_client_pov_brief.md` §15 item 9 -- the phase-2 closed decision 9 text being revoked.

## Tasks & Acceptance

**Execution:**
- `specs/adr/0009-server-driven-ui-in-the-bff.md` -- create ADR 0009 with the content listed in Always -- records the SDUI decision before any engine code.
- `specs/adr/0010-individual-positions.md` -- create ADR 0010 with the content listed in Always, citing ADR 0008 for the schema-version pattern -- records the persistence change and the revocation.
- `specs/http/bff.md` -- add the Screens section: route, header and `406`, `404`, envelope example, action types, failure policy summary, the 15-type table (type, section ids, variants, props with display-string vs cents noted), and the home moment priority table -- one contract for BFF and web.

**Acceptance Criteria:**
- Given the repository, when a reader opens `specs/adr/`, then ADR 0009 and ADR 0010 exist with Status accepted and ADR 0010 names phase-2 decision 9 as revoked.
- Given `specs/http/bff.md`, when a reader lists its component types, then exactly the 15 types of `ux.md` appear, each with its sections and variants matching `ux.md`, and the four action types appear.
- Given the change, when `git diff --stat` is run, then only `specs/` files and this story file changed.

## Design Notes

Props are documented as named fields per type (e.g. `moment_card`: `kicker`, `title`, `body`, `meta?`, `tone` ∈ neg|info|gold|neutral, `action?`). Money is a display string such as `"US$ 48.210,00"`; a value a form needs as a number is also sent as `*_cents` integer. Later stories may refine a prop list; they update this table in the same change.

## Verification

**Commands:**
- `git -C /workspace/repos/advisor-radar diff --stat` -- expected: only `specs/adr/0009-*`, `specs/adr/0010-*`, `specs/http/bff.md`, and story files.
- `grep -c '^| `' specs/http/bff.md` -- expected: the type table has 15 type rows (inspect manually).

**Manual checks (if no CLI):**
- Every type/section/variant in bff.md matches `ux.md` exactly; all text is English except quoted Portuguese UI copy.

## Auto Run Result

Status: done

- **Summary:** Recorded ADR 0009 (SDUI composed in the BFF, embedded read-only catalog) and ADR 0010 (per-product positions with units, revoking phase-2 decision 9), and added the phase-3 screen contract to `specs/http/bff.md`: route, `X-SDUI-Schema`/`406`/`400`/`404`, envelope with `title`/`subtitle`, values, four action types, failure policy, moment priority by fact owner, sections per screen, and the 15-type table with defaults.
- **Files:** `specs/adr/0009-server-driven-ui-in-the-bff.md` (new ADR); `specs/adr/0010-individual-positions.md` (new ADR); `specs/http/bff.md` (Screens section).
- **Review:** 46 findings; 37 patched (9 medium, 28 low), 1 deferred (below-minimum purchase), 8 rejected (5 false with refutation; segment 24 h window fixed by architecture; screen deadline owned by story 5; duplicates counted once each).
- **Follow-up review recommended:** true — the moment-fact ownership and the units value model were patched at medium and bind stories 4, 8, and 13.
- **Verification:** `git diff --stat` shows only the two ADRs and bff.md; `grep -c '^| \`' specs/http/bff.md` = 15.
- **Residual risks:** prop names are derived from the design and may be refined by later stories, which must update the table in the same change.
