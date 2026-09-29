---
title: 'Move BFF POV commands onto account-sim gRPC'
type: 'refactor'
created: '2026-09-29'
status: 'done'
review_loop_iteration: 0
baseline_revision: '6ad630c07e9fabec6511b162343b5c7fc56bca4b'
followup_review_recommended: true
context:
  - '{project-root}/AGENTS.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/architecture.md'
  - '{project-root}/specs/adr/0008-client-command-grpc-outbox.md'
  - '{project-root}/specs/http/bff.md'
  - '{project-root}/_bmad-output/specs/spec-sdui-pov/stories/2-serve-account-v1-from-account-sim.md'
warnings: []
deferred:
  - summary: >-
      With ACCOUNT_SIM_GRPC_TARGET unset, the POV list still answers 200 with zero-asset clients while home is 404.
    evidence: |-
      listPOV fills missing accounts with zero values; changing it would alter internal/bff/pov_test.go expectations, which this story must keep unchanged. A startup warning now signals the state.
    location: >-
      internal/bff/pov.go
    severity: low
  - summary: >-
      No process-level test covers the BFF wiring to account-sim and Bastidores via the account-sim relay.
    evidence: |-
      All new tests run the handler over bufconn; cmd/bff/main.go wiring and the relay-to-Bastidores path are exercised only manually.
    location: >-
      cmd/bff/main.go
    severity: medium
---

<intent-contract>

## Intent

**Problem:** The BFF still runs account-sim logic in process (`cmd/bff/povmem.go` over `sim.Memory`) and publishes the POV outbox itself, contradicting ADR 0008, and its POV responses are untyped `map[string]any`.

**Approach:** Replace the in-process store with a `bff.POVSource` adapter over the `account/v1` gRPC client (story 2), dialed on `ACCOUNT_SIM_GRPC_TARGET` with a client interceptor that propagates the request deadline and retries transient failures with backoff; delete `povmem.go` and the BFF's POV publishing; type the POV response DTOs with the same JSON; add the "code now follows it" note to ADR 0008. The phase-2 HTTP contract, `internal/bff/pov_test.go`, and `web/src/screens/ClientPov.test.tsx` do not change.

## Boundaries & Constraints

**Always:** Load the `golang-how-to` skill first and apply the Go skills listed in `/workspace/repos/advisor-radar/CLAUDE.md` before writing Go. The adapter maps gRPC codes back to the existing `sim` sentinel errors so `postPOV`/`getPOV` keep their exact HTTP mapping: `FailedPrecondition` → `sim.ErrInsufficient` (422 `insufficient`), `NotFound` → `sim.ErrUnknownCustomer` (404 on GET, 422 `invalid` on commands, as today), `InvalidArgument` → wrapped `sim.ErrCommand` (422 `invalid`); any other code is an upstream failure (502), and nothing is logged with payloads. Deadline: every call carries the incoming HTTP request context; when that context has no deadline the interceptor applies a default (2 s). Retry: only `codes.Unavailable`, at most 3 attempts total, exponential backoff with jitter starting at 50 ms, stopping when the context is done; retry is safe because every command carries `idempotency_key`. DTOs are named Go structs with JSON tags that reproduce the current field names, types, and order-independent content exactly (`items[]` of `customer_id, name, segment, assets, sla, advisor, since, hint`; home `customer_id, name, segment, advisor, sla, since, assets, caixa, allocation{acoes, etfs, renda_fixa, caixa}, activity: [], messages: []`), and `activity`/`messages` still serialize as `[]`, never `null`. When `ACCOUNT_SIM_GRPC_TARGET` is unset the BFF keeps the existing `emptyPOV` behavior. The BFF no longer publishes anything; the Bastidores hub still observes broker events as before. Wrap errors with `%w`; pass `context.Context` everywhere.

**Never:** Do not change routes, status codes, rate limits, counters, Bastidores steps, `specs/http/bff.md` POV tables, `internal/bff/pov_test.go`, or any web file. Do not add tracing interceptors (observability story). Do not add positions or screens. Do not import `sim.Memory` from BFF code. Unit tests must not dial ports 5435, 5673, 8400, 8420, or 9201. Do not read or edit `.env` / `.env.*`.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Deposit via gRPC | POST deposit with key, account-sim OK | `202 {"event_id"}`, counters `actions.deposit` +1 | No error expected |
| Replay | same key, account-sim `replay=true` | `202` same `event_id`, `duplicates` +1, no budget spent | No error expected |
| Over-cash | account-sim `FailedPrecondition` | `422 {"error":"insufficient"}` | refusal counted |
| Invalid / unknown on command | `InvalidArgument` or `NotFound` | `422 {"error":"invalid"}` | refusal counted |
| Unknown on GET | `GetAccount` → `NotFound` | `404` | No retry |
| Transient | first two calls `Unavailable`, third OK | `202` after retries | backoff honors context |
| Persistent outage | all attempts `Unavailable` / other code | `502` | no budget spent |
| Deadline | HTTP ctx deadline set | the server handler sees a deadline ≤ the caller's | default 2 s when absent |
| List / home JSON | three seeded accounts | byte-for-byte same JSON keys and values as before the change | — |

