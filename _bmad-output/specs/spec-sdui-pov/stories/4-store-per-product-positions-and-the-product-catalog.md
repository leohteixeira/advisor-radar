---
title: 'Store per-product positions and the product catalog'
type: 'feature'
created: '2026-09-29'
status: 'done'
review_loop_iteration: 0
baseline_revision: '4d93b0119d1aaec804036ef336a2d68e1d5f7257'
followup_review_recommended: true
context:
  - '{project-root}/AGENTS.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/architecture.md'
  - '{project-root}/specs/adr/0010-individual-positions.md'
  - '{project-root}/specs/adr/0007-uuidv7-sql-seed.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/stories/2-serve-account-v1-from-account-sim.md'
warnings: []
deferred:
  - summary: >-
      Migration 004 drops the class columns without backfilling positions; a populated database reads cash-only until cmd/db seed runs.
    evidence: |-
      ADR 0007 has CD run only migrate; the README now says to reseed after 004. A backfill would need a class-to-product mapping that the seed owns.
    location: >-
      migrations/account_sim/004_pov_positions.sql
    severity: medium
  - summary: >-
      Phase-1 advisory rules are not re-tested on the positions seed.
    evidence: |-
      architecture.md Testing lists it; the event before/after values are unchanged and tested here, but advisory tests run in the stories that change advisory (8, 9, 13).
    location: >-
      internal/advisory
    severity: low
  - summary: >-
      Value rounding and overflow are untested because every factor is 1.
    evidence: |-
      priceFactor is 1/1 until story 13 adds the Cobalto shock; story 13 must test rounding with the real factor.
    location: >-
      internal/sim/value.go
    severity: low
---

<intent-contract>

## Intent

**Problem:** account-sim stores four class balances per account and no individual asset, so Carteira positions, purchases, and revaluation (ADR 0010) have nothing to act on, and there is no product catalog or fictional registration data.

**Approach:** Store positions per fictional product plus cash in account-sim, derive the four phase-2 classes as aggregates, seed the six products, the day-0 positions, and the registration data from `architecture.md` "Seed" and `OrlaApp.dc.html` `clients()`, and expose them on `account/v1` — while every phase-2 class total, the phase-2 HTTP contract, and all existing tests stay the same.

## Boundaries & Constraints

**Always:** Load the `golang-how-to` skill first and apply the Go skills in `/workspace/repos/advisor-radar/CLAUDE.md`. A position is `(customer_id, product_id, units_cents, applied_cents)` where `units_cents` is the holding expressed in day-0 cents (ADR 0010 "units"); its market value is `sim.Value(product, units, day)` = `units × factor(product, day)`, a pure Go function with no randomness or wall clock. In this story every product's factor is 1 on every day (the day-3 Cobalto shock is story 13; keep the function shaped so it can add it). The product catalog is seed data (`seeds/account_sim/`), with `id`, `name`, `asset_class` ∈ `renda_fixa|etfs|acoes`, `risk` 1–5, `return_label` (Portuguese display text), `minimum_cents` — exactly the six rows of `architecture.md` "Products". Registration per POV customer: `email` (`@example.com`), `phone` (already masked text), `city`, `account_number` — from `OrlaApp.dc.html` `clients()`; client-since stays in the advisory book. The account aggregate: `acoes`, `etfs`, `renda_fixa` = sum of position market values of that class at the current day; `caixa` = cash; patrimony = positions at market value + cash. Seeded day-0 aggregates equal the phase-2 seed exactly (Fernanda 164000/369000/172200/114800, Thiago 204000/544000/0/6052000, Mariana 9090000/6060000/3680000/6000000 cents). A migration moves `pov_account` to cash-only (drop the three class columns) and adds `pov_product`, `pov_position`, `pov_registration`; deposit and withdrawal keep touching only cash, and their v2 event `before`/`after` equal patrimony before/after. `account/v1` gains: `Account.positions` (`product_id`, `asset_class`, `applied_cents`, `value_cents`), `Account.patrimony_cents`, `ListProducts()` (the catalog), and `GetRegistration(customer_id)`; the existing `acoes_cents`/`etfs_cents`/`renda_fixa_cents`/`caixa_cents` fields keep their numbers and are now aggregates, so the BFF needs no change. Regenerate with `scripts/gen-proto.sh`. `cmd/db seed` restores products, positions, registration, and cash idempotently (reseed). The seed consistency test (`internal/seeds`) keeps passing and covers the new files. Wrap errors with `%w`; propagate `context.Context`.

**Never:** No purchase, advance-day, `reavaliacao`, preferences, or `schema_version` 3 (stories 9, 12, 13). No BFF, web, advisory, or cases change. No real product, ticker, or brand. Unit tests do not dial 5435/5673/8400/8420/9201. Do not read or edit `.env` / `.env.*`.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Seed aggregates | reseeded store, day 0 | each client's class aggregates equal the phase-2 cents; patrimony 820000 / 6800000 / 24830000 | No error expected |
| Positions | `GetAccount` Mariana | five positions (cobalto 6000000→7200000, farol 1750000→1890000, acoesg 3600000→4060000, renda 1940000→2000000, corp 3600000→3680000; applied→value cents) | No error expected |
| Catalog | `ListProducts` | six products with class, risk, return label, minimum | No error expected |
| Registration | `GetRegistration` Thiago | `thiago.azevedo@example.com`, `+55 (48) •••••-2093`, `Florianópolis, SC · Brasil`, `Conta 2847-1 · Orla Invest` | unknown → `NotFound` |
| Deposit after migration | Fernanda +1000000 | cash 1114800; positions untouched; event before 820000, after 1820000 | No error expected |
| Withdrawal | over cash | `FailedPrecondition`; positions untouched | refusal |
| Value function | any product, day 0..n | `units × 1` in this story; deterministic | — |

