---
title: 'Serve POV commands from the BFF'
type: 'feature'
created: '2026-09-28'
status: 'done'
review_loop_iteration: 0
baseline_revision: '20982c30cb5db3ecf59babf7675b93956c80645a'
followup_review_recommended: false
warnings: []
deferred: []
---

<intent-contract>

## Intent

**Problem:** The client view has no versioned HTTP surface. A deposit, withdrawal, message, or complaint cannot return 202, 422, or 429, and nothing counts actions, refusals, or idempotent duplicates.

**Approach:** The BFF exposes the POV reads and posts. Every post requires Idempotency-Key. A new command spends a budget of 10 per minute and 40 per day per customer. A replay does not. There is no live progress stream in this story.

## Boundaries & Constraints

**Always:** Money on the wire is integer cents. 202 carries event_id and no protocol field. A withdrawal above caixa is 422 and publishes nothing through the port. Over the limit is 429 and does not call the port. Counters are actions by type, refusals by rule, and duplicate replays.

**Never:** The SSE hub, the phone UI, or an import from docs.

</intent-contract>

## Auto Run Result

Status: done

Summary: `/v1/client-pov/customers` lists the three clients. Posts for deposit, withdrawal, message, and complaint return 202 with event_id, 422, or 429. A missing Idempotency-Key is 400. Replays stay 202 after the minute cap. `GET /v1/client-pov/counters` reports the three counts.

Verification: `go test ./internal/bff/` passed. This story has no screen. The running process still needs an account-sim adapter behind `POVSource`; tests use a fake port.
