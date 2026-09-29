# Architecture diagrams

Phase-3 flows. The phase-2 command and fan-out diagrams stay in the adopted `../spec-client-pov/architecture-diagrams.md`.

## Phase A: before and after

```mermaid
flowchart LR
  subgraph Before
    W1[web] -->|HTTP| B1[bff]
    B1 --> M1[povmem.go + sim.Memory<br/>in the bff process]
  end
  subgraph After
    W2[web] -->|HTTP| B2[bff]
    B2 -->|gRPC account/v1<br/>deadline propagated| A2[account-sim]
    A2 --> P2[(PostgreSQL<br/>pov_account + outbox)]
    A2 -->|relay| R2[RabbitMQ]
  end
```

## Screen build

```mermaid
sequenceDiagram
  participant Web
  participant BFF as bff (internal/screen)
  participant Acct as account-sim
  participant Adv as advisory
  participant Cases as cases
  participant TL as timeline
  Web->>BFF: GET /v1/client-pov/customers/{id}/screens/{slug}
  par one screen deadline
    BFF->>Acct: account, positions, preferences, beta
    BFF->>Adv: customer, investor profile, moment facts
    BFF->>Cases: ListCases(customer)
    BFF->>TL: Search(customer)
  end
  BFF->>BFF: per section: first matching Variant, default last
  BFF->>BFF: fill copy from embedded catalog (text/template)
  BFF-->>Web: 200 {schema_version, slug, revision, sections[]}
  Web->>Web: render by type, one error boundary per component
```

If a source fails or times out, its sections fall back to the default variant or are omitted. The response is still `200`.

## Purchase above profile

```mermaid
sequenceDiagram
  participant Web
  participant BFF as bff
  participant Acct as account-sim
  participant RMQ as RabbitMQ
  participant Adv as advisory
  Web->>BFF: POST /v1/client-pov/customers/{id}/purchases + Idempotency-Key
  BFF->>Acct: gRPC purchase, deadline propagated
  Acct->>Acct: cash → position, outbox row, one transaction
  Acct-->>BFF: event_id
  BFF-->>Web: 202 event_id (Bastidores)
  Acct->>RMQ: account.event.recorded kind aplicacao, schema 3
  RMQ->>Adv: inbox by event_id
  Adv->>Adv: risk > profile max risk → suitability-mismatch rule
  Adv->>RMQ: alert.raised
  RMQ-->>BFF: alert.raised → SSE team queue (p95 < 2 s)
```

## Advance one market day

```mermaid
sequenceDiagram
  participant Web
  participant BFF as bff
  participant Acct as account-sim
  participant RMQ as RabbitMQ
  participant Adv as advisory
  Web->>BFF: POST /v1/client-pov/simulation/advance-day + Idempotency-Key
  BFF->>Acct: gRPC advance day (global rate limit in bff)
  Acct->>Acct: one transaction: sim_day+1, revalue every account,<br/>one reavaliacao outbox row per account<br/>key reavaliacao:{customer_id}:{sim_day}
  Acct-->>BFF: new sim_day
  BFF-->>Web: 202
  Acct->>RMQ: account.event.recorded kind reavaliacao × accounts
  RMQ->>Adv: book follows patrimony; drop rule on loss > 15%
  Adv->>RMQ: alert.raised (Mariana on the shock day)
```

On the next home load, advisory's moment facts carry the drop and the BFF serves `portfolio_drop`.
