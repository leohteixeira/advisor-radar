# Architecture

Phase-3 delta. Anything this file does not change stays as the adopted phase-2 `architecture.md` and phase-1 `architecture.md` define it. Diagrams are in `architecture-diagrams.md`. Section and component detail per screen is in `ux.md`.

## Brownfield starting point

- `cmd/account-sim` only relays the outbox. The POV account logic lives in `internal/sim` with an in-memory store (`sim.Memory`), which `cmd/bff/povmem.go` runs inside the BFF.
- `migrations/account_sim/003_pov_accounts.sql` already defines `pov_account` (four class balances in cents) and `pov_idempotency`, but no pgx adapter implements `sim.Tx`.
- `proto/` has `advisory`, `cases`, `timeline`, and `triage` v1, generated into `gen/` with protoc 5.29.3, protoc-gen-go 1.36.5, and protoc-gen-go-grpc 1.5.1. There is no `account/v1`.
- Every gRPC server is a plain `grpc.NewServer()` with no interceptors. The BFF dials with `grpc.NewClient` and a `*_GRPC_TARGET` env var. Services listen on `*_GRPC_ADDR`.
- `cases.ListCasesRequest` has no fields. `internal/bff/pov.go` returns `map[string]any`. `web/src/screens/ClientAppScreen.tsx` is 1,042 lines. `ClientPov.test.tsx` covers the phase-2 app.

## Phase A: code follows ADR 0008

This lands before any purchase, preference, or screen code.

1. **`proto/account/v1/account.proto`**, generated into `gen/account/v1` with the same toolchain. A checked-in generation script pins protoc 29.3, protoc-gen-go 1.36.5, and protoc-gen-go-grpc 1.5.1 and regenerates every `proto/*/v1`; the plugins live in `~/go/bin`, which is not on `PATH` in the Dev Container. It has RPCs for the four phase-2 commands (deposit, withdrawal, free message, complaint), each carrying `idempotency_key` and returning `event_id` and `replay`. It also has a query for one account and a list of the POV accounts. Amounts are integer USD cents.
2. **account-sim** serves it on `ACCOUNT_SIM_GRPC_ADDR`, over a pgx adapter for `sim.Tx` on `pov_account` and `pov_idempotency`. Account state and the outbox row are written in one transaction, and the existing relay publishes. A refusal maps to a gRPC status: over-cash withdrawal is `FailedPrecondition`, an unknown customer is `NotFound`, and a bad amount or channel is `InvalidArgument`. A replayed key returns the original `event_id` with `replay = true`.
3. **BFF** dials `ACCOUNT_SIM_GRPC_TARGET` with a client interceptor that propagates the request deadline and retries transient failures with backoff. Retry is safe because every command carries `idempotency_key`. Tracing interceptors come with observability. It maps the statuses back to the phase-2 HTTP answers (`202`, `422`, `404`, `400`). `cmd/bff/povmem.go` and the BFF's use of `sim.Memory` are deleted. The phase-2 HTTP contract and `ClientPov.test.tsx` do not change.
4. **Typed DTOs** replace `map[string]any` in the POV responses. This is still the phase-2 JSON shape, now typed.
5. **ADRs.** ADR 0009 records Server-Driven UI in the BFF. ADR 0010 records individual positions and revokes phase-2 decision 9. ADR 0008 gains a note that the code now follows it. The screen contract is written in `specs/http/bff.md`.

Later phases add RPCs to `account/v1`: the product catalog, the purchase, advance-day, and read and update of preferences and beta. They are born on this path, never in the BFF.

## Services