</intent-contract>

## Code Map

- `cmd/bff/povmem.go` -- delete.
- `cmd/bff/main.go:104-131` -- builds `newMemoryPOV()`, a POV `amqpPublisher`, and a flush ticker; `:233` publish helper for the POV outbox; `:338` `server.ObservePOV` stays. Replace with dialing `ACCOUNT_SIM_GRPC_TARGET` (like `ADVISORY_GRPC_TARGET` at `:67-73`, conn closed in `cleanups`).
- `internal/bff/pov.go` -- `POVSource` port (`List`, `Get`, `Apply`), `POVAccount`, `POVCommand`, `POVResult`, `emptyPOV`, `listPOV`/`getPOV` building `map[string]any`, `postPOV` error mapping. Keep the port; type the DTOs.
- `internal/bff/clients.go:294` -- `DialGRPC(target)`; add an account-sim dial with the interceptor chain here or in a new `internal/bff/account.go`.
- `internal/bff/pov_test.go` -- fake `POVSource` returning `sim` errors; must pass unchanged.
- `internal/sim/grpc.go` -- `sim.NewGRPCServer(store, logger)` and its code mapping; reuse over `sim.NewMemory()` in BFF tests via bufconn (test-only use of `sim.Memory` is allowed).
- `gen/account/v1` -- client `accountv1.NewAccountServiceClient`.
- `specs/adr/0008-client-command-grpc-outbox.md` -- append the note.
- `web/src/screens/ClientPov.test.tsx` -- must pass unchanged.

## Tasks & Acceptance

**Execution:**
- `internal/bff/account.go` -- `grpcPOV` implementing `POVSource` over `accountv1.AccountServiceClient` with the code→sentinel mapping, plus `DialAccountSim(target)` wiring a unary client interceptor chain (default deadline, `Unavailable` retry with jittered backoff) -- ADR 0008 path.
- `internal/bff/account_test.go` -- bufconn tests: the full HTTP handler over `grpcPOV` over `sim.NewGRPCServer(sim.NewMemory(), …)` for every matrix row; interceptor tests with a stub server (Unavailable twice then OK; always Unavailable → 502 after 3 attempts; deadline present server-side; default deadline applied) -- matrix coverage.
- `internal/bff/pov.go` -- replace `map[string]any` in `listPOV`/`getPOV` with typed DTOs; add a golden JSON test asserting the exact encoded keys and values for list and home -- typed contract.
- `cmd/bff/main.go`, `cmd/bff/povmem.go` -- dial `ACCOUNT_SIM_GRPC_TARGET`, delete `povmem.go`, remove the POV publisher and flush ticker -- BFF stores and publishes nothing.
- `specs/adr/0008-client-command-grpc-outbox.md` -- append "Note (2026-09-29): the code now follows this ADR" stating the BFF calls account-sim over `account/v1` with deadline and retry and no longer runs `sim.Memory` or publishes -- CAP-12.

**Acceptance Criteria:**
- Given the change, when `git ls-files cmd/bff` is listed, then `povmem.go` is absent and `grep -rn "sim.NewMemory\|sim.Memory" cmd internal/bff --include='*.go' | grep -v _test.go` is empty.
- Given `internal/bff/pov_test.go` and `web/src/screens/ClientPov.test.tsx` unchanged, when the Go and web suites run, then both pass.

## Verification

**Commands:**
- `gofmt -l . && go vet ./... && go build ./...` -- expected: clean.
- `mkdir -p .gotmp && GOTMPDIR=$PWD/.gotmp go test -race -shuffle=on ./...` -- expected: all pass.
- `go mod tidy && git diff --exit-code go.mod go.sum` -- expected: clean.
- `pnpm --dir web test` -- expected: pass, `ClientPov.test.tsx` included.
- `git diff --stat HEAD -- internal/bff/pov_test.go web/` -- expected: empty.

## Auto Run Result

Status: done

- **Summary:** The BFF reaches account-sim over `account/v1` gRPC with a 2 s default deadline and `Unavailable`-only jittered retry; `cmd/bff/povmem.go` and BFF publishing are gone; POV list/home responses are typed DTOs with byte-identical JSON; ADR 0008 carries the "code now follows it" note.
- **Files:** `internal/bff/account.go` (adapter, dial, interceptors); `internal/bff/pov.go` (typed DTOs, budget rules, Retried handling, warn logs); `internal/bff/http.go` (logger); `cmd/bff/main.go` (dial, warn, no publisher); `cmd/bff/povmem.go` (deleted); `internal/bff/account_test.go`, `internal/bff/account_internal_test.go` (tests); `specs/adr/0008-client-command-grpc-outbox.md` (note); `README.md` (target requirement).
- **Review:** 22 findings; 12 patched (5 medium, 7 low), 2 deferred, 8 rejected (1 false).
- **Follow-up review recommended:** true — five medium patches changed budget and counting semantics under retries.
- **Verification:** gofmt/vet/build clean; tidy clean; `go test -race -shuffle=on ./...` passes; `pnpm --dir web test` 41/41; `pov_test.go` and `web/` unchanged.
- **Residual risks:** local POV demo now requires account-sim running; process wiring tested manually only.
