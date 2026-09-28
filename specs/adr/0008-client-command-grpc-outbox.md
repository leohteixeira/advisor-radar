# ADR 0008 — Client commands over gRPC with outbox publish

- **Status:** accepted
- **Date:** 2026-09-28

## Context

ADR 0001 keeps synchronous reads on gRPC and asynchronous state changes on
RabbitMQ. A client POV action is a state change, but the BFF has no database and
cannot publish through an outbox. The action must enter the account state owner
before an event is durable on the broker.

## Decision

- The BFF delivers a client action to account-sim over gRPC with the caller's
  deadline.
- account-sim validates, writes the new account state and the outbox event in
  one transaction, and returns `event_id`.
- No new event name. POV facts ride on the existing `account.event.recorded` and
  `message.received` events.
- `schema_version` 1 stays whole USD dollars. On `account.event.recorded`,
  `schema_version` 2 carries integer USD cents plus optional origin or
  destination; consumers convert by version before a rule runs. Message payloads
  stay channel and text.

- This extends ADR 0001. The phase-1 query path is unchanged. The write enters
  through gRPC into the state owner, then becomes an asynchronous event via the
  outbox.

## Consequences

Client actions stay deadline-bound into account-sim and durable through the
transactional outbox. Advisory and other consumers keep the same event names and
continue to evaluate dollar amounts after a version-aware conversion.

## Reassessment trigger

Revisit if the BFF gains durable storage and can own an outbox, or if a
synchronous cross-service write must complete without an event.
