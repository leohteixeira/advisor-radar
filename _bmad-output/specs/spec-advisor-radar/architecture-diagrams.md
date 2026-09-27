# Architecture diagrams

## Runtime

```mermaid
flowchart LR
  web[web] -->|HTTP| bff[bff]
  bff -->|SSE signal| web
  bff -->|gRPC| advisory
  bff -->|gRPC| cases
  bff -->|search| indexer[timeline-indexer]
  sim[account-sim] -->|account.event.recorded| advisory
  sim -->|account.event.recorded| indexer
  sim -->|message.received| triage
  sim -->|message.received| indexer
  triage -->|message.triaged| advisory
  triage -->|message.triaged| cases
  triage -->|message.triaged| indexer
  advisory -->|alert.raised| bff
  advisory -->|alert.raised| cases
  advisory -->|alert.raised| indexer
  cases -->|case.opened and case.status.changed| bff
  cases -->|case.opened and case.status.changed| indexer
  cases -->|case.sla.breached| bff
  cases -->|case.sla.breached| advisory
```

The BFF is the only process the browser calls. It stores nothing. Opening a case from an alert is the async path `alert.raised` → `cases` → `case.opened`, not a gRPC write.

## Triage call

```mermaid
flowchart TB
  msg[message] --> fb[Fallback]
  fb --> bulk[Bulkhead]
  bulk --> br[Circuit breaker]
  br --> retry[Retry]
  retry --> limit[Rate limiter]
  limit --> timeout[Timeout]
  timeout --> http[POST /v1/evaluate]
  fb -->|primary error and caller still active| heur[Keyword heuristic]
```

One message is one breaker result. Each HTTP attempt takes one rate-limit token and its own timeout.

## Demo trace

```mermaid
sequenceDiagram
  participant Sim as account-sim
  participant Adv as advisory
  participant Bff as bff
  participant Web as web
  Sim->>Adv: account.event.recorded
  Adv->>Bff: alert.raised
  Bff->>Web: SSE signal
```

The same trace id is on the RabbitMQ headers and on the SSE event.
