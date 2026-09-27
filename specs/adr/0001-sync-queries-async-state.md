# ADR 0001 — Synchronous queries versus asynchronous state changes

- **Status:** accepted
- **Date:** 2026-09-27

## Context

Services need to read each other's state and to react when that state changes. Mixing both
concerns on the same transport couples timeouts to fan-out and makes retries hard to reason about.

## Decision

- A read of another service (a customer's book, a case) is gRPC. The caller's deadline propagates.
- A state change is a RabbitMQ event: account event, triaged message, raised alert, case opened or
  moved, SLA breached.
- Creating a case from an alert is an asynchronous command, not a gRPC call in the alert
  transaction.

## Consequences

Query paths stay deadline-bound and fail fast. State changes are durable through the broker and
can be retried independently. Opening a case from an alert does not block the alert transaction.

## Reassessment trigger

Revisit if a synchronous write across services becomes a hard product requirement that cannot be
expressed as a command on the broker.
