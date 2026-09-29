---
title: 'Write the phase-3 README demo script'
type: 'chore'
created: '2026-09-29'
status: 'done'
review_loop_iteration: 0
baseline_revision: 'af5894b'
followup_review_recommended: false
context:
  - '{project-root}/AGENTS.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/SPEC.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/architecture.md'
  - '{project-root}/README.md'
warnings: []
deferred:
  - summary: >-
      timeline-indexer fails at startup once any case.sla.breached row exists in the cases outbox. (Resolved after story 19: the startup replay indexes only the routing keys the live consumer binds, shared as timeline.RoutingKeys, and cases no longer escalates or publishes a breach for a case already Resolvido.)
    evidence: |-
      The startup replay reads every cases outbox row. internal/timeline/index.go:309 returns
      `timeline: unsupported event "case.sla.breached"` and the process exits.
      internal/cases/machine.go:304 HandleBreach writes that row for every app-opened case whose SLA
      delay dead-letters, whatever its state (a resolved case too). Nothing prunes the outbox and
      the reseed does not clear it, so after the first breach every later start crashes.
      Reproduced in the story 19 walk: Mariana's step-4 case (30 min SLA) breached at 07:24:32 and
      the next start crashed (session scratchpad s19/08-timeline-crash-after-sla-breach.txt).
      Workaround documented in the README: `CASES_DATABASE_URL= go run ./cmd/timeline-indexer`.
    location: >-
      internal/timeline/index.go:309
    severity: medium
  - summary: >-
      The reseed does not reset app-opened cases, so Mariana's home stays on case_open instead of portfolio_review after a reseed. (Resolved after story 19: seeds/cases/001_cast.sql first resolves every non-cast open case; this also fixes a reseed that failed on cases_one_open_per_customer_idx when a cast customer held an app-opened case.)
    evidence: |-
      cmd/db seed only upserts seed rows. A case opened from the app in an earlier walk stays open,
      and case_open ranks above portfolio_review, so a reseed does not restore Mariana's seed moment
      (session scratchpad s19/00-stale-state-after-reseed.txt). This contradicts the CAP-2
      expectation that after a reseed Mariana is on portfolio_review. The README tells the
      reader to close the case in /advisor-radar/fila first.
    location: >-
      cmd/db seed / cases database
    severity: low
---

<intent-contract>

## Intent

**Problem:** The README does not tell a visitor how to run and walk through phase 3. The CAP-12 documentation half requires a seven-step demo script and pointers to the decisions and the contract.

**Approach:** Add a "Phase 3 demo" section to `README.md` with a seven-step script covering the SPEC success scenarios. Every step says what to click or request and what the visitor should see. Point to ADRs 0008–0010, `specs/http/bff.md`, the design reference (`docs/design/sdui-full-pov/`), and the `add-sdui-variant` skill. Verify the script against the running local stack.

## Boundaries & Constraints

**Always:**
- The README is English. Quoted UI strings stay Portuguese, exactly as shipped.
- **Prerequisites.** State the prerequisites and startup commands using the existing README configuration sections: compose services, `cmd/db migrate` and `seed`, starting the six services with their env var names (never values from `.env`), and `pnpm --dir web dev`. Include the reseed step that resets the day and the demo state.
- **The seven steps**, grounded in SPEC.md success criteria and the actual shipped behaviour:
  1. Open the selection screen and the `#sdui` section. Compare the three clients' homes.
  2. Open each seed client's home: Fernanda sees `segment_upgrade_near`, Thiago sees `idle_cash`, Mariana sees `portfolio_review`. Turn on Raio-X.
  3. As Fernanda, deposit US$ 10,000. The home switches to `segment_upgraded`, and the team queue shows the segment card.
  4. As Mariana, file a complaint from the app. A case opens with the Singular SLA and her home shows `case_open`.
  5. As Thiago, buy "Tudo" of a product in Investir. The confirmation and Bastidores show `aplicacao`, `idle_cash` disappears, and Carteira shows the position. As Fernanda, buy Cobalto: the warning appears, the purchase is not blocked, and the team queue gets the `perfil` card.
  6. Advance the day three times. Mariana's home shows `portfolio_drop` with the day-change pill, and the `queda` alert appears. Carteira shows the revaluation.
  7. Isolated failure and versioning:
     - stop the timeline-indexer, so the home still answers and Raio-X shows `activity` omitted with "timeline fora do ar";
     - request with `X-SDUI-Schema: 2` to get a `406` (give the `curl` command);
     - turn on beta in Perfil to get home revision `v2`;
     - optionally, look at the trace and metrics in Grafana on 3410.