| Service | Phase-3 change |
|---|---|
| account-sim | `account/v1` gRPC server. Positions per product, where the four phase-2 classes become an aggregate of positions plus cash. Global simulated day. Deterministic revaluation of every account on advance. Fictional product catalog. `aplicacao` command (cash to position) with outbox and idempotency. Fictional registration data. Preferences (contact channel) and beta flag. |
| advisory | Investor profile and assessment date in the book, with the max-risk table per profile. New suitability-mismatch rule on `aplicacao` (card kind `perfil`, rule "Compra acima do perfil de investidor", alert only). The book also follows `aplicacao` and `reavaliacao`. The drop rule evaluates negative `reavaliacao`; the segment rule already runs on every account event. A gRPC query returns the client's moment facts. |
| cases | Consumes `message.triaged` through its existing `inbox` and calls `cases.Open` when intent is `reclamacao` or `encerramento` or `churn_risk` ≥ 0.5. Segment and advisor come from advisory `GetCustomer` over gRPC with a deadline. At most one open case per customer; a later message joins it. Nothing called `cases.Open` before this. `ListCases` gains an optional customer filter. |
| timeline-indexer | No contract change. It indexes `aplicacao` and `reavaliacao` like any account event. The BFF reads history through the existing `Search`. |
| bff | `internal/screen` engine and the screen routes. Purchase, preferences, and advance-day routes. Calls account-sim over gRPC. Typed DTOs. The catalog is embedded read-only config; still no database. |
| web | `web/src/sdui/` renderer, home migration, Investir, Carteira, and Perfil routes, the coded purchase form, and the simulation-strip additions. |

## Screen contract

```text
GET /v1/client-pov/customers/{id}/screens/{slug}
    slug ∈ home | investir | carteira | perfil
    optional header  X-SDUI-Schema: <supported schema_version range>
```

```json
{
  "schema_version": 1,
  "slug": "home",
  "revision": "v1",
  "sections": [
    { "id": "moment", "components": [
      { "type": "moment_card", "variant": "idle_cash",
        "props": { "title": "Thiago, 89% do seu patrimônio está em caixa",
                   "tone": "info",
                   "action": { "type": "navigate", "target": "investir" } } } ] }
  ]
}
```

- Section order in the array is the render order.
- Without `X-SDUI-Schema`, the BFF serves its current `schema_version`. When the header's range excludes it, the BFF answers `406 Not Acceptable` with the supported range in the body, and web shows an update notice.
- Action types: `navigate` (another SDUI screen), `panel` (`deposit`, `withdraw`, `message`, `complaint`, `purchase`), `note` (a notice), and `link` (external).
- Values are display strings. A value a form needs as a number is also sent as integer cents.
- Every `type` and its props are documented in `specs/http/bff.md`. The 15 types and their variants are in `ux.md`.

## Composition engine (`internal/screen`)

1. **Types.** `Page`, `Section`, and `Component` mirror the envelope.
2. **Snapshot.** One screen deadline covers these calls, fetched in parallel (`errgroup`):
   - account, positions, preferences, and beta from account-sim;
   - customer, investor profile, and moment facts from advisory;
   - open cases for the customer from cases;
   - recent movements from timeline.

   Each source result carries its own error. One source failing never cancels the others.
3. **Variants.** Each section has an ordered list of variants. The interface is declared in `internal/screen`, the consumer:

   ```go
   type Variant interface {
       Matches(Snapshot) bool
       Build(Snapshot, Catalog) (Component, error)
   }
   ```

   The first variant that matches wins. The last variant is the default and always matches.
4. **Catalog.** One versioned file is embedded with `go:embed`, keyed in English. Copy uses `text/template` with named fields (`{{.FirstName}}`, `{{.AdvisorName}}`, `{{.CashShare}}`). Go formats numbers, money (`US$ 1.234,56`), and dates before they enter a template. A test parses and executes every template.
5. **Failure policy.**
   - A section whose source failed falls back to its default variant if the default does not need that source. Example: `moment` becomes `welcome` when moment facts fail.
   - A section that depends only on the failed source is omitted. Example: `activity` without timeline.
   - A `Build` error drops that component.
   - Every drop increments `sdui_component_dropped_total`. The response is still `200`.

## Home moment priority

The BFF evaluates this list top to bottom. Advisory supplies the facts. The BFF never computes segmentation.

| # | Variant | Matches when |
|---:|---|---|
| 1 | `portfolio_drop` | The latest `reavaliacao` loss is above 15% of patrimony |
| 2 | `case_open` | The client has an open case (cases, filtered by customer) |
| 3 | `segment_upgraded` | A live POV account event (`schema_version` 2 or later) raised a segment upgrade for the client in the last 24 h |
| 4 | `segment_upgrade_near` | Patrimony is between US$ 7,500 and US$ 10,000 |
| 5 | `idle_cash` | Cash is at or above 50% of patrimony |
| 6 | `portfolio_review` | The client is Singular |
| 7 | `welcome` | Default, including when moment facts fail |

