---
id: SPEC-advisor-radar
companions:
  - architecture.md
  - architecture-diagrams.md
  - state-machines.md
  - triage.md
  - ux.md
  - ../../../docs/design/Advisor Radar.dc.html
  - ../../../docs/design/Advisor Radar - Design System.dc.html
  - ../../../docs/design/handoff/tokens.css
  - ../../../docs/design/handoff/tokens.json
  - ../../../docs/design/mock-data.js
  - ../../../../radar-triage/testdata/messages.json
sources:
  - ../../../../../docs/advisor_radar_brief.md
  - ../../../docs/design/handoff/components.md
  - ../../../../radar-triage/README.md
  - ../../../../radar-triage/internal/triage/triage.go
  - ../../../../radar-triage/internal/triage/heuristic.go
  - ../../../../radar-triage/internal/triage/jev_classifier.go
  - ../../../../radar-triage/internal/jev/client.go
---

> **Canonical contract.** This SPEC and the files in `companions:` are the complete, preservation-validated contract for what to build, test, and validate. Source documents listed in frontmatter are for traceability — consult them only if you need narrative rationale or prose color this contract intentionally omits.

# Advisor Radar

## Why

A **pain to solve** and a **mandate to meet**. Advisors covering more clients cannot see, in one place, who needs contact now: account events and customer messages live in different systems, so a frustrated client can leave before anyone calls. Advisor Radar turns fictional account events and messages into a prioritized queue with a reason and an SLA. The mandate is a live five-minute demo, on a phone, for a backend interview in the week of 28 September 2026. The data and the brand are fictional.

## Capabilities

- **CAP-1**
  - **intent:** An operator can fire a fictional market day that emits account events and customer messages.
  - **success:** One control publishes a burst; the resulting alerts and triaged messages show on the advisor queue without a reload.

- **CAP-2**
  - **intent:** The system can raise an alert from a fictional account event by a deterministic rule, and name that rule.
  - **success:** These five rules fire on the seed book: withdrawal above 20% of assets in 24 hours, drop above 15% in 5 business days, deposit larger than the previous assets, crossing a segment band, and no contact for more than 90 days.

- **CAP-3**
  - **intent:** Each inbound message can be classified by intent, frustration, churn risk, and whether the customer asked for a human.
  - **success:** On the 16-message labeled set, the model path matches intent, churn, and human on 16/16, and the keyword path matches intent on 11/16. Intent probability below 0.85 is offered to the analyst. Numbers and dates are not sent to the model, and the model does not draft a reply.

- **CAP-4**
  - **intent:** A message can still be classified when the external model cannot be called.
  - **success:** The queue does not wait on the model. The result is the keyword classification, marked degraded, and shown as "Classificação simplificada". The demo shows the breaker open, fallback in use, and recovery in half-open.

- **CAP-5**
  - **intent:** An advisor can move a case through its states while a segment SLA counts down and escalates on its own at expiry.
  - **success:** The states are Aberto, Em atendimento, Aguardando cliente, and Resolvido. Base SLA is 1440, 240, and 60 minutes for Essencial, Advance, and Singular. Expiry emits `case.sla.breached` with no cron. Letting a case expire in the demo shows the escalation.

- **CAP-6**
  - **intent:** An advisor can read one timeline of a customer's account events, triaged messages, cases, and notes, and search that text.
  - **success:** The 360 view shows those four kinds, and a text query returns the matching entries.

- **CAP-7**
  - **intent:** An advisor can watch a prioritized queue on a phone between meetings and on a desktop.
  - **success:** New signals appear without a reload. Below 900px the board scrolls horizontally and the detail replaces it. At 900px and above the same board is four columns and the detail opens in a modal.

- **CAP-8**
  - **intent:** A manager can see backlog by advisor, cases whose SLA is at risk, time from alert to first contact, and how often triage needed a human review.
  - **success:** The manager panel shows those four measures for the current demo day.

- **CAP-9**
  - **intent:** An analyst can review messages whose intent probability is below the gate and correct the intent.
  - **success:** The review queue lists those messages. Choosing an intent updates the label shown for that message and confirms the feedback.

- **CAP-10**
  - **intent:** An evaluator can follow one trace from an account event through the rule, the alert, and the SSE push, and can rerun the consumer-restart and model-down drills.
  - **success:** One trace covers that path. Restarting a consumer applies the same `event_id` once. Cutting the model still classifies every message in the burst.

