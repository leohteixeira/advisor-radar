---
title: 'Build the BFF screen engine and serve the home'
type: 'feature'
created: '2026-09-29'
status: 'done'
review_loop_iteration: 0
baseline_revision: '96d8f06f687f575742f0783bc1ce0779e91611fe'
followup_review_recommended: true
context:
  - '{project-root}/AGENTS.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/architecture.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/ux.md'
  - '{project-root}/specs/adr/0009-server-driven-ui-in-the-bff.md'
  - '{project-root}/specs/http/bff.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/stories/4-store-per-product-positions-and-the-product-catalog.md'
warnings: []
deferred:
  - summary: >-
      Segment SLA text lives in catalog.json and in povCatalog; nothing pins them together.
    evidence: |-
      cases owns SLA as a business rule; story 8 brings segment and moment facts from advisory and can consolidate.
    location: >-
      internal/screen/catalog.json
    severity: low
  - summary: >-
      The drop reporter is not wired to telemetry and there are no spans yet.
    evidence: |-
      CAP-9 (story 17) adds OTel; the engine already accepts a reporter.
    location: >-
      internal/bff/screen.go
    severity: low
---

<intent-contract>

## Intent

**Problem:** The BFF has no way to compose a client screen, so the SDUI contract in `specs/http/bff.md` has no server and web cannot migrate the home.

**Approach:** Add `internal/screen` — the envelope types, a `Snapshot` fetched in parallel under one screen deadline with per-source errors, an ordered `Variant` list per section (first match wins, last is the always-matching default), a `go:embed` catalog of section order and Portuguese copy templates, Go-side formatting, and the failure policy — and serve `GET /v1/client-pov/customers/{id}/screens/home` with the default variants (`moment` `welcome`), real wealth, actions, advisor, and timeline activity.

## Boundaries & Constraints

**Always:** Load `golang-how-to` first and apply the Go skills in `/workspace/repos/advisor-radar/CLAUDE.md`. `internal/screen` declares its own small consumer interfaces for sources (account-sim account/positions via `account/v1`, advisory customer, timeline search) and the BFF wires adapters over existing clients; `internal/screen` never imports `internal/bff` (no cycle). `Variant` is exactly `Matches(Snapshot) bool` and `Build(Snapshot, Catalog) (Component, error)`. Snapshot calls run with `errgroup` under one screen deadline (constant 800 ms, overridable in tests); one source failing never cancels the others, and each result carries its own error. The catalog is one versioned JSON (or TOML/YAML only if already a dependency — prefer JSON) file under `internal/screen/`, embedded with `go:embed`, keyed in English: per screen and revision the ordered section list (`id`, `type`, variants in evaluation order), and per variant its copy templates in `text/template` with named fields (`{{.FirstName}}`, `{{.AdvisorName}}`, …). Go formats money as `US$ 1.234,56` (pt-BR grouping, U+2212 for negatives), percentages as `62%` (largest-remainder so allocation shares sum to 100), and relative times (`agora`, `há 5 min`, `há 3 h`, `ontem`, `há 4 dias`) before values enter a template. Home `v1` sections and props follow `specs/http/bff.md`: `moment` (`moment_card`, only `welcome` in this story: kicker "Tudo em dia", title "Olá, {{first}}. Sua conta está em dia.", body "Quando algo mudar na sua carteira, você vê aqui primeiro.", tone `neutral`, no action), `wealth` (`wealth_summary` `default`: total patrimony, cash, `cash_cents`, allocation rows for non-zero classes Ações/ETFs/Renda fixa/Caixa with `share` and `bar_width`), `actions` (`action_grid` `default`: Depositar/Sacar/Mensagem/Reclamar `panel` actions `deposit`/`withdraw`/`message`/`complaint`), `advisor` (`advisor_card` `dedicated` for Singular else `default`, name and initials from advisory, meta "Resposta em até {{sla}} · cliente {{segment}}" where the SLA text comes from the catalog per segment: Essencial 24 h, Advance 4 h, Singular 1 h), `activity` (`activity_list` `recent` with up to 5 timeline rows mapped to `icon`, `title`, `meta`, else `empty` with "Suas movimentações aparecem aqui assim que acontecerem."). Envelope `title` "Olá, {{first}}", `subtitle` "Cliente {{segment}} desde {{since}}". Failure policy: account-sim `NotFound` → `404`; account failure → `wealth` omitted; advisory failure → `advisor` omitted and heading falls back to "Olá" with no subtitle; timeline failure → `activity` omitted; `moment`/`actions` always render; a `Build` error drops that component and an emptied section is omitted; response is always `200` otherwise. The envelope gains `omitted: [{id, type, reason}]` (reason = failed source name or `build_error`) so Raio-X can draw omitted sections; document it in `specs/http/bff.md` in this change. Responses set `Cache-Control: no-store`. The engine accepts a drop reporter (interface, no-op default) so story 17 can count `sdui_component_dropped_total`. Info logs never contain rendered copy, registration data, or account values.

