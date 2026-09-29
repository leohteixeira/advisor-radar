---
title: 'Serve account/v1 from account-sim'
type: 'feature'
created: '2026-09-28'
status: 'done'
review_loop_iteration: 0
baseline_revision: 'c490e098f27c396eb93c6bfb8e4e7da95d3227f8'
followup_review_recommended: true
context:
  - '{project-root}/AGENTS.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/architecture.md'
  - '{project-root}/specs/adr/0008-client-command-grpc-outbox.md'
  - '{project-root}/specs/adr/0010-individual-positions.md'
warnings: []
deferred:
  - summary: >-
      An idempotency key reused with a different command or amount is returned as a replay of the first event.
    evidence: |-
      pov_idempotency stores only (customer_id, idem_key) -> event_id; sim.Apply behaved the same in phase 2 with sim.Memory, so it is pre-existing.
    location: >-
      internal/sim/account.go
    severity: medium
  - summary: >-
      cmd/account-sim run wiring (gRPC + relay, shutdown) has no process test with a database.
    evidence: |-
      internal/proc/commands_test.go starts account-sim with no env; a DSN-gated process test needs the CI Postgres now added for the pgx tests.
    location: >-
      cmd/account-sim/main.go
    severity: medium
  - summary: >-
      The lefthook pre-push hook still skips the gated pgx tests, and nothing checks gen/ against the protos.
    evidence: |-
      CI now runs TestPGXStore_* with a Postgres service; the local hook has no database. A regen-diff check is a CI addition outside this story.
    location: >-
      lefthook.yml
    severity: low
  - summary: >-
      Port 8460 for account-sim gRPC is not in the repository port table in CLAUDE.md/AGENTS.md.
    evidence: |-
      README lists it; editing agent-context files is deferred by the review rules.
    location: >-
      CLAUDE.md
    severity: low
  - summary: >-
      Whitespace-only message text is accepted.
    evidence: |-
      sim.build checks only == "" (pre-existing phase-2 behavior).
    location: >-
      internal/sim/account.go
    severity: low
---

<intent-contract>

## Intent

**Problem:** ADR 0008 says client commands reach account-sim over gRPC, but there is no `account/v1` proto, account-sim only relays the outbox, and `sim.Tx` has no PostgreSQL adapter, so the POV logic can only run in memory inside the BFF.

**Approach:** Add `proto/account/v1/account.proto` with a checked-in generation script that pins the toolchain, implement `sim.Store`/`sim.Tx` over pgx on `pov_account`, `pov_idempotency`, and `outbox`, and serve the gRPC service from `cmd/account-sim` on `ACCOUNT_SIM_GRPC_ADDR`, mapping `sim` refusals to gRPC statuses. The BFF is not changed in this story (story 3 moves it).

## Boundaries & Constraints

**Always:** Load the `golang-how-to` skill first and apply the Go skills listed in `/workspace/repos/advisor-radar/CLAUDE.md` before writing Go. The script (`scripts/gen-proto.sh`, POSIX sh or bash with `set -eu`) prepends `$HOME/go/bin` to `PATH`, fails with a clear message unless `protoc --version` is `libprotoc 29.3`, `protoc-gen-go --version` is `v1.36.5`, and `protoc-gen-go-grpc --version` is `1.5.1`, then regenerates every `proto/*/v1/*.proto` into `gen/` with `paths=source_relative`-equivalent output matching the existing `gen/` layout; rerunning it leaves the existing `gen/{advisory,cases,timeline,triage}` byte-identical. The proto has RPCs `Deposit`, `Withdraw`, `SendMessage`, `FileComplaint` (each request carries `customer_id`, `idempotency_key`, and its fields: `amount_cents`+`origin`, `amount_cents`+`destination`, `channel`+`text`, `text`; each reply is `CommandReply{event_id, replay}`), `GetAccount(customer_id) → Account`, and `ListAccounts() → {repeated Account}`; `Account` carries `customer_id` and the phase-2 class balances as int64 cents (`acoes_cents`, `etfs_cents`, `renda_fixa_cents`, `caixa_cents`). Status mapping: `sim.ErrInsufficient` → `FailedPrecondition`; `sim.ErrUnknownCustomer` → `NotFound`; `sim.ErrAmount`, `sim.ErrCommand`, `sim.ErrKey`, a non-UUIDv7 customer id → `InvalidArgument`; anything else → `Internal` with no SQL or DSN text in the message. A replayed key returns the original `event_id` with `replay = true` and writes nothing. Account state, idempotency key, and outbox row commit in one pgx transaction; the existing relay (`outbox.RunPublisher`) publishes. Concurrent commands for one customer are serialized inside the transaction (e.g. `SELECT … FOR UPDATE` on the account row or a transaction-scoped advisory lock taken before the key lookup) so two concurrent requests with the same key yield one event and the second sees `replay = true`. The gRPC server starts only when `ACCOUNT_SIM_GRPC_ADDR` and `ACCOUNT_SIM_DATABASE_URL` are set; it runs alongside the relay, and SIGINT/SIGTERM stop both gracefully (`GracefulStop`). With no env, account-sim still logs `service=account-sim` and exits 0 on signal (`internal/proc/commands_test.go`). Wrap errors with `%w`, pass `context.Context` to every I/O call, and log no DSN or payload.