- **CAP-11**
  - **intent:** An evaluator can read the decisions, the failure drills, and the measurements without opening the code.
  - **success:** The README, the six ADRs in `architecture.md`, a one-page runbook, and written k6 and probe results are present. The k6 writeup records the sustained throughput that was measured. A recorded backup of the live demo exists next to the deterministic seed.

## Constraints

- About six days, one developer, interview in the week of 28 September 2026. Data, names, and branding are fictional. No real institution name, mark, or logo.
- One Go module. One command per service: `account-sim`, `advisory`, `triage`, `cases`, `timeline-indexer`, `bff`. The React, TypeScript, and Vite frontend talks only to the BFF, over versioned HTTP/JSON plus one SSE connection. Business rules, scoring, and SLA stay in the backend. Shipped UI copy is Portuguese.
- A query between services is gRPC with a propagated deadline. A state change is a RabbitMQ event. Opening a case from an alert is an asynchronous command.
- Every publisher uses a transactional outbox. Every consumer uses an idempotent inbox keyed by `event_id`. Retries use backoff and a dead-letter queue per queue. SLA escalation uses a queue with TTL and a dead-letter exchange, not a cron.
- Databases are not shared. PostgreSQL for `account-sim`, `advisory`, and `triage`. MySQL for `cases`. Elasticsearch for `timeline-indexer`. The BFF stores nothing.
- Triage is the `radar-triage` classifier copied into this module, not imported across repositories. Keep the `Classifier` port, the Jev adapter, the keyword heuristic, and the `Fallback` decorator. The call is Jev on the Vercel AI Gateway, model `typesafe-ai/jev`, with zero data retention, as that prototype does. Resilience wraps that call in the order in `architecture.md`.
- Human review when the top intent probability is below 0.85. A degraded result is marked and does not enter the queue unless its probability is also below 0.85. The badge shows alta at or above 0.75, média at or above 0.5, and baixa below 0.5, and never the raw number. Alta can still need review.
- "Contatado" and "Adiar 1 h" are stored in advisory's PostgreSQL. A snooze lasts one hour. Undo removes that row. The BFF stores nothing.
- The public demo is served at `leohts.tech`.
- The breaker opens after 3 consecutive message-level failures, or when 5 of the last 10 outcomes failed, and after an immediate quota error. It stays open for 15 seconds. Half-open allows one trial. Each HTTP attempt times out at 2 seconds.
- SLA base is Essencial 1440, Advance 240, Singular 60 minutes. The clock is halved when churn risk is true, frustration is Frustrado or worse, the customer asked for a human, or the alert is a relevant withdrawal.
- UI layout, tokens, and seed book match the adopted design companions. Light theme is the default.
- Never cut RabbitMQ, gRPC, outbox, inbox, or observability. If time or memory fails, cut Elasticsearch, then Jev, then MySQL, in that order.
- Do not log tokens, credentials, connection strings, or model-provider keys.

## Non-goals

- Real authentication, real brokerage or custody integration, and real messages sent to a customer.
- Generating reply text with a model.
- Multi-tenant, multi-region, or real high availability.
- Delayed reclassification after the breaker closes (`message.reclassify.requested`).
- A shared database, or a cron that escalates SLA.

## Success signal

On a phone, at `leohts.tech`, in about five minutes, an evaluator runs the demo in `ux.md`: a live queue, a market-day burst, a churn message with its triage signals, a case that escalates when its SLA expires, one Grafana trace of that path, a consumer restart with no duplicate effect, and a cut model that still classifies. Event-to-alert on screen is under 2 seconds at p95 for that demo path.

## Assumptions

- Essencial is assets up to USD 10,000 inclusive. Advance is above 10,000 through 200,000 inclusive. Singular is above 200,000. The handoff text counts USD 10,000 in two bands.
- Churn risk and the human-request flag count as true at probability 0.5 or higher.
- The frustration indicator uses the score rounded to the nearest integer from 0 to 3. The raw score remains on the result.
- `github.com/sony/gobreaker/v2` and `golang.org/x/time/rate` are the libraries to use. Bulkhead and the retry loop use the standard library.
- An analyst correction updates the intent shown in the session. The toast treats it as calibration feedback. The sources do not say the correction is stored on the server.
- Display bands stay at 0.75 and 0.5. Review membership is probability below 0.85, including part of the alta band. The seed `REVIEW` list follows that gate.
