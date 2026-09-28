---
title: 'Document the POV contract and the demo script'
type: 'feature'
created: '2026-09-28'
status: 'done'
review_loop_iteration: 0
baseline_revision: 'df49590b37d344f8d00335b726d8847f0639656c'
followup_review_recommended: false
warnings: []
deferred: []
---

<intent-contract>

## Intent

**Problem:** An evaluator cannot read how to run the client view, or the POV HTTP contract, without opening the code.

**Approach:** The README covers selection, the POV, and the demo script. The POV routes sit in the same BFF contract as the team routes. OpenTelemetry is not added.

## Boundaries & Constraints

**Always:** The demo script matches `ux.md`. The contract states integer cents, Idempotency-Key, 202 with event_id, 422, and 429. The queue link is `/advisor-radar/fila`.

**Never:** OpenTelemetry instrumentation, or an import from docs into the web app.

</intent-contract>

## Auto Run Result

Status: done

Summary: `README.md` describes selection, the three seeded clients, and the six-step demo. `specs/http/bff.md` lists the team routes and the POV routes together, and says tracing is not instrumented.

Verification: the two files were read back against `ux.md` and the routes in `internal/bff/http.go`. This story has no screen.
