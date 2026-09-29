---
title: 'Build the Perfil screen with preferences and beta'
type: 'feature'
created: '2026-09-29'
status: 'done'
review_loop_iteration: 0
baseline_revision: '870e81b'
followup_review_recommended: false
context:
  - '{project-root}/AGENTS.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/ux.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/architecture.md'
  - '{project-root}/specs/http/bff.md'
  - '{project-root}/specs/adr/0008-client-command-grpc-outbox.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/stories/11-build-the-carteira-screen-with-movement-history.md'
warnings: []
deferred:
  - summary: >-
      Web Perfil fixtures are hand-built copies of the BFF envelope.
    evidence: |-
      Carried from stories 6, 10 and 11; no shared contract test ties web/src/test/perfilFixtures.ts to Go output.
    severity: medium
---

<intent-contract>

## Intent

**Problem:** The Perfil tab is a placeholder. The client cannot see their registration data or investor profile, and cannot choose a contact channel or join the beta program. account-sim stores neither preference, so nothing could persist them.

**Approach:**
- account-sim stores `channel` (`chat` | `email`) and `beta` per POV customer. They are read and written over `account/v1` and publish no event.
- The BFF adds `PUT /v1/client-pov/customers/{id}/preferences`.
- The BFF serves `GET …/screens/perfil` (`v1`), 100% SDUI, with these sections:
  - `header` (`profile_header`)
  - `suitability` (`profile_scale`, variant = profile)
  - `registration` (`profile_field_list`)
  - `preferences` (`preference_list`)
  - `advisor` (`advisor_card`)
- Web registers the new types and renders the Perfil route. The channel and beta controls call the route and then reload the screen, matching `Perfil-Fernanda`, `Perfil-Thiago-Claro`, and `Perfil-Desktop`.

## Boundaries & Constraints

**Always:**
- **Skills.** Load `golang-how-to` first and apply the Go skills in `/workspace/repos/advisor-radar/CLAUDE.md`.
- **account-sim storage.**
  - Migration `migrations/account_sim/005_pov_preferences.sql` adds a `pov_preferences(customer_id PK, channel TEXT NOT NULL CHECK (channel IN ('chat','email')), beta BOOLEAN NOT NULL)` table. It could instead add columns on `pov_account`; pick the cleaner option and justify it in one line.
  - The seed sets every POV client to `chat` and `false`, and reseed restores them.
- **`account/v1` RPCs.**
  - `GetPreferences(customer_id)` returns `{channel, beta}`.
  - `UpdatePreferences(customer_id, channel, beta)` returns `{channel, beta}`.
  - Unknown customer → `NotFound`. Invalid channel → `InvalidArgument`.
  - No outbox row and no event.
  - Both the memory and pgx stores implement them, serialized with the existing per-customer lock.
  - Regenerate with `scripts/gen-proto.sh`.
- **BFF preferences route.**
  - `PUT /v1/client-pov/customers/{id}/preferences` takes JSON `{channel, beta}`, both required.
  - Responses:
    - `200 {channel, beta}` on success.
    - `400` for a bad id.
    - `404` for an unknown customer.
    - `422 {"error":"invalid"}` for a bad body or channel.
    - `502` when the upstream fails.
  - It does not require `Idempotency-Key`: it is an idempotent PUT.
  - It counts against the per-customer limits like other POV writes, but a PUT that changes nothing is free.
  - Document it in `specs/http/bff.md`.