- **Links.** "Decisions and contract" links: `specs/adr/0008…`, `0009…`, `0010…`, `specs/http/bff.md`, `docs/design/sdui-full-pov/`, and `.claude/skills/add-sdui-variant/SKILL.md`.
- **Verification.** Walk the script against the running local stack, either with the browser via the cached headless Chromium or with `curl` for the API parts. Fix any step that does not behave as written, in the README only. Record the verification in the spec result. If a step reveals a product bug, record it as deferred, with evidence, rather than fixing code in this chore.
- Keep the existing README sections intact apart from the new section and its table-of-contents entry, if a table of contents exists.

**Never:**
- No code change.
- Do not read or edit `.env` / `.env.*`, and never print secret values.
- No invented behaviour: the script describes only what the stack actually does.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Fresh reader | README only | can start the stack and run all seven steps | — |
| Step deviates | verification run | README corrected, or deferred bug recorded | — |

</intent-contract>

## Code Map

- `README.md` -- existing configuration and demo sections.
- `specs/adr/0008-*.md`, `0009-*.md`, `0010-*.md`, `specs/http/bff.md`, `docs/design/sdui-full-pov/`.
- `.claude/skills/add-sdui-variant/SKILL.md` (story 15).

## Tasks & Acceptance

**Execution:**
- `README.md` -- the phase-3 demo section and links.

**Acceptance Criteria:**
- Given the README, when a visitor follows the seven steps on the local stack, then each step shows what it says.

## Verification

**Commands:**
- Manual walk-through of the seven steps against the local stack -- expected: every step matches.
- `git diff --stat <baseline>` -- expected: README.md (and the spec) only.

## Review Triage Log

