# Architecture diagrams

The visitor-facing walkthrough copy is in `ux.md`. These diagrams are the command and the fan-out. The complaint call is `POST /v1/client-pov/customers/{id}/complaints`.

## Accepted client action

```mermaid
sequenceDiagram
  participant Web
  participant BFF
  participant AccountSim as account-sim
  participant RMQ as RabbitMQ
  Web->>BFF: POST /v1/client-pov/customers/{id}/… + Idempotency-Key
  BFF->>AccountSim: gRPC command, deadline propagated
  AccountSim->>AccountSim: validate, write state and outbox in one transaction
  AccountSim-->>BFF: event_id
  BFF-->>Web: 202 Accepted, event_id
  AccountSim->>RMQ: relay existing event, trace on headers
  Web->>BFF: GET /v1/client-pov/customers/{id}/stream
  RMQ-->>BFF: account.event.recorded, message.received, message.triaged, alert.raised
  BFF-->>Web: SSE progress for that customer
```

A withdrawal above available cash stops inside account-sim. No outbox row, no `202`. The BFF returns `422`.

A second POST with the same `Idempotency-Key` returns the first result and does not publish again.

## After the 202

No new event name. Who consumes each event stays the phase-1 list in the adopted architecture. These are the two demo paths.

```mermaid
flowchart LR
  acct[account-sim]
  bus[RabbitMQ]
  triage[triage]
  advisory[advisory]
  bff[bff]
  queue[team queue]
  acct -->|message.received| bus
  bus --> triage
  triage -->|message.triaged| advisory
  advisory -->|alert.raised| bff
  bff -->|SSE| queue
```

A complaint or free message takes that path. cases and timeline-indexer also receive `message.triaged`, as in phase 1.

```mermaid
flowchart LR
  acct[account-sim]
  bus[RabbitMQ]
  advisory[advisory]
  bff[bff]
  queue[team queue]
  acct -->|account.event.recorded| bus
  bus --> advisory
  advisory -->|alert.raised| bff
  bff -->|SSE| queue
```

A deposit or withdrawal takes that path and updates `book.aum` and `book.segment`. timeline-indexer receives `account.event.recorded` as well. An action that no rule turns into an alert still reaches the timeline.