Fernanda's USD 10,000 deposit leaves about 61% in cash, so `segment_upgraded` must rank above `idle_cash` for the live change to show. Thiago's seeded upgrade alert is only about 4 h old (`seeds/advisory/001_cast.sql`), which is why seeded upgrades never match: he still opens on `idle_cash`. To lose `idle_cash`, Thiago must take cash below 50%: a purchase above USD 26,520 (the demo uses "Tudo" or USD 30,000).

## Investir and purchase

- The product catalog is owned by account-sim. The highlights are a deterministic pick per profile (seed below).
- A product is **above profile** when its risk exceeds the profile's max risk. Advisory owns the max-risk table. The BFF badge and warning and the advisory alert rule apply that same test.
- `POST /v1/client-pov/customers/{id}/purchases` takes `product_id` and `amount_cents` and requires `Idempotency-Key`. It counts in the phase-2 per-customer limits (10 per 60 s, 40 per 24 h, and a replay is free). It answers `202` with `event_id`, or `422` when the amount exceeds cash.
- account-sim moves cash to the position at the fixed catalog price. It writes position, cash, and outbox in one transaction. A purchase leaves patrimony unchanged.

## Simulated market day

- There is one global `sim_day` in account-sim. It starts at 0 and reseed resets it to 0. On day 0 every position equals its seed value.
- A position's value is a pure function of product and day, with no randomness and no wall clock. Only the scripted shock moves prices: Cobalto Semicondutores falls 53.5% on day 3. Every other product and day is flat.
- `POST /v1/client-pov/simulation/advance-day` requires `Idempotency-Key` and has its own global rate limit of 20 per 10 minutes. It becomes one gRPC command.
- One transaction writes the new day, the revalued positions of every account, and one `reavaliacao` outbox row per account, keyed `reavaliacao:{customer_id}:{sim_day}`. A replay advances nothing and publishes nothing.
- The shock hits every holder of the stock. Each account alerts only past its own 15% threshold, so extra alerts on non-demo accounts are expected and documented with the seed.

## Preferences and beta

- `PUT /v1/client-pov/customers/{id}/preferences` sets `channel` (`chat` or `email`) and `beta`. It is a gRPC call to account-sim and publishes no event. The theme never reaches the server.
- A beta client receives home revision `v2`, which inserts the `highlights` section (`product_rail`, variant per profile, as on Investir) right after `moment`. Everyone else gets `v1`. Carteira, Investir, and Perfil stay at `v1`.

## Events

No new event type. The body envelope and trace headers are unchanged.

| Action | Event | Payload (`schema_version` 3) |
|---|---|---|
| Purchase | `account.event.recorded` | `kind: aplicacao`, `amount`, `before`, `after`, `product_id`, `asset_class`, `risk` |
| Advance day | `account.event.recorded` | `kind: reavaliacao`, `amount` (signed change), `before`, `after` (patrimony), `sim_day` |

Amounts are integer USD cents, as in version 2. Consumers accept versions 1, 2, and 3 and scale by version before a rule runs. The phase-2 commands keep publishing version 2.

## Seed

The same customer ids as phase 2 (ADR 0007). Positions keep each class total of the phase-2 seed, so phase-1 rules fire at the same values.

**Products** (fixed price, fictional):

| id | Name | Class | Risk | Indicative return | Minimum |
|---|---|---|---:|---|---|
| `tbill` | Orla T-Bill 6 meses | renda_fixa | 1 | 4,9% a.a. | US$ 100 |
| `corp` | Orla Corporate IG 2029 | renda_fixa | 2 | 5,6% a.a. | US$ 1.000 |
| `renda` | Maré Renda Global ETF | etfs | 2 | +3,8% em 12 meses | US$ 50 |
| `acoesg` | Maré Ações Globais ETF | etfs | 3 | +11,2% em 12 meses | US$ 50 |
| `farol` | Farol Saúde | acoes | 4 | +9,4% em 12 meses | US$ 10 |
| `cobalto` | Cobalto Semicondutores | acoes | 5 | +27,1% em 12 meses | US$ 10 |

**Investor profiles:**

| Profile | Max risk | Highlights | Seed clients |
|---|---:|---|---|
| conservador | 2 | `tbill`, `corp` | Fernanda Lima, assessed 12/03/2026 |
| moderado | 3 | `acoesg`, `corp` | Mariana Costa, assessed 20/01/2026 |
| arrojado | 5 | `cobalto`, `acoesg` | Thiago Azevedo, assessed 04/08/2026 |

