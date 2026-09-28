---
title: 'Update the book from POV account events'
type: 'feature'
created: '2026-09-28'
status: 'done'
review_loop_iteration: 0
baseline_revision: '83685a6b53849f21bbfee070988225d9056f717a'
followup_review_recommended: false
warnings: []
deferred: []
---

<intent-contract>

## Intent

**Problem:** Schema version 2 account events are converted to dollars before the rules run, but the advisory book still keeps the seeded AUM and segment.

**Approach:** On a first delivery of schema version 2, write `book.aum` and `book.segment` from the dollar `after` amount in the same transaction as the inbox claim. Fernanda's USD 10,000 deposit and Mariana's USD 60,000 withdrawal raise the phase-1 alerts and move the book.

## Boundaries & Constraints

**Always:** Version 1 stays whole dollars and does not rewrite the book. A replay of the same event_id does not write the book again. Segment bounds stay Essencial through 10000, Advance through 200000, Singular above that.

**Never:** A new event name, a BFF route, or a frontend change.

</intent-contract>

## Auto Run Result

Status: done

Summary: A schema version 2 account event updates the dollar book after the inbox claim. Fernanda moves to Advance at 18200 with aporte and segment alerts. Mariana moves to Advance at 188300 with a saque alert. Version 1 does not touch the book. A replay keeps the same book row.

Verification: `go test ./internal/advisory/` passed. This story has no screen.
