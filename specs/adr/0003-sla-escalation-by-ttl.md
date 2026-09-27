# ADR 0003 — SLA escalation by TTL and dead-letter exchange

- **Status:** accepted
- **Date:** 2026-09-27

## Context

Cases carry a segment SLA that must escalate when the clock expires. A cron scanner would add a
polling path, drift under load, and duplicate the broker's delay semantics.

## Decision

SLA escalation uses a queue with TTL and a dead-letter exchange. When the message expires, the
dead-letter path emits `case.sla.breached`. There is no cron for escalation.

## Consequences

Escalation is driven by the broker's TTL. Demo expiry and production expiry share one mechanism.
Operational lag depends on the broker remaining available.

## Reassessment trigger

Revisit if RabbitMQ is cut or if SLA clocks must survive broker loss without a replay path.