**Never:** No other moment variant, day change, beta revision, `X-SDUI-Schema`, cases source, investor profile, investir/carteira/perfil screens (later stories; those slugs answer `404` for now). No web change. No new dependency beyond `golang.org/x/sync/errgroup` (already indirect). Unit tests do not dial 5435/5673/8400/8420/9201. Do not read or edit `.env` / `.env.*`. Never a `utils` package.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Thiago home | all sources OK | `200`, `schema_version 1`, `slug home`, `revision v1`, sections moment/wealth/actions/advisor/activity in that order; wealth total "US$ 68.000,00", cash "US$ 60.520,00", shares sum 100% | No error expected |
| Mariana advisor | segment Singular | `advisor_card` `dedicated`, kicker "Sua assessora dedicada", "Resposta em até 1 h · cliente Singular" | — |
| No activity | timeline returns 0 rows | `activity_list` `empty` with the empty text | — |
| Timeline down | Search errors or exceeds deadline | `200`; no `activity` section; `omitted` has `activity`/`activity_list`/`timeline` | not propagated |
| Advisory down | GetCustomer errors | `200`; no `advisor`; title "Olá", no subtitle; `omitted` lists `advisor` | not propagated |
| Account down | GetAccount errors (not NotFound) | `200`; no `wealth`; `omitted` lists `wealth` | not propagated |
| Unknown customer | GetAccount NotFound | `404` | — |
| Bad id / slug | non-v7 id → `400`; slug `investir` or `x` → `404` | — | — |
| Build error | a variant's Build returns error | that component dropped, section omitted, reporter called | response `200` |

</intent-contract>

## Code Map

- `internal/bff/http.go:108-115` -- POV route registration; add `GET /v1/client-pov/customers/{id}/screens/{slug}` here, delegating to `internal/screen`.
- `internal/bff/account.go` -- `grpcPOV` and `accountv1` client; story 4 adds positions/patrimony/registration on `Account`. Expose what the screen source adapter needs.
- `internal/bff/clients.go:18-48` -- `QueueSource.GetCustomer` → `Customer{ID, Name, Segment, AUM, Advisor, Since}`; check whether `Advisor` is a name or an operator id (resolve via `ListOperators` if an id).
- `internal/bff/timeline.go` -- `TimelineClient.Search(ctx, customerID, query, kind)` → `[]TimelineEntry{Kind, Title, Text, Meta, Ago}`; kinds include `aporte`, `saque`, `mensagem`, `caso`, `segmento`, `queda`.
- `internal/bff/pov.go` -- `povCatalog` (per-client SLA text) and `customerSince`; do not remove phase-2 routes.
- `cmd/bff/main.go` -- wiring of queue, timeline, account-sim clients.
- `docs/design/sdui-full-pov/project/OrlaApp.dc.html:749-802` -- design `homeSections` (props, variant selection, omitted handling).
- `specs/http/bff.md` -- Screens section; add `omitted` to the envelope.

## Tasks & Acceptance

**Execution:**
- `internal/screen/page.go` -- `Page`, `Section`, `Component`, `Action`, `Omitted` with JSON tags matching bff.md.
- `internal/screen/snapshot.go` -- source interfaces, `Snapshot` with per-source results, parallel fetch under the deadline.
- `internal/screen/format.go` (+ test) -- money, percent with largest remainder, relative time.
- `internal/screen/catalog.go`, `internal/screen/catalog.json` -- embedded catalog, template execution with named fields.
- `internal/screen/home.go` -- home section variants (`welcome`, wealth `default`, actions `default`, advisor `default`/`dedicated`, activity `recent`/`empty`).
- `internal/screen/engine.go` -- per section: first matching variant, failure policy, omitted list, drop reporter.
- `internal/screen/*_test.go` -- one test per variant (`Matches` and `Build`), a test that parses and executes every catalog template, a screen test per seed client (Fernanda, Thiago, Mariana with fake sources built from the seed values), failure-policy tests for each source and for a `Build` error, deadline test with a slow fake source.
- `internal/bff/screen.go` (+ test) -- source adapters over account-sim, advisory, timeline; the HTTP handler (404/400, `Cache-Control: no-store`, JSON) -- HTTP surface.
- `cmd/bff/main.go` -- wire the engine.
- `specs/http/bff.md` -- document `omitted` and the home v1 sections served now.

**Acceptance Criteria:**
- Given the three seed clients with all sources up, when web requests their home, then each gets one `200` JSON with the five sections and `moment` `welcome`, matching bff.md prop names.
- Given the engine, when copy in `catalog.json` changes, then only Go tests need updating and no web file changes.

## Verification

**Commands:**
- `gofmt -l . && go vet ./... && go build ./...` -- expected: clean.
- `mkdir -p .gotmp && GOTMPDIR=$PWD/.gotmp go test -race -shuffle=on ./...` -- expected: pass.
- `go mod tidy && git diff --exit-code go.mod go.sum` -- expected: clean, or only `golang.org/x/sync` promoted to a direct requirement.

## Auto Run Result

Status: done

- **Summary:** `internal/screen` composes SDUI pages: parallel Snapshot under an 800 ms deadline with per-source errors and panic recovery, per-variant needs with fallback to the default, embedded JSON catalog with text/template copy, Go formatting, failure policy with an `omitted` list, and a drop reporter. The BFF serves `GET /v1/client-pov/customers/{id}/screens/home` (moment welcome, wealth, actions, advisor default/dedicated, activity recent/empty from client-facing timeline rows) with `Cache-Control: no-store`.
- **Files:** `internal/screen/*` (engine, snapshot, catalog, format, home, page + tests); `internal/bff/screen.go` (+ tests), `internal/bff/{http,pov,account,timeline}.go`; `internal/timeline/{index,grpc}.go` and `proto/timeline/v1` (`source`, `occurred_at`); `specs/http/bff.md`; `go.mod` (x/sync direct).
- **Review:** 38 findings; 13 patched (1 high, 5 medium, 7 low), 2 deferred, the rest rejected with evidence.
- **Follow-up review recommended:** true — a high finding (client-facing activity filtering) was patched with a proto change.
- **Verification:** gofmt/vet/build clean; `go test -race -shuffle=on ./...` passes; web 41/41; live smoke: account-sim + BFF against local Postgres returned `200` with the expected sections and `advisor` omitted while advisory was down.
- **Residual risks:** Elasticsearch-backed timeline must persist the new fields; verified only with the in-memory index.
