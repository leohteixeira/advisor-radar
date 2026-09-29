---
name: add-sdui-variant
description: Add a new variant of an existing Server-Driven UI section in the Advisor Radar BFF (for example a new home moment_card moment) — the catalog.json entry and copy, the Variant implementation and its tests, the priority placement, and the specs/http/bff.md contract rows — so the variant ships by rebuilding and restarting only the BFF. Use when asked to add or create an SDUI variant, a home moment, or a new card for an existing section. Not for a new component type (that also needs web code), a new moment fact (that belongs to the service that owns it), or a beta-only variant (every home revision carries the same variants).
---

# Add an SDUI variant

A variant is one way to fill an existing section of an SDUI screen. The screen engine in
`internal/screen` walks a section's variants in catalog order and builds the first one whose
`Matches` is true; the last variant is the default and always matches (ADR 0009). A new variant
of an existing component type needs no web change and ships with a BFF rebuild and restart.

Work in `internal/screen` (package `screen`) and `specs/http/bff.md`. Load the Go skills
`AGENTS.md` requires before writing Go.

## Inputs

Collect these before you start. Ask for any that is missing; do not guess the fact or the copy.

| Input | Example (`cash_only`) | Notes |
|---|---|---|
| Screen and section id | `home` / `moment` | The section fixes the component `type` (`moment_card`). |
| Variant name | `cash_only` | English snake_case, unique within the type under `copy`. |
| Fact it matches on | the account has no positions | A value already in the `Snapshot`, read as-is. |
| Data source | account-sim (`SourceAccount`); no new source | Say whether a new Snapshot source or field is needed. |
| Copy with field names | title `{{if .FirstName}}{{.FirstName}}, sua{{else}}Sua{{end}} conta só tem caixa` | Portuguese. One text per prop key. |
| Tone, icon, action | `info`, `cash`, `navigate` → `investir` | From the sets the type accepts. |
| Priority slot | just above `welcome` | Position in the section's variant list. |

## Guardrails

- Portuguese copy lives only in `internal/screen/catalog.json`. Go code, identifiers, keys,
  tests, and the contract prose are English.
- Web never formats or computes: Go formats money, percentages, dates, and counts before they
  enter a template.
- `Matches` compares no business threshold and computes no segmentation. A condition such as
  "cash ≥ 50%" is a fact the owning service evaluates (advisory for moments, cases for open
  cases); the BFF only reads the boolean or value it receives.
- No `utils` or `helpers` package. Keep helpers next to the variant in `internal/screen`.
- Never read or edit `.env` or `.env.*`. Configure a local run with exported variables.

## Where things are