### 2026-09-29 — Review pass
- verdicts: 25 findings — high 0, medium 2, low 23, false 0, maybe-false 0
- findings:
  - Blind Hunter:
    - `[low]` `[patch]` The `.env` paragraph comes after `cmd/db migrate`/`seed`, which fail without it. — Moved before migrate/seed.
    - `[low]` `[patch]` `docker compose up -d` returns before Postgres is healthy. — `--wait` added.
    - `[low]` `[patch]` The credentials for the URLs are never named. — The README now points to the Compose defaults and the Configuration section, with no connection string.
    - `[low]` `[patch]` `export CASES_DATABASE_URL=` leaks into the whole shell (`envfile.Load` keeps empty values). — Per-command prefix.
    - `[low]` `[patch]` The known issue has no lasting path: the breach row survives reseed, so every later walk needs the workaround. — Stated.
    - `[low]` `[patch]` The step 7 `curl` doesn't say whose id it uses or that it bypasses the Vite proxy. — Mariana is named and the direct-BFF call is noted.
    - `[low]` `[patch]` Step 4 depends on triage without saying so, and has QA-log wording. — Reworded for the reader.
    - `[low]` `[patch]` Step 5's "Aplicado US$ 65.720,00" versus the US$ 60.520,00 purchase is unexplained; the difference is the seeded US$ 5.200,00 (`seeds/account_sim/003_pov_positions.sql:39`). — Explained.
    - `[low]` `[patch]` The new script doesn't relate to the phase-1/2 "Demo script", which it partly repeats. — One sentence added. The duplicate links stay, since the intent requires the "Decisions and contract" list.
    - `[low]` `[patch]` The Grafana step omits the 60 s metric interval and the login. — Links to Observability.
    - `[low]` `[reject]` The effect of the day-3 shock on Fernanda's small Cobalto buy isn't described. — Rejected: the step makes no claim about Fernanda, and the fix would add unverified behaviour to the script.
  - Edge Case Hunter:
    - `[medium]` `[patch]` `http://127.0.0.1:3400/advisor-radar` without the trailing slash is a 404 on the Vite dev server (base `/advisor-radar/`). — Both URLs now end in `/`.
    - `[low]` `[patch]` Migrate can run before Postgres is ready. — Same root cause as the Blind Hunter's `--wait` finding; shared fix.
    - `[low]` `[patch]` The `.env` is described after the command block. — Same root cause as the Blind Hunter's ordering finding; shared fix.
    - `[low]` `[patch]` "Any free ports work" ignores `ADVISORY_HTTP_URL` and the fixed BFF 8400. — Both constraints stated.
    - `[low]` `[patch]` `ADVISORY_HTTP_URL` has no value. — `http://127.0.0.1:8410` given (grouped with the previous row).
    - `[low]` `[patch]` Closing the case in time does not avoid the breach row (`internal/cases/machine.go:304` ignores the state). — Stated.
    - `[low]` `[patch]` The 30-minute window holds only on a cases database with no earlier breach. — Same root cause as the Blind Hunter's lasting-path finding; shared fix.
    - `[low]` `[patch]` `export` leaks an empty `CASES_DATABASE_URL`. — Same root cause as the Blind Hunter's `export` finding; shared fix.
    - `[low]` `[patch]` The header anchor reads "SDUI" on a phone (`web/src/screens/SelectionScreen.tsx:76`). — Noted.
    - `[medium]` `[patch]` Claim check: a fresh reader cannot reach step 1 because of the missing trailing slash. — Same root cause as the trailing-slash finding; shared fix.
  - Verification Gap: no gaps found.
  - Intent Alignment:
    - `[low]` `[reject]` The verification is not recorded in the spec. — Rejected: the fix edits this build's spec, which the Auto Run Result does at finalize.
    - `[low]` `[defer]` The timeline-indexer product bug is written in the README, not deferred in the spec. — The README note stays (it describes real behaviour), and the bug is recorded under `deferred` (medium).
    - `[low]` `[patch]` The prerequisites don't use or link the existing Configuration section. — Linked (same fix as the Blind Hunter's credentials row).
    - `[low]` `[defer]` The reseed doesn't reset the demo state (app-opened cases) as the intent assumed. — Pre-existing product behaviour, recorded under `deferred` (low); the README documents the manual close.

## Auto Run Result

**Summary:** `README.md` gains a "## Phase 3 demo" section (+96 lines; no existing section changed; the README has no table of contents). It has three parts.

- **Run the stack:**
  - Infra comes first with `docker compose up -d --wait`.
  - Then `.env`: a per-service table of variable names, with example listen addresses only. Credentials point to the Compose defaults and to the Configuration section.
  - The port-pairing rules: each gRPC target matches its address, `ADVISORY_HTTP_URL` matches `ADVISORY_HTTP_ADDR`, and the BFF stays on 8400.
  - Then migrate, seed, `pnpm --dir web install --frozen-lockfile`, the six `go run ./cmd/<svc>` commands, and `pnpm --dir web dev` at `http://127.0.0.1:3400/advisor-radar/`.
  - The reseed step says what it resets and what it keeps: close app-opened cases in the queue first, and the known timeline-indexer issue with its per-command workaround.
- **Script:** seven steps, each saying what to click or request and what appears, with the UI strings quoted in Portuguese exactly as shipped:
  1. selection `#sdui`;
  2. the three homes with Raio-X;
  3. Fernanda's US$ 10.000 deposit, giving `segment_upgraded` and the "Mudança de segmento" card;
  4. Mariana's complaint, giving `case_open`, the Singular clock and `00:30:00` under churn;
  5. Thiago's "Tudo" purchase, giving `aplicacao · schema 3`, `idle_cash` replaced by `welcome`, and the Carteira position; Fernanda's Cobalto purchase, giving the warning, no block, and the `perfil` card;
  6. advancing the day ×3, giving `portfolio_drop`, `with_day_change`, `queda` and the Carteira revaluation;
  7. timeline-indexer down, giving "timeline fora do ar"; `curl` with `X-SDUI-Schema: 2`, giving `406`; beta, giving `v2`; and Grafana (optional).
