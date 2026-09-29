---
id: SPEC-client-pov
companions:
  - architecture.md
  - architecture-diagrams.md
  - ux.md
  - ../spec-advisor-radar/SPEC.md
  - ../spec-advisor-radar/architecture.md
  - ../../../docs/design/clients/README.md
  - ../../../docs/design/clients/ClientApp.dc.html
  - ../../../docs/design/clients/Main.dc.html
  - ../../../docs/design/clients/Selecao-mobile.dc.html
  - ../../../docs/design/clients/ClientPov.dc.html
  - ../../../docs/design/clients/ClientPov-Desktop.dc.html
  - ../../../docs/design/base/handoff/tokens.css
sources:
  - ../../../../../docs/advisor-radar/advisor_radar_client_pov_brief.md
---

> **Canonical contract.** This SPEC and the files in `companions:` are the complete, preservation-validated contract for what to build, test, and validate. Source documents listed in frontmatter are for traceability — consult them only if you need narrative rationale or prose color this contract intentionally omits.

# Advisor Radar client point of view

## Why

A **pain to solve** for a live interview demo. Phase 1 already turns fictional account events into a team queue, but those events are seeded, so a visitor only sees the advisor side and has to trust the path. Phase 2 lets the interviewer cause the event: file a complaint in the client app, switch views, and see the same alert on the queue, on one trace, in under 2 seconds. The selection screen also tells a cold visitor what the product is and how the services connect. The team, the analyst, and the manager keep the phase-1 product.

## Capabilities

- **CAP-1**
  - **intent:** A visitor can choose the team view or the client view, and can see what the product does and how one complaint crosses the system.
  - **success:** `/advisor-radar` renders the selection screen and no longer redirects to the queue. The team card opens the existing queue. The client card opens the client list. The screen states the three pillars and plays an eight-step complaint walkthrough that can be paused and honors `prefers-reduced-motion`.

- **CAP-2**
  - **intent:** A visitor can enter the fictional brokerage as one seeded client per segment, without a separate sign-in.
  - **success:** The list shows Fernanda Lima (Essencial), Thiago Azevedo (Advance), and Mariana Costa (Singular), each with segment, assets, base SLA, advisor, and a hint. The home shows assets, allocation, cash available to withdraw, the advisor, and recent activity. A fixed strip names the simulation and the current client and can switch client.

- **CAP-3**
  - **intent:** A visitor can deposit, withdraw, send a free message, or file a complaint so the pipeline the team already uses receives one real event.
  - **success:** An accepted deposit or withdrawal is `account.event.recorded` (`aporte` or `saque`). An accepted free message or complaint is `message.received` (complaint on chat; free message on chat or e-mail). The response is `202` with `event_id`. The same `Idempotency-Key` does not add a second event. A withdrawal above available cash is refused with a readable error and publishes nothing. On a path that raises a queue alert, the click reaches that alert in under 2 seconds at p95.

- **CAP-4**
  - **intent:** After an accepted action, a visitor can see that it was recorded and can open the team queue.
  - **success:** The confirmation shows a protocol, the routing key, the `event_id`, and progress from the outbox to the broker to triage or rules to the queue, pushed on that customer's SSE stream, and it links to `/advisor-radar/fila`. The protocol is the first two groups of `event_id`, uppercased, drawn by the UI. The `202` body has `event_id` and no protocol field. Until OpenTelemetry is instrumented, the panel shows the `event_id` and the steps the stream has observed, and does not show a live `trace_id`. If live Bastidores is the cut that shipped, the confirmation still shows the `event_id` and the queue link.

- **CAP-5**
  - **intent:** The team queue, the 360 view, and the manager panel keep phase-1 behavior and show the client's actions.
  - **success:** Every accepted POV action appears on that customer's timeline. A deposit or withdrawal updates the book assets and segment that the rules and the queue use. Mariana's preset "Estou pensando em sair" shows as a churn-risk alert with the Singular SLA. Fernanda's USD 10,000 deposit shows the large-deposit and segment-change alerts. Other team behavior matches the adopted phase-1 SPEC.

