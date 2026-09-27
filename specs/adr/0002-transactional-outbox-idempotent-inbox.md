# ADR 0002 — Transactional outbox and idempotent inbox

- **Status:** accepted
- **Date:** 2026-09-27

## Context

Publishers write local state and emit events. Consumers may receive the same delivery more than
once. Without a shared pattern, dual-write races and duplicate side effects appear under restart
and broker redelivery.

## Decision

- Every publisher writes events through a transactional outbox in the same transaction as the
  local write.
- Every consumer uses an idempotent inbox keyed by `event_id`; a second delivery is a no-op.
- Each queue has backoff retries and its own dead-letter queue.

## Consequences

Local writes and publishes stay consistent. Consumers are safe under at-least-once delivery.
Failed messages land in a per-queue dead-letter queue for inspection instead of looping forever.

## Reassessment trigger

Revisit if the MVP adopts an exactly-once broker or drops RabbitMQ.