**Never:** Do not touch the BFF, `cmd/bff/povmem.go`, or web. Do not add positions, products, or new commands (stories 4, 9, 12, 13). Do not read or edit any `.env` or `.env.*` file. Unit tests must not dial ports 5435, 5673, 8400, 8420, or 9201. Do not add gRPC interceptors on the server (tracing comes with observability).

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Deposit | Fernanda, 1000000 cents, origin, new key | `event_id` UUIDv7, `replay=false`; caixa +1000000; one v2 `account.event.recorded` outbox row | No error expected |
| Replay | same key again | same `event_id`, `replay=true`, no second outbox row, balance unchanged | No error expected |
| Over-cash withdrawal | amount > caixa | no row written | `FailedPrecondition` |
| Unknown customer | valid UUIDv7 not seeded | no row | `NotFound` |
| Bad input | amount ≤ 0, missing key, bad channel, empty text, non-v7 id | no row | `InvalidArgument` |
| Message / complaint | chat/e-mail + text | `message.received` v1 outbox row, balance unchanged | No error expected |
| Query | `GetAccount` Mariana; `ListAccounts` | the seeded balances; three accounts | unknown → `NotFound` |
| Store failure | pgx error mid-transaction | rollback, nothing persisted | `Internal`, no SQL text |

</intent-contract>

## Code Map

- `internal/sim/account.go` -- `Tx`, `Store`, `Apply`, `Reseed`, errors, `POVSeed`, `build`; reuse unchanged (small additions allowed only if the adapter needs them).
- `internal/sim/memory.go` -- in-memory `Store` used by tests; keep.
- `internal/outbox/pgx.go` -- pattern for a pgx `WithTx` (Begin, deferred Rollback, Commit) and the outbox insert SQL (`event_id` is `UUID`).
- `migrations/account_sim/001–003` -- `outbox(event_id UUID)`, `pov_account(customer_id TEXT, acoes, etfs, renda_fixa, caixa BIGINT)`, `pov_idempotency(customer_id, idem_key, event_id TEXT)`.
- `cmd/account-sim/main.go` -- currently waits when DSN or broker is unset; restructure so gRPC and relay run concurrently, each optional.
- `cmd/advisory/main.go:106-125` -- house pattern for `net.Listen` + `grpc.NewServer()` + serve goroutine + `GracefulStop` on shutdown.
- `internal/advisory/grpc.go`, `internal/cases/grpc.go` -- house pattern for a gRPC server type in the owning package and `status.Error` mapping.
- `proto/cases/v1/cases.proto`, `gen/cases/v1/*.pb.go` -- proto style (`option go_package = "github.com/leohteixeira/advisor-radar/gen/<svc>/v1;<svc>v1"`) and generated headers (`protoc v5.29.3`, `protoc-gen-go v1.36.5`, `protoc-gen-go-grpc v1.5.1`, `source: proto/<svc>/v1/<svc>.proto`).
- `internal/proc/commands_test.go` -- starts every command with no env; must keep passing.
- `README.md` -- no configuration section yet.

## Tasks & Acceptance