- **CAP-6**
  - **intent:** An operator can put the three client accounts back to the starting seed so the demo can be repeated.
  - **success:** After reseed, the three accounts match the class balances in `architecture.md`. Mariana's available cash is USD 60,000, and Fernanda's assets are the Essencial amount a USD 10,000 deposit both exceeds and uses to cross the segment band.

- **CAP-7**
  - **intent:** An evaluator can follow one client action from the click to the queue and can read the phase-2 decisions without opening the code.
  - **success:** Once tracing is on, one `trace_id` covers the HTTP request, the gRPC command, the outbox row, and the RabbitMQ headers, and Bastidores shows that id. ADR 0008, the POV HTTP contract next to the current BFF contract, and the README demo script are present. Counts exist for POV actions by type, refusals by rule, and requests collapsed by idempotency.

## Constraints

- Interview in the week of 28 September 2026, one developer. Data, names, and branding are fictional. Orla Invest copies no real institution.
- Phase 1 still bends this work: one database per service, an outbox on every publisher, an inbox on every consumer, the BFF stores nothing, the frontend talks only to the BFF, and business rules stay in the backend. Team behavior is the adopted phase-1 SPEC.
- A client action is a gRPC command to account-sim with a propagated deadline. account-sim writes the new state and the event in one transaction and returns `event_id`. The BFF answers `202` and does not publish. ADR 0008, extending ADR 0001, is written before that code.
- No new event type. Origin, destination, and integer-cent amounts ship on `schema_version` 2. `schema_version` 1 stays whole dollars, as already stored. Consumers accept both and read `amount`, `before`, and `after` by version. Every POST requires `Idempotency-Key`.
- account-sim is the source of truth for balances. Advisory updates `book.aum` and `book.segment` only from `account.event.recorded`. triage, cases, and timeline-indexer keep their contracts. Recent activity and message history come from the timeline.
- Seed ids are the advisory book's fixed UUIDv7 values (ADR 0007). Artboard cash, allocation mix, and activity rows are not the seed. AUM targets and the available-cash inequality are in `architecture.md`. Thiago keeps the segment-upgrade history already seeded.
- The client app does not evaluate alert rules; a withdrawal warning on the form is presentational. Shipped UI copy is Portuguese. Selection and the client list stay on the Advisor Radar light theme. The client app has its own themes, dark by default. Desktop actions use a 440px side panel; mobile actions are full screen. Deposit is a plus in a circle; withdrawal is a bank.
- A withdrawal debits available cash and total assets. It does not sell ações, ETFs, or renda fixa. account-sim stores those four classes and no individual asset. Caixa is the available cash.
- Bastidores progress is `GET /v1/client-pov/customers/{id}/stream`, an in-memory hub on the BFF. The public app is rooted at `https://leohts.tech/advisor-radar`. The team queue is `/advisor-radar/fila`; review, the manager panel, and the 360 view are `/advisor-radar/revisao`, `/advisor-radar/painel`, and `/advisor-radar/clientes/{id}`.
- Info logs omit customer free text and full account values. The BFF allows 10 new POV commands per customer per 60 seconds and 40 per customer per 24 hours. A replay of a known `Idempotency-Key` does not spend that budget. Over the limit the response is `429` and nothing is published.
- If time runs out, cut only in the order in `architecture.md`. The minimum ship is selection, the client list, a preset complaint, and a deposit and a withdrawal through gRPC, the outbox, and idempotency.

## Non-goals

- Client authentication. Choosing a client is the session.
- Advisor replies inside the client app, and any message sent to a real person.
- Working Investir, Carteira, and Perfil screens. Those items are visual only.
- Trading, quotes, a full statement, and real foreign exchange.
- Creating clients in the UI. The three POV clients come from the seed.
- A dark theme for selection or the client list.
- A new event type, or the BFF publishing events.
- A notifications inbox. The control on the client app is visual only.

## Success signal

On a phone, the interviewer opens `/advisor-radar`, explains the architecture walkthrough in about 30 seconds, enters as Mariana Costa, sends "Estou pensando em sair", and sees that alert at the top of the team queue with the Singular SLA. The click-to-alert path is under 2 seconds at p95. As Fernanda Lima, a USD 10,000 deposit raises the large-deposit and segment-change alerts. Sending the same withdrawal twice with the same idempotency key leaves one event.
