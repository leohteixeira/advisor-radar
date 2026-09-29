---
title: 'Personalize the home by client moment'
type: 'feature'
created: '2026-09-29'
status: 'done'
review_loop_iteration: 0
baseline_revision: 'ffdbcc9a6bfe12a5a0dfe69fc58fc9617665f78c'
followup_review_recommended: true
context:
  - '{project-root}/AGENTS.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/architecture.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/ux.md'
  - '{project-root}/specs/http/bff.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/stories/5-build-the-bff-screen-engine-and-serve-the-home.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/stories/7-open-cases-from-triaged-messages.md'
warnings: []
deferred:
  - summary: >-
      cmd/advisory wiring of the account-sim reader into the gRPC server is untested.
    evidence: |-
      Dropping the WithAccountReader option in main would make every advisory moment fall back to welcome with no failing test; the repo tests no service main, and covering it needs run() refactored into a testable constructor.
    location: >-
      cmd/advisory/main.go
    severity: medium
---

<intent-contract>

## Intent

**Problem:** Every client opens on the same `welcome` moment. Advisory has no investor profile, no max-risk table, and no way to state a client's moment facts. `ListCases` cannot filter by customer. The home therefore cannot personalize, and the demo's live changes (Fernanda's upgrade, Mariana's case) do not show.

**Approach:**
- Advisory gains a per-customer investor profile, with assessment date and the max-risk table, plus two gRPC queries: `GetInvestorProfile` and `GetMomentFacts`. Advisory evaluates each moment condition.
- Cases' `ListCases` gains an optional `customer_id` filter.
- The BFF Snapshot adds three sources (moment facts, investor profile, open cases) and applies the fixed priority in `specs/http/bff.md` "Home moment priority", excluding `portfolio_drop` (story 13).
- Expected result: Fernanda opens on `segment_upgrade_near`, Thiago on `idle_cash`, Mariana on `portfolio_review`. A US$ 10,000 deposit by Fernanda switches her to `segment_upgraded`. A complaint by Mariana switches her to `case_open`.

## Boundaries & Constraints

**Always:**

- **Skills.** Load `golang-how-to` first and apply the Go skills in `/workspace/repos/advisor-radar/CLAUDE.md`.

- **Advisory profile data.**
  - `migrations/advisory/003_investor_profile.sql` adds `investor_profile TEXT NOT NULL` (`conservador|moderado|arrojado`, with a CHECK) and `profile_assessed_on DATE NOT NULL` to `book`.
  - The seed sets Fernanda to conservador 2026-03-12, Mariana to moderado 2026-01-20, and Thiago to arrojado 2026-08-04.
  - Every other book customer gets a literal profile and date from one fixed-seed draw. Write the literals into the seed and note the generator seed in a SQL comment. Make the migration safe on a populated database: add the columns with a default, then drop the default.
  - The max-risk table is Go code in advisory: conservador 2, moderado 3, arrojado 5. It is the only place this mapping lives. The BFF gets `max_risk` from the RPC.

- **Proto `advisory/v1`** (regenerate with `scripts/gen-proto.sh`).
  - `GetInvestorProfile(customer_id)` returns `{profile, max_risk, assessed_on (YYYY-MM-DD)}`, or `NotFound`.
  - `GetMomentFacts(customer_id)` returns:
    - `segment_upgraded` (bool) and `upgraded_segment` (string);
    - `segment_upgrade_near` (bool) and `upgrade_gap_cents`;
    - `idle_cash` (bool), `cash_cents`, and `patrimony_cents`;
    - `portfolio_review` (bool);
    - `portfolio_drop` (bool, always false until story 13; keep the field so story 13 only fills it).