- `internal/screen/catalog.json` — embedded with `go:embed` in `catalog.go`. Layout:
  - `screens.<slug>.revision` (served by default), optional `beta_revision` (served when the
    customer's stored beta flag is on), and `revisions.<rev>` with `title`, `subtitle`, and
    `sections: [{id, type, variants: [...]}]`. The variant list is the evaluation order; the last
    entry is the default.
  - `copy.<type>.<variant>.<key>`: one `text/template` per prop text. Templates execute against
    `screen.Fields` with `missingkey=error`.
  - `segments.<segment>.sla`: the SLA text `Catalog.SLA` returns.
  - Home has two revisions: `v1` and the beta `v2`, which is `v1` with the `highlights` section
    inserted after `moment`. `TestCatalog_HomeBetaRevision` fails unless every home section,
    variants included, is identical in both.
- `internal/screen/engine.go` — `Variant` (`Matches(Snapshot) bool`,
  `Build(Snapshot, Catalog) (Component, error)`), the `registered{variant, needs, uses}` entry,
  `resolve` (a catalog name without an implementation fails `New`), and `section`, the failure
  policy:
  - a variant whose `needs` source failed is skipped, and evaluation continues down the list;
  - a section whose default needs a failed source is omitted with that source as the reason;
  - a `Build` error or panic drops the whole section with reason `build_error`; it does **not**
    fall through to the next variant;
  - a `Build` that returns `errAbsent` leaves the section out of both `sections` and `omitted`,
    with no reason and no fall-through (a section absent by design, such as a Carteira class with
    no position);
  - the built component's `Type` and `Variant` must equal the section type and catalog name.
- `internal/screen/home.go` — `builtinVariants(cat)`, the registry of every
  `(type, variant) → registered`, merging `momentVariants`, `investirVariants`,
  `carteiraVariants`, and `perfilVariants`; `customerFields(s)` (the advisory fields
  `FirstName`, `AdvisorName`, `Segment`, `Since`, empty when advisory failed).
- `internal/screen/moment.go` — the `moment_card` variants, their name constants in priority
  order, `momentVariants(cat)` with each variant's `needs` and `uses`, helpers `hasAdvisor`,
  `hasSLA`, and `momentComponent`. Other types live in `home.go`, `investir.go`, `carteira.go`,
  `perfil.go`, and `daychange.go`.
- `internal/screen/snapshot.go` — `Snapshot` and its `Fetched[T]` results (`.OK()`, `.Value`,
  `.Err`), the `Source` constants (`SourceAccount`, `SourceAdvisory`, `SourceTimeline`,
  `SourceMoments`, `SourceProfile`, `SourceCases`, `SourceCatalog`, `SourceRegistration`,
  `SourcePreferences`), and the fact types: `Account` (class totals, `Cash`, `Patrimony`,
  `Positions`, `SimDay`, `DayChange`), `Customer`, `Activity`, `MomentFacts`,
  `InvestorProfile`, `OpenCase`, `Product`; `Registration` and `Preferences` are in `perfil.go`.
- `internal/screen/catalog.go` — `Fields` (every template field, each formatted text),
  `Catalog.Text`, `Catalog.SLA`, and `copier{cat, typ, variant, fields}` whose `text(key)` keeps
  the first error in `cp.err`.
- `internal/screen/format.go` — the formatters: `Money`, `SignedMoney`, `CompactMoney`,
  `Percent`, `ChangePercent`, `PercentBP`, `ChangePercentBP`, `SignTone`, `Shares` (largest
  remainder), `RelativeTime`, `Days`, `Protocol`, `FirstName`, `Initials`.
- `internal/screen/page.go` — the envelope and props structs (`MomentCard`, `WealthSummary`, …),
  tones (`TonePos`, `ToneNeg`, `ToneInfo`, `ToneGold`, `ToneNeutral`), action types
  (`ActionNavigate`, `ActionPanel`, `ActionNote`, `ActionLink`), panels (`PanelDeposit`,
  `PanelWithdraw`, `PanelMessage`, `PanelComplaint`, `PanelPurchase`), and screens
  (`ScreenInvestir`, `ScreenCarteira`).
- Test helpers: fixtures `fernandaFixture`, `thiagoFixture`, `marianaFixture` with `.snapshot()`
  and `.sources()`, fakes `fakeAccounts`, `fakeMoments`, … and `errDown` (`snapshot_test.go`);
  `props[T]`, `build` (`home_test.go`); `assertMoment`, `withCase`, `fernandaUpgraded`
  (`moment_test.go`); `newTestEngine`, `variantOf`, `sectionIDs` (`engine_test.go`); `embedded`
  (`catalog_test.go`).
- `internal/bff/screen.go` — the adapters from gRPC to the Snapshot ports and `getScreen`, the
  route `GET /v1/client-pov/customers/{id}/screens/{slug}`.
- `web/src/sdui/registry.tsx` — the component registry, keyed by `type`.

## Steps

### 1. Read the contract and the catalog

1. Read `specs/http/bff.md` "Screens (phase 3)": the failure policy, the priority table of the
   section (for home `moment`, "Home moment priority"), the "served now" table of the screen, and
   the component row of the type with its accepted tones and props.
2. Read the section in `catalog.json` in **every** revision of the screen that has it, and the
   existing `copy.<type>` entries.
3. Check the fact. If the `Snapshot` does not carry it, or it would take a comparison against a
   business threshold, stop: first add the fact to the owning service, its proto, the adapter in
   `internal/bff/screen.go`, and the Snapshot (port, `Sources`, `allSources`, `failed`,
   `fetchSnapshot`, the `omitted` reason list in `bff.md`). That is a cross-service change with
   its own story, not this skill.
4. Check shadowing. List every variant above the chosen slot and ask whether its fact already
   holds whenever yours does. If it does, your variant shows only when that one's source fails
   or its extra conditions do not hold; move the slot, or accept it and say so in the contract.
   Example: for a slot just above `welcome` matching "no position", an account with some cash is
   100% cash, so advisory also reports `idle_cash`; an all-cash Essencial client between
   US$ 7.500 and US$ 10.000 also gets `segment_upgrade_near`; and a Singular client gets
   `portfolio_review`. The last two outrank the slot even when `idle_cash` is skipped (moments
   or profile down), so such a variant shows only for an empty non-Singular account or when those
   variants are skipped.

### 2. Add the catalog entry

1. Insert the variant name in the section's `variants` list at its slot, in every revision that
   has the section (home: `v1` and `v2`), and in every screen that mirrors the section (Investir
   `highlights` and home `v2` `highlights` carry the same list). Never after the default. The
   profile-keyed sections (Investir and home `v2` `highlights`, Perfil `suitability`) have no
   catch-all default: an unmatched profile is a `build_error`, and
   `TestEngine_EverySectionEndsInDefault` exempts them.
2. Add `copy.<type>.<variant>` with one key per prop text (`moment_card`: `kicker`, `title`,
   `body`, and `meta` or `action` when used). Use Go field names from `Fields`
   (`{{.FirstName}}`, `{{.AdvisorName}}`, `{{.Cash}}`, …). The contract writes the same fields in
   snake case (`{{first}}`, `{{advisor}}`, `{{cash}}`).
3. Guard every advisory field that can be empty, so the text reads well when advisory failed:
   `{{if .FirstName}}{{.FirstName}}, sua{{else}}Sua{{end}} conta …`.
4. When the copy needs a value `Fields` lacks, add a documented field to `Fields` in
   `catalog.go` and set it in the `full` value of `TestCatalog_EveryTemplateExecutes`.

### 3. Implement the Variant

In the file of the type (`moment.go` for `moment_card`):

1. Add the name constant in priority order, for example `momentCashOnly = "cash_only"`.
2. Add a stateless struct with value receivers (the engine calls it concurrently).
3. `Matches` checks `.OK()` on every source it reads, then reads the fact only. It must also hold
   every precondition `Build` needs (advisor present with `hasAdvisor`, SLA present with
   `hasSLA`, product named, …): a `Build` error drops the section instead of falling through.
4. `Build` re-checks its sources and returns an error when one is missing, starts from
   `customerFields(s)`, sets the extra fields with the formatters from `format.go`, reads the copy
   with `copier{cat: c, typ: typeMomentCard, variant: momentX, fields: f}`, and fills the props
   struct from `page.go` with a tone and action constant. Return through
   `momentComponent(momentX, props, cp.err)` for a moment, or check `cp.err` and return
   `Component{Type: …, Variant: …, Props: …}`.
5. Register it in `momentVariants` (or `builtinVariants`, `investirVariants`, …):
   `{typeMomentCard, momentX}: {variant: xMoment{}, needs: []Source{…}, uses: []Source{…}}`.
   Every source `Matches` or `Build` reads must be in `needs` or `uses`: the engine fetches only
   the sources a plan lists, so an unlisted source stays failed (`errNotFetched`) and the variant
   never matches. `needs` lists the sources the variant cannot do without, so a failure skips the
   variant and the section falls through to the next one. `uses` lists optional sources `Build`
   reads only when they answered. A `needs` source the screen did not read before becomes
   required, so its failure is now logged as a screen failure; a `uses` source is fetched but its
   failure is never a screen failure.

### 4. Write the tests

In the type's `_test.go` (`moment_test.go` for moments). Use table-driven cases where they fit
and `t.Parallel()` like the neighbours.

1. `Test<Name>Moment`: `Matches` is true on a fixture snapshot and false when the fact does not
   hold and when each needed source is down (`Fetched[T]{Err: errDown}`); `Build` returns the
   exact props (`props[MomentCard](t, build(t, v, snap), typeMomentCard, momentX)` and
   `assertMoment`), the form without the name when advisory is down, and an error when a needed
   source is down.
2. Priority: update `TestMomentVariants_Registry` (the expected order and the `needs` map). Add
   `TestHome_Moment` rows that build the whole home with `newTestEngine`: the new variant wins
   below its slot, a variant above it still wins, and failure fall-through (the new variant's
   source down lands on the next variant).
3. For a section other than `moment`, update the pinned variant lists in
   `TestParseCatalog_Embedded`.
4. `TestCatalog_EveryTemplateExecutes` parses and executes the new copy automatically, and
   `TestEngine_EverySectionEndsInDefault` and `TestCatalog_HomeBetaRevision` keep the list shape
   honest. Seed-client tests (`TestEngine_Build_SeedClients`,
   `TestEngine_Build_HomeRevisionPerSeedClient`, `internal/bff/moment_test.go`) change only if a
   seed client now gets the new variant; that changes the demo, so say so.
5. The `internal/bff` handler tests build screens through hand-written fakes (for example
   `thiagoAccount` in `screen_test.go`, a `bff.POVAccount` with class totals). A fake that omits
   data your fact reads can start matching the new variant. When such a test fails, make the
   fake realistic (give it the positions, cases, or facts the real client has); change the
   expected variant only when the real client would get it too.

### 5. Update the contract

In `specs/http/bff.md`:

1. The section's priority table (home: "Home moment priority") — insert the row, renumber, and
   name the fact and who evaluates it. Update the tone sentence under it.
2. The screen's "served now" table: the variants cell, and the Source cell when the variant
   reads a source the row does not list yet. Add a bullet with the copy in contract field names,
   tone, icon, action, sources, and when it is skipped. When the title uses `{{first}}`, add its
   form without the name to "Every `{{first}}` title has a form without the name …".
3. The "Failure policy" bullets that list what can still match when a source fails. A moment
   that does not need the moment facts joins "When the moment facts fail, only `case_open` and
   `welcome` can match." A moment whose copy names the advisor joins "When the advisory customer
   read fails, the moments whose copy names the advisor … do not match".
4. The "Components" row of the type: the variants cell, and any prop note that names variants.
5. A seed-client sentence, if a seed client's variant changed.

### 6. Check web

- An existing `type` needs no rendering change. Confirm the tone is in the type's accepted set
  (the component's `TONES`) and the icon is in `web/src/sdui/icons.tsx`; an unknown icon renders
  nothing, an unknown tone renders neutral.
- A new home moment is also documented in `web/src/selection/sdui.ts` `MOMENT_RULES` (the
  selection showcase's copy of the priority table, pinned by
  `web/src/screens/SelectionScreen.test.tsx`). Without a row the showcase says "Sem linha
  documentada…" for it and the "N de 7" ranks go stale. That is a web release, separate from the
  BFF-only restart; do it in the same change only when asked, updating the rank strings that
  test pins, and otherwise record it as a follow-up.
- A new `type` is out of scope: it needs a component in `web/src/sdui/registry.tsx` and a row in
  the component table.

### 7. Run the Go gate

From the repository root:

```bash
lefthook run pre-push      # the whole CI gate, gitleaks first; or run the steps below
gitleaks git --redact --no-banner   # as the pre-push hook does, with the pinned 8.30.1
mkdir -p .gotmp
go mod tidy && git diff --exit-code go.mod go.sum
test -z "$(gofmt -l .)"
go vet ./...
go build ./...
GOTMPDIR=$PWD/.gotmp go test -race -shuffle=on ./...
```

When you touched web, also run `pnpm --dir web test` and `pnpm --dir web build`.

### 8. Restart only the BFF and verify

Leave account-sim, advisory, cases, and timeline running. Rebuild and restart the BFF alone; the
catalog is embedded with `go:embed`, so restarting an old binary serves the old catalog. The BFF
loads `.env` from its working directory and never overrides a variable already exported, so
start it the way it ran before (same directory or same exported variables). `bin/` is
gitignored.

Find customer ids with
`curl -sf http://127.0.0.1:8400/v1/client-pov/customers | jq -r '.items[] | "\(.customer_id) \(.name) \(.segment)"'`.

```bash
CUSTOMER=<customer-id>
BFF=http://127.0.0.1:8400
SCREEN=$BFF/v1/client-pov/customers/$CUSTOMER/screens/home
MOMENT='.revision, (.sections[] | select(.id == "moment") | .components[0] | {variant, props}), .omitted'

# --fail-with-body exits non-zero and prints the body on a non-2xx answer.
curl -sS --fail-with-body -H 'X-SDUI-Schema: 1' "$SCREEN" | jq -c "$MOMENT"   # before: the old variant
go build -o bin/bff ./cmd/bff
# stop only the running BFF (Ctrl-C in its terminal, or kill its pid), then:
./bin/bff
curl -sS --fail-with-body -H 'X-SDUI-Schema: 1' "$SCREEN" | jq -c "$MOMENT"   # after: the new variant
```

`.omitted` must not list the section: a `build_error` there means `Build` failed (step 3.3).

Pick a customer whose Snapshot holds the fact and no fact above the slot (step 1.4). When no
seed client does, prove the priority with `TestHome_Moment` rows (step 4.2) and do the live
check on throwaway databases and processes, as the `cash_only` proof run did:

1. Create databases on the local Postgres (for example `s15_account_sim`, `s15_advisory`,
   `s15_triage`, `s15_cases`), export `ACCOUNT_SIM_DATABASE_URL`, `ADVISORY_DATABASE_URL`,
   `TRIAGE_DATABASE_URL`, and `CASES_DATABASE_URL` pointing at them, and run
   `go run ./cmd/db migrate` and `go run ./cmd/db seed`.
2. Change only throwaway rows so the fact holds. The proof run emptied Fernanda's account:
   `DELETE FROM pov_position WHERE customer_id = '<id>'` and
   `UPDATE pov_account SET caixa = 0 WHERE customer_id = '<id>'`. The class totals are
   aggregates of `pov_position` (migration `004_pov_positions.sql` dropped the class columns),
   so no other column needs zeroing.
3. Start every throwaway process from a directory with no `.env` (the proof run used the
   scratchpad), so nothing is inherited from the shared configuration. If you start them from
   the repository root instead, also export each variable below that you do not want as an empty
   string (`ACCOUNT_SIM_BROKER_URL=`, `ADVISORY_BROKER_URL=`, `ADVISORY_HTTP_ADDR=`, …): an
   exported empty value blocks the `.env` one. Without a broker URL, account-sim does not relay
   its outbox and advisory consumes nothing, so the shared RabbitMQ is untouched.
   - account-sim: `ACCOUNT_SIM_DATABASE_URL` (throwaway) and `ACCOUNT_SIM_GRPC_ADDR` on a free
     port (`127.0.0.1:29160`). No `ACCOUNT_SIM_BROKER_URL`.
   - advisory: `ADVISORY_DATABASE_URL` (throwaway), `ADVISORY_GRPC_ADDR` on a free port
     (`127.0.0.1:29161`), and `ACCOUNT_SIM_GRPC_TARGET=127.0.0.1:29160`. No
     `ADVISORY_BROKER_URL` and no `ADVISORY_HTTP_ADDR` (it would clash with the running
     advisory).
   - BFF: a second one on a free port, `BFF_HTTP_ADDR=127.0.0.1:29100`, with
     `ACCOUNT_SIM_GRPC_TARGET=127.0.0.1:29160` and `ADVISORY_GRPC_TARGET=127.0.0.1:29161`. The
     running BFF on 8400 keeps its original targets, so there is nothing to restore. Start the
     old binary first, curl, stop it, start the rebuilt one, and curl again with
     `BFF=http://127.0.0.1:29100`.
   - cases and timeline stay as configured. The proof run left `CASES_GRPC_TARGET` and
     `TIMELINE_GRPC_TARGET` unset, so the throwaway BFF read no open case and an empty timeline.
     If you export them to the running services instead, those reads are shared (read-only), and
     a real open case for the customer outranks most moments.
4. If you repointed the running BFF instead of starting a second one, restart it afterwards with
   its original `ACCOUNT_SIM_GRPC_TARGET` and `ADVISORY_GRPC_TARGET`.
5. Stop the throwaway processes and drop the throwaway databases.

Never edit the seed files or the shared databases for a check.

For a home variant, also check the beta revision. The preferences `PUT` replaces both fields and
spends the customer's POV budget when it changes something (`429` over it), so keep the stored
channel and check the status:

```bash
PREFS=$(curl -sS --fail-with-body -H 'X-SDUI-Schema: 1' "$BFF/v1/client-pov/customers/$CUSTOMER/screens/perfil" \
  | jq -c '.sections[] | select(.id == "preferences") | .components[0].props')
CHANNEL=$(jq -r '.channel.value' <<<"$PREFS")   # the stored channel, sent back unchanged
BETA=$(jq -r '.beta.enabled' <<<"$PREFS")       # the stored beta flag, restored at the end
curl -sS -o /dev/null -w '%{http_code}\n' -X PUT -H 'Content-Type: application/json' \
  -d "{\"channel\":\"$CHANNEL\",\"beta\":true}" "$BFF/v1/client-pov/customers/$CUSTOMER/preferences"   # expect 200
curl -sS --fail-with-body -H 'X-SDUI-Schema: 1' "$SCREEN" | jq -c "$MOMENT"   # expect "v2" and the same moment
curl -sS -o /dev/null -w '%{http_code}\n' -X PUT -H 'Content-Type: application/json' \
  -d "{\"channel\":\"$CHANNEL\",\"beta\":$BETA}" "$BFF/v1/client-pov/customers/$CUSTOMER/preferences"   # restore; expect 200
```

On a `429`, wait for the budget window and retry; the beta check is not done until the restore
answers `200`.

## Done when

- `catalog.json` names the variant at its slot in every revision and every screen with the
  section, with its copy.
- The `Variant` is registered with every source it reads in `needs` or `uses`, and covered by
  `Matches`, `Build`, priority, and fall-through tests.
- The `internal/bff` handler tests pass without changing an expected variant only to fit a fake
  (step 4.5).
- `specs/http/bff.md` lists it in the priority, served-now, failure-policy, and component rows.
- The web `MOMENT_RULES` row for a new home moment is done or recorded as a follow-up (step 6).
- The Go gate passes and the restarted BFF serves the variant for a matching customer, in `v1`
  and, for a home variant, in the beta `v2`.