- **Perfil sections.**
  - `header`: `initials`, `name`, `subtitle` "Cliente {{segment}} desde {{since}}", and `account` (registration `account_number`).
  - `suitability`, variant = profile:
    - `title` "Seu perfil de investidor"
    - `subtitle` "Define os destaques de Investir e quando uma compra recebe aviso."
    - `current_label` "Seu perfil"
    - `levels` for conservador, moderado and arrojado, each `{key, label, description, limit "Produtos até risco {{max}}", max_risk, current}`. `max_risk` comes from advisory `GetInvestorProfile` for the client's own level and from the same advisory table for the others. Add a `max_risk` table field to the RPC response if needed so the BFF never hardcodes it.
    - `footer` "Última avaliação em {{dd/mm/yyyy}}. Para refazer o questionário, fale com a sua assessora."
    - Descriptions come from `ux.md` "Perfil".
  - `registration`: `title` "Dados cadastrais" and `fields` Nome, E-mail, Telefone, Cidade, Segmento, Cliente desde, plus a `footnote` saying the data is fictional. The fields come from `GetRegistration` plus the advisory customer.
  - `preferences`:
    - `title` "Preferências"
    - `theme` `{label "Tema", hint "Fica salvo só neste navegador"}`
    - `channel` `{label "Canal preferido", hint "Por onde a assessoria fala com você", value, options [{chat,"Chat"},{email,"E-mail"}]}`
    - `beta` `{label "Programa beta", hint, enabled}`. When off, the hint is "Veja antes as novas versões das telas."; when on, it is "Ligado. Você recebe a revision v2 do início antes dos outros clientes."
  - `advisor`: reuse the home variants `default` and `dedicated`.
  - **Heading.** "Perfil" / "Seus dados, seu perfil de investidor e suas preferências".
- **Failure policy.**
  - Advisory down: `header`, `suitability` and `advisor` are omitted. `registration` drops only Segmento and Cliente desde; if that is awkward, omit it.
  - Registration down: `registration` is omitted, and `header` renders without `account`.
  - Preferences down: `preferences` is omitted.
  - Profile down: `suitability` is omitted.
  - `perfil` now answers `200`.
  - One test per variant. Seed-client screen tests for the three seed clients.
- **Web.**
  - Register `profile_header`, `profile_scale`, `profile_field_list` and `preference_list`.
  - The Perfil route renders the envelope.
  - The theme control toggles the existing local theme; the server never sees it.
  - The channel options and the beta switch call `PUT …/preferences` and then re-fetch the screen.
  - On failure, show `role="alert"` "Não foi possível salvar. Tente de novo." and keep the previous value.
  - Controls are 44 px targets and keyboard accessible: radio group for channel, `role="switch"` for beta.
  - Coverage of `src/sdui/**` stays at 100%.
  - `ClientPov.test.tsx` keeps passing; its tab-note expectations change only if Perfil no longer shows the note.
  - Visual check against `Perfil-Fernanda` (dark), `Perfil-Thiago-Claro` (light) and `Perfil-Desktop`, recorded.

**Never:**
- No beta home `v2` rendering (story 16 serves `v2`; this story only stores the flag).
- No event for preferences.
- No questionnaire.
- Web holds no max-risk table and no copy beyond coded controls.
- Unit tests do not dial 5435/5673/8400/8420/9201.
- Do not read or edit `.env` / `.env.*`.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Fernanda Perfil | conservador, assessed 2026-03-12 | suitability current = conservador, "Última avaliação em 12/03/2026…"; registration e-mail `…@example.com`; preferences chat/off | — |
| Set channel | PUT `{channel:"email", beta:false}` | `200`; screen reload shows E-mail selected; no event published | — |
| Beta on | PUT `{channel:"chat", beta:true}` | `200`; hint "Ligado. Você recebe a revision v2…" | — |
| Invalid channel | `{channel:"sms"}` | `422 invalid` | — |
| Unknown customer | valid v7 id not in POV | `404` | — |
| Reseed | after changes | chat/false again | — |
| Registration down | GetRegistration fails | `registration` omitted; header without account | — |
| Advisory down | GetCustomer fails | header, suitability, advisor omitted | — |

</intent-contract>

## Code Map