- **Fact rules.** Advisory evaluates these as small pure functions, one per fact:
  - `segment_upgraded`: a segment-upgrade alert (kind `segmento`, `to` above `from`) for the customer, raised in the last 24 h from an account event with `schema_version` ≥ 2.
    - Record the source event's schema version on new alerts: add a column in the same migration, or put it in the alert payload if one already carries it.
    - The seeded cast alerts are version 1 or absent, so they never match. Thiago still opens on `idle_cash`.
    - `upgraded_segment` is the `to` segment.
  - `segment_upgrade_near`: `750000 ≤ patrimony_cents < 1000000`; `upgrade_gap_cents = 1000000 − patrimony_cents`.
  - `idle_cash`: patrimony > 0 and `cash_cents * 2 ≥ patrimony_cents`.
  - `portfolio_review`: the book segment is Singular.
  - Patrimony and cash come from account-sim `account/v1` `GetAccount`, called by advisory through a small consumer port (`AccountReader`) with a propagated deadline (2 s default when the context has none). It is dialed on a new `ACCOUNT_SIM_GRPC_TARGET` for advisory. Document that variable where advisory configuration is documented; no `.env` edit.
  - When the account read fails, `GetMomentFacts` returns `Unavailable` and the BFF falls back as the failure policy says.
  - An unknown customer returns `NotFound`.