- **Decisions and contract:** links to ADRs 0008, 0009 and 0010, `specs/http/bff.md`, `docs/design/sdui-full-pov/` and the `add-sdui-variant` skill.

**Files:** `README.md` — the phase-3 demo section. No code changed.

**Verification:** the implementer walked all seven steps against the real local stack from a reseed, with the cached headless Chromium for the UI and `curl` for the API. Evidence is in the session scratchpad `s19/`: the transcripts `00`–`09` and the `step1-*`…`step8-*` screenshots.
1. `#sdui` showed "200 · 1 requisição" and "4 de 7 · segment_upgrade_near", "5 de 7 · idle_cash", "6 de 7 · portfolio_review".
2. The homes showed `segment_upgrade_near`, `idle_cash` and `portfolio_review` (with `advisor_card · dedicated`). The Raio-X banner read "slug home · revision v1 · schema 1 · 5 seções".
3. Fernanda showed `segment_upgraded`, "Fernanda, você agora é cliente Advance", and the queue had the "Mudança de segmento" card.
4. "Estou pensando em sair" was classified `reclamacao` with churn 0.98. The case opened, home showed `case_open`, and the queue showed "Aberto" at 00:30:00.
5. Thiago's purchase showed `aplicacao · schema 3` and home turned `welcome`. Carteira showed "Aplicado US$ 65.720,00". Fernanda's Cobalto purchase showed the warning, was not blocked, and the "Compra acima do perfil" card arrived about 750 ms later.
6. On day 3, Mariana's home showed `portfolio_drop` with "−US$ 38.520,00 (−15,5%) no dia 3". Her `queda` card was in the queue, and Carteira showed US$ 209.780,00.
7. Isolated failure and versioning:
   - With timeline-indexer stopped, the home returned 200 and the banner read "4 seções · timeline fora do ar", with `activity · activity_list · omitido`.
   - `X-SDUI-Schema: 2` returned `406 {"supported":{"min":1,"max":1}}`. `1` and `1-2` returned 200, and `abc` returned 400.
   - Beta on gave `v2` with 6 sections; off gave `v1` again.
   - Grafana Tempo showed `sdui.screen` / `sdui.snapshot.*` / `sdui.section` spans, and Prometheus had the `sdui_*`, `pov_purchases_total` and `advisory_suitability_alerts_total` series.

Steps that deviated from the spec's wording are written as observed:
- the reseed does not close app-opened cases;
- a different complaint preset can be classified `resgate` and open no case;
- the Singular clock shows 00:30:00 under churn.

The review patches (trailing slash, ordering, `--wait`, credentials pointer, port rules, per-command workaround, wording) are text-only and checked by reading: anchors exist, the Vite base is `/advisor-radar/`, and no connection string or credential is written. After the walk, the implementer resolved the open case, ran a final reseed, and stopped every process and the RabbitMQ and Elasticsearch containers. The day is back to 0 and the three homes are on their seed moments. To avoid the crash for the next reader, the implementer also deleted two stale `case.sla.breached` rows from the local cases outbox: this is local dev state only, and the bug is deferred. `git diff --stat af5894b` covers `README.md` only, and `go vet ./...` is clean.

**Review:** 25 findings (the Verification Gap layer found none).
- **Patched:** 21 rows in 15 entries: 1 medium (the entry URL without a trailing slash is a 404 on Vite, reported by two rows) and 14 low.
- **Deferred:** 2 — the timeline-indexer crash on `case.sla.breached` (medium) and the reseed not resetting app-opened cases (low).
- **Rejected:** 2 — Fernanda's day-3 effect (would add unverified behaviour) and the verification record (this section).

**Followup review recommended:** no. One medium entry and no high were patched.

**Residual risks:**
- Step 4 depends on triage's classification, whether by the model or the heuristic.
- Until the deferred timeline-indexer bug is fixed, every walk after the first SLA breach needs the documented workaround.
- The exact money values quoted in steps 3–6 hold only from a clean reseed with no other day advance.