- `internal/sim/{account,memory,pgx,grpc}.go`, `migrations/account_sim/`, `seeds/account_sim/`, `proto/account/v1/account.proto`, `scripts/gen-proto.sh` -- preferences storage and RPCs.
- `internal/bff/{pov,account,http,screen}.go` -- POV route style, limits, adapter.
- `internal/screen/*`, `internal/screen/catalog.json` -- sources (registration, preferences, profile, customer), Perfil variants.
- `internal/advisory/grpc.go` -- `GetInvestorProfile` / max-risk table (story 8).
- `web/src/sdui/*`, `web/src/screens/ClientAppScreen.tsx` (theme storage) -- components, route.
- `docs/design/sdui-full-pov/project/{OrlaApp,Perfil-Fernanda,Perfil-Thiago-Claro,Perfil-Desktop}.dc.html` -- `profileSections` at `OrlaApp.dc.html:890`.

## Tasks & Acceptance

**Execution:**
- account-sim: migration, seed, RPCs, and tests, including gated pgx.
- BFF: preferences route, Perfil sections, failure policy, docs, and tests.
- Web: four components, route, controls, and tests at 100% coverage.

**Acceptance Criteria:**
- Given the local stack, when Fernanda opens Perfil and switches the channel to E-mail and beta on, then the values persist across reload and no event appears in Bastidores.
- Given the change, when the Go suite, the gated pgx tests, `pnpm --dir web test`, and `pnpm --dir web build` run, then all pass.

## Verification

**Commands:**
- `sh scripts/gen-proto.sh && git status --porcelain gen/` -- expected: only account (and advisory if extended) changed.
- `gofmt -l . && go vet ./... && go build ./...` -- expected: clean.
- `mkdir -p .gotmp && GOTMPDIR=$PWD/.gotmp go test -race -shuffle=on ./...` -- expected: pass.
- `ACCOUNT_SIM_TEST_DATABASE_URL='postgres://localdev:localdev@127.0.0.1:5435/account_sim?sslmode=disable' GOTMPDIR=$PWD/.gotmp go test -race -count=1 ./internal/sim/` -- expected: pass.
- `go mod tidy && git diff --exit-code go.mod go.sum` -- expected: clean.
- `pnpm --dir web test && pnpm --dir web build` -- expected: pass; coverage thresholds met.

## Review Triage Log

### 2026-09-29 — Review pass
- verdicts: 24 findings — high 0, medium 2, low 20, false 0, maybe-false 2
- findings:
  - `[low]` `reject` Beta hint promises a v2 home and uses "revision" — the copy is fixed verbatim by the spec; story 16 serves v2.
  - `[low]` `patch` bff.md contradicts itself on beta v2 — says story 16 serves it.
  - `[low]` `patch` Preference writes are an event-less exception with no ADR note — note added to ADR 0008.
  - `[low]` `reject` Stored channel has no consumer — the spec stores it for the screen only.
  - `[low]` `reject` Preference changes spend the command budget; arrow keys PUT per step — the spec requires the per-customer limits; native radio behavior.
  - `[low]` `patch` A second change while saving is dropped silently — controls disabled and aria-busy while saving.
  - `[low]` `reject` One save-failure message for every status — the spec fixes the message.
  - `[low]` `reject` A saved change followed by a failed reload shows the screen error — rare, and retry recovers.
  - `[medium]` `patch` Memory and pgx differ for a missing preferences row, and zero-value seeds are stored invalid — defaults in both stores.
  - `[low]` `reject` profileScale overrides the table row with the profile's own max_risk — both come from advisory's same table.
  - `[low]` `reject` profile_scale copy duplicated per level — the catalog has one copy block per variant by design.
  - `[low]` `patch` Theme button announces state without pressed semantics — aria-pressed added.
  - `[low]` `reject` PUT body decoding accepts unknown fields — same decoder as the other POV routes.
  - `[low]` `reject` No audit trail for channel changes — out of the MVP; no event by spec.
  - `[low]` `reject` Concurrent PUTs can lose an update through the BFF's read-then-write — needs two simultaneous saves from one client; the fix needs a new RPC field.
  - `[low]` `reject` A deadline after commit shows failure with the old value — the spec says keep the previous value on failure.
  - `[low]` `reject` (dup of 16) The stale value stays until reload — same reason.
  - `[low]` `patch` Pending choice survives a reload with the same stored key — cleared on every new envelope.
  - `[maybe-false]` `reject` Old advisory without max_risk_table drops suitability — single-repository deploy ships both together; would only be low.
  - `[low]` `patch` Empty Since renders an empty "Cliente desde" row — row skipped.
  - `[medium]` `patch` (dup of 9) Memory zero-value preferences — same fix.
  - `[low]` `patch` (dup of 9) Zero-value seed in ResetPOV — same fix.
  - `[low]` `patch` The BFF's gRPC mapping of max_risk_table is untested — assertion and a Perfil request over real advisory gRPC.
  - `[maybe-false]` `patch` Intent: the pgx path does not assert "no outbox row" — assertion added in the gated pgx test.

