---
title: 'Ship the add-sdui-variant skill'
type: 'feature'
created: '2026-09-29'
status: 'done'
review_loop_iteration: 0
baseline_revision: '8385498'
followup_review_recommended: false
context:
  - '{project-root}/AGENTS.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/architecture.md'
  - '{project-root}/specs/http/bff.md'
  - '{project-root}/specs/adr/0009-server-driven-ui-in-the-bff.md'
warnings: []
deferred: []
---

<intent-contract>

## Intent

**Problem:** A developer has no documented, repeatable path to add a section variant. The CAP-10 promise — a new variant shows after restarting only the BFF — is not packaged.

**Approach:** Write `.claude/skills/add-sdui-variant/SKILL.md` in this repository. It is an agent skill with name and description frontmatter. Given a section, a moment (the matching rule), and copy, it produces:
- the catalog entry in `internal/screen/catalog.json`;
- the `Variant` implementation with its `Matches`/`Build` tests;
- the priority placement;
- the `specs/http/bff.md` contract update.

Prove the skill by following it once on a throwaway variant, then removing that variant.

## Boundaries & Constraints

**Always:**
- **Skill file.** `SKILL.md` has YAML frontmatter with `name: add-sdui-variant` and a `description` that says when to use it. The body is English and imperative. It names the real files and functions as they exist after stories 5–13: the catalog JSON schema, the variant list order, the needs and sources in the Snapshot, the formatters, and the template test.
- **Skill inputs.**
  - section id and screen;
  - variant name;
  - the fact it matches on;
  - the data source, and whether a new Snapshot source is needed;
  - the copy with `{{.Field}}` names;
  - tone, icon, and action.
- **Skill steps.**
  1. Read the contract and catalog.
  2. Add the catalog entry, placing it in the section's variant order.
  3. Implement the `Variant`: `Matches` reads only Snapshot facts (the BFF computes no business threshold, so thresholds belong to the fact owner), `Build` fills props via the catalog templates and the Go formatters, and it declares its needs so the failure policy falls through correctly.
  4. Write the tests: `Matches` true and false, `Build` props, priority order, and failure fall-through. The template test runs automatically.
  5. Update the `specs/http/bff.md` variants and props tables.
  6. Check that web needs no change when the type already exists. When the type is new, point to the registry step.
  7. Run the Go gate.
  8. Restart only the BFF and verify with `curl` against the screen route (show the command).
- **Skill guardrails.** Portuguese copy only in the catalog. No web formatting. No `utils` package. Never edit `.env`.
- **Proof run.**
  - Follow the skill for a throwaway moment variant: for example `cash_only` on home, matching when the account has no positions, placed just above `welcome`, with copy "{{first}}, sua conta só tem caixa".
  - Run the Go tests.
  - Build and restart only the BFF against running account-sim and advisory, and `curl` a customer whose snapshot matches. You may need a temporary fake fact; use a unit screen test if no seed client matches.
  - Record the evidence: the diff stat of the throwaway change and the `curl` or test output. Then revert the throwaway variant completely.
  - Note in the skill any step the proof run showed was wrong or missing, and fix the skill text.
- **Discoverability.** Reference the skill from `specs/adr/0009-server-driven-ui-in-the-bff.md` or `README.md` in one line.

**Never:**
- The throwaway variant does not remain in the codebase.
- No change to the BFF behaviour.
- Do not edit `/workspace/.claude` (workspace-level) or `CLAUDE.md`/`AGENTS.md`.
- Do not read or edit `.env` / `.env.*`.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Proof run | throwaway `cash_only` | catalog entry + variant + tests + contract row produced by following the skill; tests pass; BFF-only restart serves it | — |
| Revert | after proof | `git status` shows only the skill and the one-line reference | — |

</intent-contract>

## Code Map

- `internal/screen/{engine,home,catalog,snapshot,format}.go`, `internal/screen/catalog.json` -- variant mechanics.
- `specs/http/bff.md` -- variants and props tables.
- `web/src/sdui/registry.tsx` -- where a new type would be registered.

## Tasks & Acceptance

**Execution:**
- `.claude/skills/add-sdui-variant/SKILL.md` -- the skill.
- Proof run, then revert.
- One-line reference in ADR 0009 or README.

**Acceptance Criteria:**
- Given the skill, when an agent follows it for a new moment, then it produces the catalog entry, the variant with tests, and the contract update, and the variant appears after restarting only the BFF.

## Verification

**Commands:**
- `gofmt -l . && go vet ./... && go build ./...` -- expected: clean after the revert.
- `mkdir -p .gotmp && GOTMPDIR=$PWD/.gotmp go test -race -shuffle=on ./...` -- expected: pass after the revert.
- `git status --porcelain` -- expected: only the skill file and the reference line.