</intent-contract>

## Code Map

- `internal/sim/account.go` -- `Account` (four class fields + `Assets()`), `POVSeed`, `build` (deposit/withdrawal touch `Caixa`), `Tx` interface. Add `Position`, `Product`, `Registration`, `Value`, aggregate computation; keep `Acoes/ETFs/RendaFixa/Caixa` as computed aggregates so `build` and the BFF adapter keep working.
- `internal/sim/memory.go` -- in-memory store: seed positions, products, registration.
- `internal/sim/pgx.go` -- pgx adapter from story 2 (advisory-lock serialization in `LookupKey`); `GetAccount` must load positions, `PutAccount` writes cash only, `ResetPOV` restores positions and registration too.
- `internal/sim/grpc.go` -- map new fields and RPCs; status mapping unchanged.
- `proto/account/v1/account.proto`, `scripts/gen-proto.sh` -- extend and regenerate.
- `migrations/account_sim/003_pov_accounts.sql` -- current schema; add `004_pov_positions.sql`.
- `seeds/account_sim/002_pov_accounts.sql` -- phase-2 class seed; rewrite for cash, and add products, positions, registration (new file or same file), with `ON CONFLICT DO UPDATE` for reseed.
- `cmd/db/main.go` -- applies migrations and seeds per service.
- `internal/seeds/consistency_test.go` -- checks UUIDv7 ids and cross-file ids.
- `docs/design/sdui-full-pov/project/OrlaApp.dc.html:600-640` -- `clients()` registration and positions (design `v0` = day-0 value).
- `internal/bff/account.go` -- BFF adapter reads the four class fields; must keep working untouched.

## Tasks & Acceptance

**Execution:**
- `migrations/account_sim/004_pov_positions.sql` -- products, positions, registration tables; drop class columns from `pov_account`.
- `seeds/account_sim/002_pov_accounts.sql` (+ new seed file if cleaner) -- cash, six products, day-0 positions, registration; reseed-safe.
- `internal/sim/account.go`, `internal/sim/value.go` -- position/product/registration types, pure `Value`, aggregates and patrimony.
- `internal/sim/memory.go`, `internal/sim/pgx.go` -- stores load and persist positions; `Tx` gains `ListProducts`, `GetRegistration`, and position loading.
- `proto/account/v1/account.proto`, `gen/account/v1/` -- new fields and RPCs via the script.
- `internal/sim/grpc.go` -- serve the new RPCs and fields.
- `internal/sim/*_test.go` -- matrix rows over `sim.Memory` via bufconn and in the gated pgx tests (migrate + seed + assert aggregates); a test that the Go seed and the SQL seed agree.

**Acceptance Criteria:**
- Given the reseeded database, when the BFF calls `GetAccount` for the three clients, then `GET /v1/client-pov/customers/{id}` returns the same JSON as before this story.
- Given the change, when the full Go suite and the gated pgx tests run, then all pass and no BFF, web, advisory, or cases file changed.

## Verification

**Commands:**
- `sh scripts/gen-proto.sh && git status --porcelain gen/` -- expected: only `gen/account/v1` changed.
- `gofmt -l . && go vet ./... && go build ./...` -- expected: clean.
- `mkdir -p .gotmp && GOTMPDIR=$PWD/.gotmp go test -race -shuffle=on ./...` -- expected: pass.
- `ACCOUNT_SIM_TEST_DATABASE_URL='postgres://localdev:localdev@127.0.0.1:5435/account_sim?sslmode=disable' GOTMPDIR=$PWD/.gotmp go test -race -count=1 ./internal/sim/` -- expected: pass (local compose Postgres).
- `go mod tidy && git diff --exit-code go.mod go.sum` -- expected: clean.

## Auto Run Result

Status: done

- **Summary:** account-sim stores cash plus per-product positions (units in day-0 cents, value = units × factor(product, day), factor 1 for now); the four classes and patrimony are aggregates equal to the phase-2 seed. Six fictional products, eleven day-0 positions, and three registrations are seeded; `account/v1` exposes positions, patrimony, `ListProducts`, and `GetRegistration`.
- **Files:** `migrations/account_sim/004_pov_positions.sql`; `seeds/account_sim/002_pov_accounts.sql`, `003_pov_positions.sql`; `internal/sim/{account,value,memory,pgx,grpc}.go`; `proto/account/v1/account.proto` + `gen/account/v1`; `cmd/db/main.go` (seed in one transaction); tests in `internal/sim` and `internal/seeds`; `specs/adr/0010-individual-positions.md`; `README.md`.
- **Review:** 27 findings; 9 patched (2 medium, 7 low), 3 deferred, 15 rejected (2 false).
- **Follow-up review recommended:** true — the read-locking and single-transaction seed patches are medium and touch concurrency.
- **Verification:** gofmt/vet/build/tidy clean; `go test -race -shuffle=on ./...` passes; gated pgx tests pass; `cmd/db migrate` + `seed` applied cleanly to the local compose Postgres.
- **Residual risks:** deploys must reseed after 004.