## Auto Run Result

**Summary:**
- **account-sim.** Stores `channel` and `beta` per POV customer in a new `pov_preferences` table.
  - Migration 005 and seed 004 set chat/false; reseed restores them, and a zero-value seed falls back to the defaults.
  - Two new RPCs, `GetPreferences` and `UpdatePreferences`, work in both stores under the per-customer lock. They write no outbox row and publish no event.
- **advisory.** `GetInvestorProfile` now also returns the max-risk table.
- **BFF.**
  - Adds `PUT /v1/client-pov/customers/{id}/preferences`. A no-op PUT is free; a real change spends the per-customer budget.
  - Serves Perfil `v1` with `header`, `suitability`, `registration`, `preferences` and `advisor`, and the failure policy.
- **Web.**
  - Registers `profile_header`, `profile_scale`, `profile_field_list` and `preference_list`.
  - The channel is a radio group and beta is a `role="switch"`. A change calls the PUT and then reloads the screen; on failure it shows an alert and keeps the previous value.
  - The theme control stays local to the browser.
  - The tab note is removed, since every tab now has a screen.
- **Docs.** ADR 0008 has a note recording that preference writes are the event-less exception.

**Files:**
- `internal/sim/{preferences,memory,pgx,grpc,account}.go`
- `migrations/account_sim/005_pov_preferences.sql`, `seeds/account_sim/004_pov_preferences.sql`
- `proto/account/v1`, `proto/advisory/v1` + gen
- `internal/advisory/{grpc,profile}.go`
- `internal/bff/{preferences,account,clients,screen,http}.go`
- `internal/screen/{perfil,catalog,snapshot,home}.go`, `catalog.json`
- `web/src/sdui/components/{ProfileHeader,ProfileScale,ProfileFieldList,PreferenceList}.tsx`, `registry`, `types`
- `web/src/screens/ClientAppScreen.tsx`, `app.css`
- tests, fixtures
- `specs/http/bff.md`, ADR 0008, README

**Review:** 24 findings. 10 patched (1 distinct medium, which accounts for 2 medium rows once duplicates are counted, plus 8 low). 1 deferred item recorded: fixture drift. 14 rejected, with the reasons in the triage log.

**Followup review recommended:** no. Only one distinct medium entry was patched.

**Verification:**
- gofmt, vet and build are clean.
- `go test -race -shuffle=on ./...` passes.
- The gated pgx tests for `internal/sim` and `internal/advisory` pass.
- `go mod tidy` is clean.
- gen-proto changes only `account/v1` and `advisory/v1`.
- web 311/311 passes with 100% coverage of `src/sdui/**`, and the build passes.

**Visual and live check:** before the review patches. Screenshots are in the session scratchpad under `s12/`.
- Perfil for Fernanda (dark), Thiago (light) and desktop matches the `Perfil-Fernanda`, `Perfil-Thiago-Claro` and `Perfil-Desktop` artboards; one light-theme accent fix was made.
- Controls are at least 44 px.
- Live: Fernanda switched to E-mail with beta on; both persisted across reload, and the outbox row count was unchanged.

**Residual risks:**
- Two concurrent PUTs from one client can report a stale no-op.
- Web fixtures are hand-built.
- The beta flag has no effect until story 16.