## Review Triage Log

### 2026-09-29 — Review pass
- verdicts: 23 findings (22 from the layers, 1 orchestrator spot-check) — high 0, medium 2, low 18, false 3, maybe-false 0. The two medium rows share one root cause and form one patched entry.
- findings:
  - Blind Hunter:
    - `[low]` `[patch]` The Inputs example copy `{{.FirstName}}, sua conta …` breaks the skill's own step 2.3 guard rule. — Replaced with the guarded form.
    - `[medium]` `[patch]` The throwaway live check inherits `.env` broker URLs and `ADVISORY_HTTP_ADDR`. The throwaway advisory would consume shared queues and clash on the HTTP port. The BFF targets were not named and there was no restore step. — Confirmed: `cmd/advisory` and `cmd/account-sim` read `ADVISORY_BROKER_URL`, `ACCOUNT_SIM_BROKER_URL` and `ADVISORY_HTTP_ADDR` through `envfile.Load`. Step 8 now names every variable to export, the BFF targets, the restore step, and that cases and timeline stay shared.
    - `[false]` `[reject]` The example data change leaves `acoes`/`etfs`/`renda_fixa` non-zero, so the class totals contradict the empty positions. — Disproved: `migrations/account_sim/004_pov_positions.sql:7-9` drops those columns, so class totals are sums of `pov_position`. The implementer still rewrote the sentence to say exactly what the proof run changed (`pov_position` rows deleted, `caixa = 0`) and why that is enough.
    - `[low]` `[patch]` The beta `PUT` hard-codes `chat` (overwriting the channel), ignores a 429, and gives no restore command. — Confirmed: there is no GET preferences route. The step now reads the channel in Perfil, checks for 200, and restores it.
    - `[low]` `[patch]` The jq check hides an omitted section and non-200 answers. — It now prints `.omitted` and the status.
    - `[low]` `[patch]` `errAbsent` was missing from the engine failure policy (`engine.go:23-31`). — Bullet added.
    - `[low]` `[patch]` Step 5 missed three `bff.md` spots: the served-now Source cell, the nameless-title sentence (`bff.md:189`), and the advisor-copy failure bullet (`bff.md:140`). — Added.
    - `[low]` `[patch]` The description promised "prototype", but a beta-only variant is impossible under `TestCatalog_HomeBetaRevision`. — Wording corrected.
    - `[low]` `[patch]` The gate omitted gitleaks, which CI and pre-push run first. — Gate line added.
    - `[low]` `[patch]` Done-when omitted the web `MOMENT_RULES` follow-up, the beta check and the bff fake check, and `CUSTOMER=<id>` had no discovery route. — Added, pointing to `GET /v1/client-pov/customers`.
  - Edge Case Hunter:
    - `[medium]` `[patch]` The throwaway services inherit `.env` broker URLs and HTTP address (same root cause as the Blind Hunter's medium finding). — The fix is shared.
    - `[low]` `[patch]` A source that `Matches` reads but that is in neither `needs` nor `uses` is never fetched (`planSources`, `engine.go:269`), so the variant never matches. — Rule added in step 3.5.
    - `[low]` `[patch]` The shadowing example named only `idle_cash`, but `segment_upgrade_near` and `portfolio_review` also outrank a slot above `welcome`. — Named.
    - `[low]` `[patch]` A section mirrored across screens (Investir `highlights` → home `v2`) needs the same variant list. — Sentence added in step 2.1.
    - `[low]` `[patch]` The profile-keyed sections have no catch-all default and are exempt in `TestEngine_EverySectionEndsInDefault` (`engine_test.go:137-141`). — The exemption is noted.
    - `[low]` `[patch]` `errAbsent` was missing (same root cause as the Blind Hunter's `errAbsent` finding). — The fix is shared.
    - `[low]` `[patch]` "Its failure is logged as a screen failure" read as applying to `uses`. Per `planSources`, only `needs` sources are required. — Reworded.
    - `[low]` `[patch]` The beta `PUT` overwrites the channel and ignores a 429 (same root cause as the Blind Hunter's beta finding). The reviewer's "GET first" suggestion does not apply, since no GET route exists. — The fix is shared.
    - `[low]` `[patch]` The jq filter hides `build_error` and non-200 answers (same root cause as the Blind Hunter's jq finding). — The fix is shared.
  - Verification Gap: no gaps found.
  - Intent Alignment:
    - `[low]` `[reject]` No saved artifact shows `curl` returning `cash_only`. — The implementer's live-check transcript is recorded in the Auto Run Result, which is the spec's own evidence section. The only fix would be to edit this build's spec.
    - `[false]` `[reject]` The skill narrows the "web needs no change" promise. — Disproved: rendering needs no change for an existing type. The `MOMENT_RULES` note documents an optional showcase copy, which is not a CAP-10 regression.
    - `[false]` `[reject]` The `cash_only` example derives "no positions" instead of reading a supplied fact. — Disproved: an emptiness check on a Snapshot list compares no business threshold, and the throwaway variant is reverted, so nothing ships.
  - Orchestrator spot-check:
    - `[low]` `[patch]` Step 6 cites `web/src/selection/SelectionScreen.test.tsx`, which does not exist; the file is `web/src/screens/SelectionScreen.test.tsx`. — Path fixed.

## Auto Run Result

**Summary:** Added `.claude/skills/add-sdui-variant/SKILL.md` (339 lines), an agent skill with `name`/`description` frontmatter. Given a screen section, a variant name, the fact, the copy, the tone/icon/action and a priority slot, it walks the developer through eight steps:
1. read the contract and the catalog, and check the fact and shadowing;
2. add the catalog entry in every revision with the section, plus the copy;
3. implement the `Variant` and register it with `needs`/`uses`;
4. write the `Matches`/`Build`/priority/fall-through tests, and check the `internal/bff` fakes;
5. update the four areas of `specs/http/bff.md`;
6. check web (tones, icons, the `MOMENT_RULES` showcase follow-up);
7. run the CI gate;
8. rebuild and restart only the BFF, check before and after with `curl`, check the beta `v2`, or use an isolated throwaway stack when no seed client holds the fact.

It names the real files, helpers and tests as they are after stories 5–13 and 16. The README "Contract" paragraph now links the skill; ADR 0009 already names it.

**Proof run:** the implementer followed the skill on a throwaway home `moment_card` variant, `cash_only` (the account has no positions; slot just above `welcome`), in the worktree, then reverted it.
- **Diff stat:** `internal/bff/screen_test.go` +4, `internal/screen/catalog.json` +8, `internal/screen/moment.go` +31, `internal/screen/moment_test.go` +86/-1, `specs/http/bff.md` +6/-6. Total: 5 files, 135 insertions, 6 deletions.
- **Tests:** `TestCashOnlyMoment`, `TestMomentVariants_Registry`, `TestHome_Moment` (priority, shadowing and fall-through rows), `TestCatalog_EveryTemplateExecutes` and `TestCatalog_HomeBetaRevision` passed. So did the full `go test -race -shuffle=on ./...`, `vet`, `build`, `gofmt` and the tidy check.
- **Live, BFF-only restart:** on throwaway `s15_*` databases, Fernanda's positions were deleted and `caixa` set to 0. The old BFF served `v1` moment `welcome`. After rebuilding and restarting only the BFF, it served `v1` moment `cash_only` ("Fernanda, sua conta só tem caixa", tone `info`, icon `cash`, navigate to `investir`). With beta on it served `v2` (`moment, highlights, wealth, actions, advisor, activity`) with the same moment. Thiago kept `idle_cash`.
- **Revert:** the throwaway was fully reverted, the processes were stopped and the databases dropped. `git status` showed only the skill and the README line.

**What the proof run changed in the skill:**
- a warning that the bff handler-test fakes may start matching the new variant;
- a shadowing example;
- the failure-policy sentence that goes stale;
- the web `MOMENT_RULES` showcase copy;
- the concrete restart and throwaway procedure.

**Files:**
- `.claude/skills/add-sdui-variant/SKILL.md` — the new skill.
- `README.md` — one sentence linking the skill from the "Contract" paragraph.

**Review:** 23 findings (the Verification Gap layer found none).
- **Patched:** 19 rows in 15 entries (four Edge Case Hunter rows share a root cause with Blind Hunter rows), one medium and the rest low. The medium was the throwaway stack inheriting `.env` broker URLs and HTTP address, reported by both layers.
- **Rejected:** 4 rows:
  - the curl-evidence row, because the evidence is recorded here;
  - two Intent Alignment readings, disproved;
  - the `pov_account` class columns, which migration 004 dropped.
- **Deferred:** 0.

**Followup review recommended:** no. One medium entry and no high were patched.

**Verification:** no Go or web code changed. `go vet ./...` and `go build ./...` pass on the main tree. Every file, function, test and bff.md sentence the skill cites was spot-checked against the tree (`engine.go` `errAbsent`/`planSources`, `moment.go` helpers, the test names, `bff.md:140`/`:189`, the migration columns, `lefthook.yml` gitleaks, `web/src/screens/SelectionScreen.test.tsx`). `git diff --stat` covers only the README and the skill.

**Residual risk:** the skill mirrors today's code. A future change to the engine's failure policy or to the catalog layout must update it, and no test pins its text.