The other 19 book customers get profiles from one fixed-seed draw, written into the advisory seed as literals.

**Day-0 positions** (USD; applied → current):

| Client | Positions | Cash | Patrimony |
|---|---|---:|---:|
| Fernanda | farol 1.590 → 1.640; renda 1.660 → 1.690; acoesg 1.900 → 2.000; tbill 1.690 → 1.722 | 1.148 | 8.200 |
| Thiago | cobalto 1.900 → 2.040; acoesg 5.200 → 5.440 | 60.520 | 68.000 |
| Mariana | cobalto 60.000 → 72.000; farol 17.500 → 18.900; acoesg 36.000 → 40.600; renda 19.400 → 20.000; corp 36.000 → 36.800 | 60.000 | 248.300 |

On day 3 Cobalto drops to 46.5% of its value. That is Mariana −15.5% (drop alert) and Thiago about −1.6% (no alert); Fernanda holds none. The artboards draw this on day 12, which is illustrative only. Registration data (e-mail, masked phone, city, account number, client-since) is in `OrlaApp.dc.html` `clients()`.

## Observability

- Bootstrap: no service has OpenTelemetry code today. Each service gets the OTel SDK with an OTLP exporter configured by `OTEL_EXPORTER_OTLP_ENDPOINT`, and a service name set in code, and Compose adds `grafana/otel-lgtm` (the phase-1 choice) to view traces and metrics.
- Spans: one per screen build and one per section, with `sdui.slug`, `sdui.revision`, `sdui.section`, and `sdui.variant`. Snapshot calls are child spans.
- Metrics: `sdui_variant_served_total{slug,section,variant}`, `sdui_component_dropped_total{slug,type,reason}`, build latency per screen, purchases by class, and suitability-mismatch alerts.
- Logs: `slog` JSON, never rendered copy, registration data, or full account values at `info`.

## Runtime configuration

The repository runs no service container; `deploy/` holds only Postgres. New env vars go wherever services are configured:

| Service | Variable | Purpose |
|---|---|---|
| account-sim | `ACCOUNT_SIM_GRPC_ADDR` | gRPC listen address (Phase A) |
| bff | `ACCOUNT_SIM_GRPC_TARGET` | account-sim gRPC target (Phase A) |
| cases | `ADVISORY_GRPC_TARGET` | advisory target for `GetCustomer`; already in the shared root `.env`, so no new line |
| every service | `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP export (observability story) |

All services load the one root `.env` (`internal/envfile`, no `${VAR}` expansion). So the OTel service name is set in code per command, with `OTEL_SERVICE_NAME` only as a process-level override, never in `.env`. The `grafana/otel-lgtm` Grafana UI must not publish 3000, which is Cybersecurity's web port; the observability story picks a free port and adds it to the repository port table.

## Testing

- **Go.** One test per variant (`Matches` and `Build`). One test executes every catalog template. One screen test per seed client asserts the expected variants. There are failure-policy tests for each Snapshot source and for a `Build` error. The phase-1 rules are re-tested on the positions seed.
- **React.** One test per component type. There are tests for an unknown `type` and for a throwing component. `ClientPov.test.tsx` passes before and after the home migration, and that migration lands before the new screens.
- **Phase A.** The existing bff and account-sim POV tests pass before and after the gRPC move.

## Cut order

If time runs short, cut in this order:

1. The `X-SDUI-Schema` header and the version range. Keep only `schema_version` in the response.
2. Desktop layout of the new screens. The centered phone layout is enough.
3. The scripted shock and the drop alert. Revaluation stays.
4. The beta program in Perfil. The beta segment becomes a fixed list in the catalog.
5. Movement history in Carteira.

Never cut these:

- the server-driven home with the three seed moments;
- isolated component failure;
- purchase with outbox, idempotency, and the suitability-mismatch alert;
- Perfil with the investor profile;
- ADR 0009;
- the `add-sdui-variant` skill.

## Documentation deliverables

- ADR 0009 and ADR 0010, plus the note on ADR 0008.
- The `account/v1` proto.
- Screens and components in `specs/http/bff.md`.
- The phase-3 demo script in the README.
- `add-sdui-variant` documented in its own `SKILL.md`.
- `docs/design/sdui-full-pov/README.md`, kept as the design reference.
