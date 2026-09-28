---
title: 'Seed POV accounts and accept client commands'
type: 'feature'
created: '2026-09-28'
status: 'done'
review_loop_iteration: 0
baseline_revision: '5c33936029a1599c2f67d978d56d8cedad341972'
followup_review_recommended: false
warnings: []
deferred: []
---

<intent-contract>

## Intent

**Problem:** account-sim publishes seeded market-day rows, but it does not own the three POV balances or accept a client command that either commits state and an outbox event together or refuses with nothing written.

**Approach:** Store Fernanda, Thiago, and Mariana with four class balances in integer cents. A deposit or withdrawal updates caixa and total assets in the same transaction as the schema version 2 outbox row. A withdrawal above caixa rolls back. The same idempotency key returns the original event. Reseed restores the three balances.

## Boundaries & Constraints

**Always:** Customer ids are the advisory book UUIDv7 values. Mariana caixa is 6000000 cents. A withdrawal does not change ações, ETFs, or renda fixa. Complaint and free message publish `message.received` and do not move cash. Refusal leaves no outbox row.

**Never:** BFF HTTP, rate limits, the book update in advisory, or a frontend change.

</intent-contract>

## Auto Run Result

Status: done

Summary: `sim.Apply` and `sim.Reseed` own the POV balances. Tests cover the seed sums, a cash-only withdrawal, a refusal with an empty outbox, an outbox failure that restores the balance, idempotent replay, a complaint that does not move cash, and reseed.

Files changed:
- `internal/sim/account.go` — command transaction
- `internal/sim/account_test.go` — the cases above
- `migrations/account_sim/003_pov_accounts.sql` — balance and idempotency tables
- `seeds/account_sim/002_pov_accounts.sql` — the three starting rows

Verification: `go test ./internal/sim/` passed.

Residual risk: the gRPC server is not bound in `cmd/account-sim` yet. The BFF story calls this command API. This story has no screen.