- **Advisor id.** `advisory/v1` `Customer` gains `string advisor_id = 7` (the book's `advisor_id`); the cases `AdvisoryLookup` (story 7) reads it and drops the `ListOperators` name match. **Cases.** `ListCasesRequest` gains `string customer_id = 1`.
  - Empty means every case, exactly as today.
  - When set, the store filters `WHERE customer_id = $1` and a non-UUID value is `InvalidArgument`.
  - The Bastidores board call keeps sending an empty request.

- **BFF Snapshot** (`internal/screen`).
  - Add `Moments`, `Profile`, and `Cases` (the customer's non-`Resolvido` cases: id, opened_at or opened-ago minutes, state) sources with their own consumer interfaces, adapters in `internal/bff`, and their own errors, all under the same deadline.
  - Extend `Source` and `failed`.
  - Variant needs follow the failure policy:
    - `case_open` needs cases (on failure it does not match and evaluation continues);
    - the advisory facts variants need moments;
    - `idle_cash` also needs the profile for its label, and falls through if the profile failed;
    - `welcome` needs nothing.

- **Moment variants**, in this order in the catalog: `case_open`, `segment_upgraded`, `segment_upgrade_near`, `idle_cash`, `portfolio_review`, `welcome`. Copy follows `ux.md` "Home moments", with tone, icon, and action from the design (`OrlaApp.dc.html:680-705`).
  - **`case_open`**: title "Sua reclamação está com a {{advisor}}" from the advisory customer's advisor name. Body "Como cliente {{segment}}, você recebe resposta em até {{sla}}." using the catalog's segment SLA text. Meta "Protocolo {{protocol}} · aberto há {{age}}":
    - protocol is the first 13 characters of the most recent open case id, uppercased (e.g. `01A0E3A5-2F4C`);
    - age is the story 5 relative-time formatter over the case's opened time.

    Action `panel message` ("Ver conversa").
  - **`segment_upgraded`**: "{{first}}, você agora é cliente {{segment}}", "Sua assessoria passa a responder em até {{sla}}.", using the upgraded segment and its SLA. Action `navigate investir` ("Ver produtos").
  - **`segment_upgrade_near`**: "{{first}}, faltam {{gap}} para o Advance", the fixed body, action `panel deposit` ("Depositar").
  - **`idle_cash`**: "{{first}}, {{cash_share}} do seu patrimônio está em caixa". Body "{{cash}} parados há {{idle_days}} dias. Veja produtos para o seu perfil {{profile}}."
    - `cash_share` uses the story 5 percent formatter.
    - `idle_days` is the whole days since the most recent client-facing account row in the timeline source, minimum 1, with "1 dia" singular.
    - When the timeline failed or has no account row, the body is "{{cash}} parados em caixa. Veja produtos para o seu perfil {{profile}}."
    - `profile` is lowercase ("arrojado").

    Action `navigate investir` ("Ver produtos").
  - **`portfolio_review`**: the design text with the advisor's name, action `panel message` ("Conversar").
  - The BFF formats money and percentages only. It compares no threshold.
  - A moment whose copy needs the advisor name while advisory `GetCustomer` failed falls through to the next variant.

- **Contract docs.** Update `specs/http/bff.md`: the moment variants now served, the new sources in the failure policy wording if needed, and the served-home note that said `welcome` is fixed. Update the matching `ux.md` copy only if a template differs.

- **Web.** No change is needed. The story 6 `MomentCard` already renders every tone, `meta`, and action. If a web test fixture must add a variant for coverage, that is allowed; verify that `pnpm --dir web test` still passes.

- **Style.** Wrap errors with `%w`; propagate `context.Context`. Info logs carry no copy or account values.

**Never:**
- No `portfolio_drop` evaluation, `with_day_change`, purchase, suitability alert, Investir or Perfil screens, or `X-SDUI-Schema` (stories 9–16).
- The BFF never computes segmentation or a threshold.
- Do not share databases across services.
- Unit tests do not dial 5435/5673/8400/8420/9201.
- Do not read or edit `.env` / `.env.*`. Never a `utils` package.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Fernanda day 0 | patrimony 820000, cash 114800 | `segment_upgrade_near`, "Fernanda, faltam US$ 1.800,00 para o Advance", action deposit | — |
| Thiago day 0 | cash 6052000 / 6800000 | `idle_cash`, "Thiago, 89% do seu patrimônio está em caixa", profile "arrojado" | — |
| Mariana day 0 | Singular, cash 24% | `portfolio_review` with advisor name | — |
| Fernanda deposits US$ 10,000 | v2 event → advisory upgrade alert (Essencial→Advance) | `segment_upgraded` "Fernanda, você agora é cliente Advance", SLA "4 h" | — |
| Mariana complaint | story 7 opened a case | `case_open`, "Como cliente Singular, você recebe resposta em até 1 h.", meta "Protocolo … · aberto há …" | — |
| Cases down | ListCases fails | `case_open` skipped; next match | not propagated |
| Advisory moments down | GetMomentFacts fails | `case_open` if cases says so, else `welcome` | not propagated |
| Account-sim down for advisory | advisory GetAccount fails | `GetMomentFacts` Unavailable → BFF as above | — |
| Unknown customer | GetMomentFacts / GetInvestorProfile | `NotFound` | — |
| ListCases filter | `customer_id` = Mariana / empty / "x" | only her cases / all / `InvalidArgument` | — |
| Seeded upgrade | Thiago's cast `segmento` alert (v1) | not `segment_upgraded` | — |

</intent-contract>

## Code Map

- `internal/advisory/{grpc,pgx,rules,apply}.go`, `migrations/advisory/002_uuidv7.sql`, `seeds/advisory/001_cast.sql` -- book, alerts (payload JSONB), gRPC server; segment rule `ruleSegment` at `rules.go:132`; apply stores alerts.
- `proto/advisory/v1/*.proto`, `proto/cases/v1/*.proto`, `scripts/gen-proto.sh`, `gen/`.
- `cmd/advisory/main.go` -- wiring; add the account-sim dial (see `internal/bff/account.go` `DialAccountSim` for interceptor style; advisory may use a simpler deadline-only interceptor).
- `internal/cases/grpc.go` -- `CaseReader.ListCases` and the gRPC `ListCases`.
- `internal/bff/clients.go` -- `GRPCCases`, advisory client; `internal/bff/screen.go` -- snapshot source adapters.
- `internal/screen/{snapshot,home,engine,catalog}.go`, `internal/screen/catalog.json` -- sources, needs, variants, copy.
- `docs/design/sdui-full-pov/project/OrlaApp.dc.html:680-705` -- design moment builders.
- `internal/seeds/consistency_test.go` -- seed id checks.

## Tasks & Acceptance

**Execution:**
- Advisory: migration 003, seed profiles, max-risk table, fact rules (+ table tests per fact), `AccountReader` port + account/v1 adapter, the two RPCs (+ bufconn tests), `cmd/advisory` wiring and docs.
- Cases: `ListCases` filter (+ tests); BFF board unaffected. Advisory `Customer` gains `advisor_id` (field 7) and the cases `AdvisoryLookup` uses it instead of matching the name through `ListOperators` (story 7 deferral).
- BFF: Snapshot sources and adapters, moment variants and catalog copy (+ one test per variant `Matches`/`Build`, priority-order test, each failure row, the three seed homes and the two live transitions with fake sources), `specs/http/bff.md`.

**Acceptance Criteria:**
- Given the reseeded local stack, when each seed client's home is requested, then Fernanda gets `segment_upgrade_near`, Thiago `idle_cash`, and Mariana `portfolio_review`. After Fernanda's US$ 10,000 deposit her home gets `segment_upgraded`, and after Mariana's complaint hers gets `case_open`.
- Given the change, when the Go suite and `pnpm --dir web test` run, then all pass.

## Verification

**Commands:**
- `sh scripts/gen-proto.sh && git status --porcelain gen/` -- expected: only advisory and cases generated files changed.
- `gofmt -l . && go vet ./... && go build ./...` -- expected: clean.
- `mkdir -p .gotmp && GOTMPDIR=$PWD/.gotmp go test -race -shuffle=on ./...` -- expected: pass.
- `go mod tidy && git diff --exit-code go.mod go.sum` -- expected: clean.
- `pnpm --dir web test` -- expected: pass.

## Review Triage Log

### 2026-09-29 — Review pass
- verdicts: 30 findings — high 0, medium 5, low 22, false 3, maybe-false 0
- findings:
  - `[low]` `[reject]` blind: `case_open` says "reclamação" for closing or churn cases — the title is the `ux.md` copy; every case comes from a client-app message.
  - `[low]` `[reject]` blind: `OpenCase.State` unused — harmless field; state-specific copy is not in the intent.
  - `[false]` `[reject]` blind: resolved filter compares a display label — `Resolvido` is the canonical stored state value in cases (`StateResolvido`).
  - `[low]` `[reject]` blind: protocol is the UUIDv7 timestamp prefix — the protocol format is defined by phase 2 and `ux.md`; collisions need two cases in one millisecond.
  - `[low]` `[patch]` blind: `segment_upgrade_near` body hardcodes threshold and SLA — filled from the catalog SLA and formatter.
  - `[low]` `[patch]` blind: near ignores the segment — requires Essencial.
  - `[false]` `[reject]` blind: `schema_version` used as the live marker — the intent prescribes `schema_version` ≥ 2.
  - `[false]` `[reject]` blind: `idle_days` minimum 1 — the intent prescribes a minimum of 1.
  - `[low]` `[reject]` blind: advisory→account-sim client lacks retry/tracing interceptors — the adapter applies the deadline; tracing is story 17; a blip degrades to `welcome` by policy.
  - `[low]` `[patch]` blind: canceled/expired context mapped to Unavailable/Internal — `status.FromContextError`.
  - `[low]` `[patch]` blind: deploy order advisory before cases — README note.
  - `[low]` `[patch]` blind: new service dependency and double balance read unrecorded — ADR 0011.
  - `[low]` `[patch]` blind: README misses advisory migration and reseed — added.
  - `[low]` `[patch]` blind: duplicated date layout and inline no-book status — `time.DateOnly`, `errNoBook`.
  - `[low]` `[patch]` blind: `MomentBook` reads non-atomically — one read-only transaction.
  - `[low]` `[patch]` blind: missing tests (advisor_id end to end, cash > patrimony) — added; the cash > patrimony case clamps to 100% (Shares treats negatives as 0) rather than erroring, and the test pins that; deadline forwarding through the server rejected (adapter covered).
  - `[medium]` `[patch]` verification-gap: `GetCustomer.advisor_id` never asserted through the real server — gated bufconn test.
  - `[medium]` `[patch]` verification-gap: `GRPCQueue.MomentFacts` upgrade fields never mapped in a test — bufconn test over every field.
  - `[medium]` `[defer]` verification-gap: `cmd/advisory` account reader wiring untested — the repo tests no service `main`; needs a testable constructor.
  - `[low]` `[patch]` edge: exactly the Advance floor shows no near moment — band aligned with `SegmentFromAssets`.
  - `[medium]` `[patch]` edge: live upgrade alert survives a reseed and shows `segment_upgraded` on day 0 — latest alert `To` must equal the book segment.
  - `[low]` `[patch]` edge: Advance client in band told to reach Advance — grouped with the Essencial guard.
  - `[low]` `[patch]` edge: `Build` fails on a segment without SLA and omits the moment — SLA checked in `Matches`.
  - `[low]` `[reject]` edge: non-complaint case titled as complaint — grouped with the rejected copy finding.
  - `[low]` `[patch]` edge: context errors hidden as Unavailable — grouped.
  - `[low]` `[reject]` edge: clock skew ignores a just-raised alert — needs skew between local services on one host.
  - `[low]` `[patch]` edge: removed name fallback needs advisory first — grouped with the deploy-order note.
  - `[medium]` `[patch]` edge claim: reseeded Fernanda gets `segment_upgraded` — grouped with the segment check.
  - `[low]` `[reject]` intent: demo transitions tested with injected facts on each side — the implementer's live smoke ran both transitions end to end; the `GRPCQueue` mapping test closes the BFF side.
  - `[low]` `[reject]` intent: "advisory down" differs between engine and BFF tests — both are real failure modes, each tested; the advisor-name rule is documented.

## Auto Run Result

Status: done

- **Summary:**
  - **Advisory.** Advisory stores an investor profile and assessment date per book customer. The max-risk table lives only in Go. It serves `GetInvestorProfile` and `GetMomentFacts`, whose facts are pure functions:
    - `segment_upgraded` fires on a live v2+ upgrade in the last 24 h whose target matches the book segment;
    - `segment_upgrade_near` fires for Essencial clients from US$ 7.500 up to the Advance boundary;
    - `idle_cash`;
    - `portfolio_review`.

    It reads balances from account-sim with a deadline. `Customer` carries `advisor_id`.
  - **Cases.** `ListCases` filters by customer, and intake uses `advisor_id`.
  - **BFF.** Moments, profile and open cases are added as Snapshot sources, and the home applies the fixed priority: `case_open`, `segment_upgraded`, `segment_upgrade_near`, `idle_cash`, `portfolio_review`, `welcome`.
- **Files:**
  - Migration and seed: `migrations/advisory/003_investor_profile.sql`, `seeds/advisory/001_cast.sql`.
  - Advisory: `internal/advisory/{profile,moments,account,grpc,pgx,apply}.go`.
  - Proto and generated code: `proto/advisory/v1`, `proto/cases/v1` and `gen/`.
  - `internal/cases/{grpc,advisory}.go`.
  - Screen: `internal/screen/{moment,snapshot,engine,home,catalog,format,page}.go` and `catalog.json`.
  - BFF: `internal/bff/{clients,screen,http}.go`.
  - `cmd/advisory/main.go`.
  - Tests in every touched package.
  - Docs: `specs/adr/0011-advisory-reads-account-sim-for-moments.md`, `specs/http/bff.md`, `ux.md` notes, and `README.md`.
  - `.github/workflows/ci.yml`: `ADVISORY_TEST_DATABASE_URL`.
- **Review:** 30 findings. 17 patched in 13 fixes: 5 medium (3 entries) and 14 low. 1 deferred: the `cmd/advisory` wiring test. 12 rejected with evidence, 3 of them false.
- **Follow-up review recommended:** true. Three medium entries changed fact semantics: segment matching, the Essencial band, and caller-context mapping.
- **Verification:**
  - gofmt, vet and build are clean, and tidy is clean.
  - Proto regeneration changes only the advisory and cases generated files.
  - `go test -race -shuffle=on ./...` passes.
  - The gated advisory, cases and account-sim pgx tests pass.
  - `pnpm --dir web test` passes 189/189.
  - Live smoke by the implementer:
    - Fernanda gets `segment_upgrade_near` ("faltam US$ 1.800,00"), Thiago `idle_cash`, and Mariana `portfolio_review`.
    - After a US$ 10.000 deposit, Fernanda gets `segment_upgraded`.
    - After a complaint, Mariana gets `case_open`.
    - With account-sim down, the home degrades to `welcome` and wealth is omitted.
- **Residual risks:**
  - A home render reads the balance twice, non-atomically (ADR 0011).
  - The keyword heuristic misses some complaint wordings when no model key is set.