**Execution:**
- `scripts/gen-proto.sh` -- pinned generation script as in Always -- reproducible `gen/`.
- `proto/account/v1/account.proto`, `gen/account/v1/` -- the service and messages, generated by the script -- the ADR 0008 contract.
- `internal/sim/pgx.go` -- `PGXStore` implementing `sim.Store`/`sim.Tx` (`GetAccount`, `PutAccount`, `LookupKey`, `SaveKey`, `InsertOutbox`, `ResetPOV`) with per-customer serialization -- durable state and outbox in one transaction.
- `internal/sim/grpc.go` -- `GRPCServer` implementing `accountv1.AccountServiceServer` over a `sim.Store`, with the status mapping -- the command and query surface.
- `internal/sim/grpc_test.go` -- bufconn tests over `sim.Memory` for every matrix row, asserting codes, replay, and outbox rows -- matrix coverage without ports.
- `internal/sim/pgx_test.go` -- integration test gated on `ACCOUNT_SIM_TEST_DATABASE_URL` (skipped when unset) that migrates a throwaway schema or uses the given database, seeds, and asserts deposit, replay, over-cash rollback, and a concurrent same-key pair yielding one outbox row -- real Postgres semantics.
- `cmd/account-sim/main.go` -- start gRPC on `ACCOUNT_SIM_GRPC_ADDR` next to the relay with graceful shutdown -- the server runs.
- `README.md` -- add a short "Configuration" section listing `ACCOUNT_SIM_GRPC_ADDR` (e.g. `0.0.0.0:8460`) and `ACCOUNT_SIM_GRPC_TARGET` (e.g. `127.0.0.1:8460`, used by the BFF from story 3), noting both belong in the local `.env`, plus `scripts/gen-proto.sh` -- discoverable config.

**Acceptance Criteria:**
- Given the pinned toolchain, when `scripts/gen-proto.sh` runs twice, then `git status --porcelain gen/` shows only the new `gen/account/v1` files and the second run changes nothing.
- Given account-sim with DSN and `ACCOUNT_SIM_GRPC_ADDR`, when a client calls `Deposit`, then the account row and outbox row are committed together and the relay publishes the event.
- Given account-sim with no env, when it receives SIGTERM, then it exits 0 having logged `service=account-sim`.

## Design Notes

Serialization golden example inside the pgx `LookupKey` (runs first in `sim.Apply`):

```go
// Serialize commands per customer so a concurrent same-key request becomes a replay.
if _, err := t.tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, customerID); err != nil {
    return "", false, fmt.Errorf("sim pgx: lock customer: %w", err)
}
```

`pov_account.customer_id` and `pov_idempotency.event_id` are TEXT; `outbox.event_id` is UUID — cast in SQL, never format ids by hand.

## Verification

**Commands:**
- `sh scripts/gen-proto.sh && git status --porcelain gen/` -- expected: only `gen/account/v1/` new.
- `gofmt -l . && go vet ./... && go build ./...` -- expected: no output, success.
- `mkdir -p .gotmp && GOTMPDIR=$PWD/.gotmp go test -race -shuffle=on ./...` -- expected: all pass.
- `go mod tidy && git diff --exit-code go.mod go.sum` -- expected: clean.

## Auto Run Result

Status: done

- **Summary:** account-sim serves `account/v1` gRPC (Deposit, Withdraw, SendMessage, FileComplaint, GetAccount, ListAccounts) over a pgx `sim.Store` that writes account, idempotency key, and outbox in one transaction, serialized per customer with a namespaced advisory lock. Refusals map to FailedPrecondition, NotFound, InvalidArgument, Canceled, DeadlineExceeded, or Internal. A pinned `scripts/gen-proto.sh` regenerates every proto.
- **Files:** `proto/account/v1/account.proto`, `gen/account/v1/*` (generated); `scripts/gen-proto.sh` (pinned toolchain); `internal/sim/pgx.go` (pgx adapter); `internal/sim/grpc.go` (server and status mapping); `internal/sim/account.go` (MaxAmountCents); `internal/sim/grpc_test.go`, `internal/sim/pgx_test.go` (bufconn and Postgres tests); `cmd/account-sim/main.go` (gRPC next to relay, ping, graceful stop); `.github/workflows/ci.yml` (Postgres service for pgx tests); `README.md` (configuration).
- **Review:** 20 findings; 9 patched (3 medium, 6 low), 6 deferred, 5 rejected (1 false).
- **Follow-up review recommended:** true — three medium patches (context-error mapping, amount bound, CI Postgres) changed behavior and CI; the CI service has not run on GitHub yet.
- **Verification:** gofmt/vet/build clean; go mod tidy clean; `go test -race -shuffle=on ./...` passes; pgx tests pass against local Postgres; gen-proto rerun leaves only `gen/account/v1` new.
- **Residual risks:** CI Postgres job untested until pushed; process wiring covered only manually.
