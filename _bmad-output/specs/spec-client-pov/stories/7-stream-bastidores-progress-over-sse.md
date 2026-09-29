---
title: 'Stream Bastidores progress over SSE'
type: 'feature'
created: '2026-09-28'
status: 'done'
review_loop_iteration: 0
baseline_revision: 'b407f95'
followup_review_recommended: false
warnings: []
deferred: []
---

<intent-contract>

## Intent

**Problem:** The confirmation shows the protocol and the event id, but it does not move through outbox, broker, triage or rules, and the queue.

**Approach:** An in-memory hub on `GET /v1/client-pov/customers/{id}/stream` marks the outbox step when the command returns, then advances from `account.event.recorded`, `message.received`, `message.triaged`, and `alert.raised`. The phone confirmation renders those steps.

## Boundaries & Constraints

**Always:** The hub stores nothing durable. A deposit uses the advisory labels. A complaint uses the triage labels. Steps read feito, agora, or aguardando.

**Never:** OpenTelemetry, a desktop layout, or an import from docs.

</intent-contract>

## Auto Run Result

Status: done

Summary: The BFF hub marks outbox on 202, published on the account or message event, and evaluated plus queue on `alert.raised` or `message.triaged`. The phone confirmation lists the steps from the SSE event `bastidores`.

Verification: `go test ./internal/bff/` passed. `pnpm test` passed (40 tests). The live browser stream was not exercised because the BFF process was not running; the step text was asserted in the phone test with a fake EventSource.
